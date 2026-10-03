//! Observe original compiled-Wasm locations at host calls. No instruction is
//! inserted into the runtime. These records are proof-relative evidence for a
//! separately reviewed fee-hook decoder, not fee amounts or runtime admission.

use crate::ProbeError;
use serde::{Deserialize, Serialize};
use sp_core::hashing::sha2_256;
use sp_externalities::ExternalitiesExt;
use sp_wasm_interface::{Function, FunctionContext, HostFunctionRegistry, HostFunctions};
use std::collections::BTreeMap;

const MAXIMUM_RULES: usize = 32;
const MAXIMUM_RECORDS: usize = 4096;
const MAXIMUM_RETAINED_BYTES: usize = 2 * 1024 * 1024;
const MAXIMUM_STACK: usize = 64;

#[derive(Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct HookRule {
    pub purpose: String,
    pub function_index: u32,
    pub function_body_sha256: [u8; 32],
    pub offset_start: u32,
    pub offset_end: u32,
}

/// Digests bind an independently supplied review reference. They do not prove
/// that a reviewer signed it or that its labels identify deployed fee semantics.
#[derive(Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct ObservationProfile {
    pub schema: String,
    pub runtime_code_sha256: [u8; 32],
    pub source_review_sha256: [u8; 32],
    pub rules: Vec<HookRule>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Frame {
    pub function_index: u32,
    pub function_offset: u32,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Observation {
    pub ordinal: usize,
    pub purpose: String,
    pub operation: String,
    pub key_hex: String,
    pub value_hex: Option<String>,
    pub stack: Vec<Frame>,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct ObservationReport {
    pub profile_sha256: [u8; 32],
    pub source_review_sha256: [u8; 32],
    pub authority: String,
    pub original_function_bodies_preserved: bool,
    pub host_calls: usize,
    pub discarded_on_rollback: usize,
    pub observations: Vec<Observation>,
}

pub(super) struct Observer {
    profile: ObservationProfile,
    stack: Vec<Frame>,
    records: Vec<Observation>,
    transactions: Vec<usize>,
    total_records: usize,
    total_bytes: usize,
    calls: usize,
    discarded: usize,
}
sp_externalities::decl_extension! { pub(super) struct HistoricalObserver(Observer); }

/// Memory import/export normalization may move module offsets. Function-relative
/// positions remain admissible only if every code body is byte-identical after
/// the exact pinned SDK normalization used by this executor configuration.
fn bodies(wasm: &[u8]) -> Result<BTreeMap<u32, Vec<u8>>, ProbeError> {
    let mut functions = 0u32;
    let mut result = BTreeMap::new();
    for payload in wasmparser::Parser::new(0).parse_all(wasm) {
        match payload.map_err(|e| ProbeError::new(format!("observer Wasm parse: {e}")))? {
            wasmparser::Payload::ImportSection(section) => {
                for import in section {
                    if matches!(
                        import
                            .map_err(|e| ProbeError::new(format!("observer import: {e}")))?
                            .ty,
                        wasmparser::TypeRef::Func(_)
                    ) {
                        functions = functions
                            .checked_add(1)
                            .ok_or_else(|| ProbeError::new("observer function overflow"))?;
                    }
                }
            }
            wasmparser::Payload::CodeSectionEntry(body) => {
                result.insert(functions, wasm[body.range()].to_vec());
                functions = functions
                    .checked_add(1)
                    .ok_or_else(|| ProbeError::new("observer function overflow"))?;
            }
            _ => {}
        }
    }
    Ok(result)
}

impl HistoricalObserver {
    pub(super) fn new(
        profile: ObservationProfile,
        code_hash: [u8; 32],
        wasm: &[u8],
        heap_pages: Option<u64>,
    ) -> Result<Self, ProbeError> {
        if profile.schema != "urnetwork-original-wasm-hook-observation-v1"
            || profile.runtime_code_sha256 != code_hash
            || profile.source_review_sha256 == [0; 32]
            || profile.rules.is_empty()
            || profile.rules.len() > MAXIMUM_RULES
        {
            return Err(ProbeError::new(
                "observer profile, code or review reference differs",
            ));
        }
        let original = bodies(wasm)?;
        let mut normalized =
            sc_executor_common::runtime_blob::RuntimeBlob::uncompress_if_needed(wasm)
                .map_err(|e| ProbeError::new(format!("observer SDK runtime normalization: {e}")))?;
        normalized
            .convert_memory_import_into_export()
            .map_err(|e| ProbeError::new(format!("observer SDK memory normalization: {e}")))?;
        let heap = heap_pages
            .map(|pages| sc_executor::HeapAllocStrategy::Static {
                extra_pages: pages as u32,
            })
            .unwrap_or(sc_executor::HeapAllocStrategy::Dynamic {
                maximum_pages: Some(1024),
            });
        normalized
            .setup_memory_according_to_heap_alloc_strategy(heap)
            .map_err(|e| ProbeError::new(format!("observer SDK heap normalization: {e}")))?;
        if bodies(&normalized.serialize())? != original {
            return Err(ProbeError::new(
                "observer SDK normalization changed original function bytes",
            ));
        }
        for (index, rule) in profile.rules.iter().enumerate() {
            let body = original
                .get(&rule.function_index)
                .ok_or_else(|| ProbeError::new("observer callsite function absent"))?;
            if !matches!(
                rule.purpose.as_str(),
                "fee-withdraw" | "fee-refund" | "ethereum-executed"
            ) || sha2_256(body) != rule.function_body_sha256
                || rule.offset_start >= rule.offset_end
                || rule.offset_end as usize > body.len()
                || profile.rules[..index].iter().any(|prior| {
                    prior.function_index == rule.function_index
                        && prior.offset_start < rule.offset_end
                        && rule.offset_start < prior.offset_end
                })
            {
                return Err(ProbeError::new(
                    "observer original body, callsite range or unique purpose differs",
                ));
            }
        }
        Ok(Self(Observer {
            profile,
            stack: Vec::new(),
            records: Vec::new(),
            transactions: Vec::new(),
            total_records: 0,
            total_bytes: 0,
            calls: 0,
            discarded: 0,
        }))
    }

    pub(super) fn finish(&mut self) -> Result<ObservationReport, ProbeError> {
        if !self.0.transactions.is_empty() {
            return Err(ProbeError::new("observer unfinished storage transaction"));
        }
        let profile_bytes = serde_json::to_vec(&self.0.profile)
            .map_err(|e| ProbeError::new(format!("observer profile encoding: {e}")))?;
        Ok(ObservationReport {
            profile_sha256: sha2_256(&profile_bytes),
            source_review_sha256: self.0.profile.source_review_sha256,
            authority: "caller-supplied-unapproved-callsite-profile".to_owned(),
            original_function_bodies_preserved: true,
            host_calls: self.0.calls,
            discarded_on_rollback: self.0.discarded,
            observations: std::mem::take(&mut self.0.records),
        })
    }
}

/// The extension is job-owned; no global trace/budget can cross concurrent
/// replays. The complete block/postroot must succeed before any trace escapes.
pub(super) fn observe(
    mut ext: &mut dyn sp_core::traits::Externalities,
    operation: &str,
    key: &[u8],
    value: Option<&[u8]>,
) {
    let Some(observer) = ext.extension::<HistoricalObserver>() else {
        return;
    };
    let observer = &mut observer.0;
    observer.calls += 1;
    assert!(observer.calls <= 65536, "observer host work bound");
    let mut selected = None;
    for rule in &observer.profile.rules {
        if observer.stack.iter().any(|frame| {
            frame.function_index == rule.function_index
                && frame.function_offset >= rule.offset_start
                && frame.function_offset < rule.offset_end
        }) {
            assert!(
                selected.is_none(),
                "observer ambiguous original callsite purpose"
            );
            selected = Some(rule.purpose.clone());
        }
    }
    if let Some(purpose) = selected {
        // Hex encoding expands payloads; refuse before allocating that copy.
        let payload_bytes = key
            .len()
            .checked_add(value.map_or(0, <[u8]>::len))
            .expect("observer input overflow");
        assert!(
            payload_bytes <= MAXIMUM_RETAINED_BYTES / 2,
            "observer retained input bound"
        );
        let record = Observation {
            ordinal: observer.calls,
            purpose,
            operation: operation.to_owned(),
            key_hex: format!("0x{}", hex::encode(key)),
            value_hex: value.map(|bytes| format!("0x{}", hex::encode(bytes))),
            stack: observer.stack.clone(),
        };
        let size = serde_json::to_vec(&record)
            .expect("observer bounded record encoding")
            .len();
        observer.total_bytes = observer
            .total_bytes
            .checked_add(size)
            .expect("observer byte overflow");
        observer.total_records += 1;
        assert!(
            observer.total_records <= MAXIMUM_RECORDS
                && observer.total_bytes <= MAXIMUM_RETAINED_BYTES,
            "observer retained evidence bound"
        );
        observer.records.push(record);
    }
}

pub(super) fn transaction(mut ext: &mut dyn sp_core::traits::Externalities, operation: &str) {
    let Some(observer) = ext.extension::<HistoricalObserver>() else {
        return;
    };
    let observer = &mut observer.0;
    match operation {
        "start" => {
            assert!(
                observer.transactions.len() < 32,
                "observer transaction depth bound"
            );
            observer.transactions.push(observer.records.len());
        }
        "commit" => {
            observer
                .transactions
                .pop()
                .expect("observer transaction imbalance");
        }
        "rollback" => {
            let retained = observer
                .transactions
                .pop()
                .expect("observer transaction imbalance");
            observer.discarded += observer.records.len() - retained;
            observer.records.truncate(retained);
        }
        _ => panic!("observer unsupported transaction operation"),
    }
}

/// Wrap registration, not runtime code. The SDK executes the same host body;
/// Wasmtime supplies function-relative positions from the actual active stack.
pub(super) struct ObservedHosts<H>(std::marker::PhantomData<H>);
impl<H: HostFunctions> HostFunctions for ObservedHosts<H> {
    fn host_functions() -> Vec<&'static dyn Function> {
        H::host_functions()
    }
    fn register_static<T: HostFunctionRegistry>(registry: &mut T) -> Result<(), T::Error> {
        struct Registry<'a, T>(&'a mut T);
        impl<T: HostFunctionRegistry> HostFunctionRegistry for Registry<'_, T> {
            type State = T::State;
            type Error = T::Error;
            type FunctionContext = T::FunctionContext;
            fn with_function_context<R>(
                caller: wasmtime::Caller<Self::State>,
                callback: impl FnOnce(&mut dyn FunctionContext) -> R,
            ) -> R {
                let enabled = sp_externalities::with_externalities(|mut ext| {
                    ext.extension::<HistoricalObserver>().is_some()
                })
                .unwrap_or(false);
                let stack = if enabled {
                    let trace = wasmtime::WasmBacktrace::force_capture(&caller);
                    assert!(
                        !trace.frames().is_empty() && trace.frames().len() <= MAXIMUM_STACK,
                        "observer Wasm stack bound"
                    );
                    trace
                        .frames()
                        .iter()
                        .map(|frame| Frame {
                            function_index: frame.func_index(),
                            function_offset: u32::try_from(
                                frame
                                    .func_offset()
                                    .expect("observer original code location missing"),
                            )
                            .expect("observer offset overflow"),
                        })
                        .collect()
                } else {
                    Vec::new()
                };
                T::with_function_context(caller, |context| {
                    if enabled {
                        sp_externalities::with_externalities(|mut ext| {
                            ext.extension::<HistoricalObserver>()
                                .expect("observer disappeared")
                                .0
                                .stack = stack;
                        })
                        .expect("observer execution context absent");
                    }
                    callback(context)
                })
            }
            fn register_static<Params, Results>(
                &mut self,
                name: &str,
                function: impl wasmtime::IntoFunc<Self::State, Params, Results> + 'static,
            ) -> Result<(), Self::Error> {
                self.0.register_static(name, function)
            }
        }
        H::register_static(&mut Registry(registry))
    }
}
