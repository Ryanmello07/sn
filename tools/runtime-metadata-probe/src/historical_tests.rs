//! Synthetic programs exercise the public decoder and exact pinned SDK hosts.
//! Parent proofs contain every fixture trie node; expected post-state roots
//! are independently built from complete maps, never copied from replay output.
//! These programs are not actual-runtime admission or fee-hook witnesses.

use super::*;
use sp_core::{
    storage::{ChildInfo, Storage, StorageChild},
    H256,
};
use sp_runtime::generic::{Digest, DigestItem};
use sp_state_machine::{prove_read_on_trie_backend, TestExternalities};
use std::borrow::Cow;

const OWNER: &[u8] = b"synthetic-child";
const ACCOUNT: &[u8] = b"zzzz-account";
const READ: &str = "(drop (call $get (i64.const 51539609536)))";
const WRITE: &str = "(call $set (i64.const 51539609536) (i64.const 4294970400))";
const CHILD_WRITE: &str =
    "(call $child_set (i64.const 64424512512) (i64.const 4294970312) (i64.const 4294970400))";

/// Offsets are fixed fixture data. The normal program modifies one complete
/// state; no expected-root value or verifier branch is imported by the Wasm.
fn wasm(imports: &str, body: &str) -> Vec<u8> {
    let version = RuntimeVersion {
        spec_name: Cow::Borrowed("synthetic-historical-runtime"),
        spec_version: 1,
        transaction_version: 1,
        system_version: 1,
        ..RuntimeVersion::default()
    }
    .encode();
    let escaped = version
        .iter()
        .map(|byte| format!("\\{byte:02x}"))
        .collect::<String>();
    wat::parse_str(format!(r#"(module
        (import "env" "ext_storage_get_version_1" (func $get (param i64) (result i64)))
        (import "env" "ext_storage_set_version_1" (func $set (param i64 i64)))
        (import "env" "ext_storage_clear_version_1" (func $clear (param i64)))
        (import "env" "ext_storage_root_version_2" (func $root (param i32) (result i64)))
        (import "env" "ext_storage_start_transaction_version_1" (func $begin))
        (import "env" "ext_storage_commit_transaction_version_1" (func $commit))
        (import "env" "ext_storage_rollback_transaction_version_1" (func $rollback))
        (import "env" "ext_default_child_storage_get_version_1" (func $child_get (param i64 i64) (result i64)))
        (import "env" "ext_default_child_storage_set_version_1" (func $child_set (param i64 i64 i64)))
        (import "env" "ext_default_child_storage_clear_version_1" (func $child_clear (param i64 i64)))
        {imports}
        (memory (export "memory") 8)
        (global (export "__heap_base") i32 (i32.const 8192))
        (data (i32.const 32) "{escaped}")
        (data (i32.const 1984) "zzzz-account")
        (data (i32.const 3072) "synthetic-child")
        (data (i32.const 3016) "k")
        (data (i32.const 3104) "v")
        (func (export "Core_version") (param i32 i32) (result i64) (i64.const {version_span}))
        (func (export "Core_execute_block") (param i32 i32) (result i64)
            (local $n i32) {body} (i64.const 0)))"#,
        version_span = (version.len() as u64) << 32 | 32
    )).expect("synthetic Wasm fixture compiles")
}

fn encoded(raw: &[u8]) -> String {
    format!("0x{}", hex::encode(raw))
}

fn parent_storage(code: &[u8]) -> Storage {
    let mut storage = Storage::default();
    storage
        .top
        .insert(well_known_keys::CODE.to_vec(), code.to_vec());
    storage.top.insert(ACCOUNT.to_vec(), vec![7; 96]);
    storage
        .top
        .insert(b"unrelated-original".to_vec(), vec![8; 96]);
    storage.children_default.insert(
        OWNER.to_vec(),
        StorageChild {
            child_info: ChildInfo::new_default(OWNER),
            data: [
                (b"k".to_vec(), vec![9; 96]),
                (b"other".to_vec(), vec![10; 96]),
            ]
            .into_iter()
            .collect(),
        },
    );
    storage
}

/// Complete raw snapshots retain all top-level, child, and hashed-value nodes.
/// The child expected map uses the SDK's full-state builder independently.
fn job(code: &[u8], change: impl FnOnce(&mut Storage)) -> HistoricalJob {
    let initial = parent_storage(code);
    let backing = TestExternalities::<Blake2Hasher>::new_with_code_and_state(
        code,
        initial.clone(),
        StateVersion::V1,
    );
    let (nodes, parent_root) = backing.into_raw_snapshot();
    let parent = NativeHeader::new(
        4,
        H256::repeat_byte(2),
        parent_root,
        H256::repeat_byte(1),
        Digest::default(),
    );
    let mut expected = initial;
    change(&mut expected);
    let backing = TestExternalities::<Blake2Hasher>::new_with_code_and_state(
        code,
        expected,
        StateVersion::V1,
    );
    let body = vec![vec![1u8, 2, 3].encode()];
    let child = NativeHeader::new(
        5,
        BlakeTwo256::ordered_trie_root(body.clone(), StateVersion::V1),
        *backing.backend.root(),
        parent.hash(),
        Digest::default(),
    );
    HistoricalJob {
        schema: HISTORICAL_SCHEMA.to_owned(),
        parent_header_hex: encoded(&parent.encode()),
        parent_hash: parent.hash().0,
        child_header_hex: encoded(&child.encode()),
        child_hash: child.hash().0,
        extrinsics_hex: body.iter().map(|bytes| encoded(bytes)).collect(),
        runtime_code_hex: encoded(code),
        runtime_code_sha256: sha2_256(code),
        runtime_code_blake2b_256: blake2_256(code),
        execution_state_version: 1,
        proof_nodes_hex: nodes
            .into_iter()
            .map(|(_, (value, _))| encoded(&value))
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect(),
    }
}

fn run(job: &HistoricalJob) -> Result<HistoricalReport, ProbeError> {
    let raw = replay_historical_json(&serde_json::to_vec(job).unwrap())?;
    Ok(serde_json::from_slice(&raw).expect("complete report decodes"))
}

fn replace_child(job: &mut HistoricalJob, mutate: impl FnOnce(&mut NativeHeader)) {
    let mut child: NativeHeader = scale_exact(
        "fixture child",
        &hex_bytes("fixture child", &job.child_header_hex, MAXIMUM_HEADER_BYTES).unwrap(),
    )
    .unwrap();
    mutate(&mut child);
    job.child_header_hex = encoded(&child.encode());
    job.child_hash = child.hash().0;
}

fn top_only_proof(job: &mut HistoricalJob, include_account: bool) {
    let code = hex_bytes("fixture code", &job.runtime_code_hex, MAXIMUM_CODE_BYTES).unwrap();
    let backing = TestExternalities::<Blake2Hasher>::new_with_code_and_state(
        &code,
        parent_storage(&code),
        StateVersion::V1,
    );
    let child = ChildInfo::new_default(OWNER);
    let mut keys = vec![
        well_known_keys::CODE.to_vec(),
        well_known_keys::HEAP_PAGES.to_vec(),
        child.prefixed_storage_key().as_slice().to_vec(),
    ];
    if include_account {
        keys.push(ACCOUNT.to_vec());
    }
    let proof = prove_read_on_trie_backend(&backing.backend, keys).unwrap();
    job.proof_nodes_hex = proof.into_iter_nodes().map(|node| encoded(&node)).collect();
}

#[test]
fn historical_complete_parent_proof_replays_top_and_child_writes() {
    let code = wasm(
        "",
        &format!("{READ} {WRITE} {CHILD_WRITE} (drop (call $root (i32.const 1)))"),
    );
    let job = job(&code, |storage| {
        storage.top.insert(ACCOUNT.to_vec(), b"v".to_vec());
        storage
            .children_default
            .get_mut(OWNER)
            .unwrap()
            .data
            .insert(b"k".to_vec(), b"v".to_vec());
    });
    let report = run(&job).expect("complete authenticated parent state refused");
    assert!(report.post_state_reproduced && report.storage_calls >= 4);
    assert_ne!(report.parent_state_root, report.child_state_root);
    assert_eq!(report.parent_hash, job.parent_hash);
    assert_eq!(report.child_hash, job.child_hash);
    assert_eq!(report.anchor_authority, "caller-supplied-unapproved");
    assert!(!report.runtime_admitted && !report.production_selection);
    assert_eq!(report.native_fee_debit, None);
    assert!(!report.native_fee_withdrawal_refund_observed);
}

#[test]
fn historical_complete_parent_proof_retains_unread_top_and_child_nodes() {
    let code = wasm("", "");
    let job = job(&code, |_| {});
    let nodes = job
        .proof_nodes_hex
        .iter()
        .map(|node| hex_bytes("node", node, MAXIMUM_CODE_BYTES).unwrap());
    let parent: NativeHeader = scale_exact(
        "parent",
        &hex_bytes("parent", &job.parent_header_hex, MAXIMUM_HEADER_BYTES).unwrap(),
    )
    .unwrap();
    let backend =
        create_proof_check_backend::<Blake2Hasher>(*parent.state_root(), StorageProof::new(nodes))
            .unwrap();
    for (key, expected) in parent_storage(&code).top {
        assert_eq!(backend.storage(&key).unwrap(), Some(expected));
    }
    for (key, expected) in &parent_storage(&code).children_default[OWNER].data {
        assert_eq!(
            backend
                .child_storage(&ChildInfo::new_default(OWNER), key)
                .unwrap()
                .as_ref(),
            Some(expected)
        );
    }
    assert_eq!(backend.storage(b"missing").unwrap(), None);
    assert_eq!(
        backend
            .child_storage(&ChildInfo::new_default(OWNER), b"missing")
            .unwrap(),
        None
    );
    assert!(run(&job).unwrap().post_state_reproduced);
}

#[test]
fn historical_missing_read_node_is_not_authenticated_absence() {
    let code = wasm("", READ);
    let mut job = job(&code, |_| {});
    top_only_proof(&mut job, false);
    let error = run(&job).expect_err("missing account proof became empty storage");
    assert!(
        error.to_string().contains("execute block refused"),
        "{error}"
    );
}

#[test]
fn historical_missing_write_path_refuses_unchanged_declared_root() {
    let code = wasm("", WRITE);
    let mut job = job(&code, |_| {});
    top_only_proof(&mut job, false);
    let error = run(&job).expect_err("missing write path accepted the old root");
    assert!(
        error.to_string().contains("post-state proof incomplete"),
        "{error}"
    );
}

#[test]
fn historical_missing_child_write_path_refuses_unchanged_declared_root() {
    let code = wasm("", CHILD_WRITE);
    let mut job = job(&code, |_| {});
    top_only_proof(&mut job, true);
    let error = run(&job).expect_err("missing child proof accepted original child/root");
    assert!(
        error.to_string().contains("post-state proof incomplete"),
        "{error}"
    );
}

#[test]
fn historical_missing_child_read_refuses_before_empty_value() {
    let code = wasm(
        "",
        "(drop (call $child_get (i64.const 64424512512) (i64.const 4294970312)))",
    );
    let mut job = job(&code, |_| {});
    top_only_proof(&mut job, true);
    let error = run(&job).expect_err("missing child node became an absent value");
    assert!(
        error.to_string().contains("execute block refused"),
        "{error}"
    );
}

#[test]
fn historical_complete_proof_supports_same_key_rollback_and_child_delete() {
    let code = wasm("", &format!("(call $begin) {WRITE} (call $rollback) {WRITE} {WRITE} (call $child_clear (i64.const 64424512512) (i64.const 4294970312))"));
    let job = job(&code, |storage| {
        storage.top.insert(ACCOUNT.to_vec(), b"v".to_vec());
        storage
            .children_default
            .get_mut(OWNER)
            .unwrap()
            .data
            .remove(b"k".as_slice());
    });
    assert!(
        run(&job)
            .expect("same-key/child deletion with complete proof refused")
            .post_state_reproduced
    );
}

#[test]
fn historical_empty_block_and_sealed_header_keep_original_identity() {
    let code = wasm("", "");
    let mut job = job(&code, |_| {});
    job.extrinsics_hex.clear();
    replace_child(&mut job, |header| {
        header.set_extrinsics_root(BlakeTwo256::ordered_trie_root(Vec::new(), StateVersion::V1));
        header
            .digest_mut()
            .push(DigestItem::Seal(*b"FAKE", vec![17; 64]));
    });
    let report = run(&job).expect("empty block with external seal refused");
    assert_eq!(report.child_hash, job.child_hash);
    assert_eq!(report.parent_state_root, report.child_state_root);
    assert_eq!(report.extrinsics, 0);
    assert!(!report.runtime_admitted);
}

#[test]
fn historical_header_body_parent_code_and_post_root_are_independent_guards() {
    let code = wasm("", WRITE);
    let valid = job(&code, |storage| {
        storage.top.insert(ACCOUNT.to_vec(), b"v".to_vec());
    });
    for fault in [
        "parent",
        "child",
        "body",
        "code",
        "code-proof",
        "post-root",
        "version",
    ] {
        let mut changed = valid.clone();
        match fault {
            "parent" => changed.parent_hash[0] ^= 1,
            "child" => changed.child_hash[0] ^= 1,
            "body" => changed.extrinsics_hex[0] = encoded(&vec![4u8, 5, 6].encode()),
            "code" => changed.runtime_code_sha256[0] ^= 1,
            "code-proof" => {
                let other = wasm("", "");
                changed.runtime_code_hex = encoded(&other);
                changed.runtime_code_sha256 = sha2_256(&other);
                changed.runtime_code_blake2b_256 = blake2_256(&other);
            }
            "post-root" => replace_child(&mut changed, |header| {
                header.set_state_root(H256::repeat_byte(19))
            }),
            "version" => changed.execution_state_version = 0,
            _ => unreachable!(),
        }
        let error = run(&changed).expect_err("substituted replay input accepted");
        assert!(error.to_string().contains("differs"), "{fault}: {error}");
    }
}

#[test]
fn historical_unsupported_host_is_not_a_successful_noop() {
    let code = wasm(
        "(import \"env\" \"ext_offchain_timestamp_version_1\" (func $clock (result i64)))",
        "(drop (call $clock))",
    );
    let error = run(&job(&code, |_| {})).expect_err("unsupported clock host silently executed");
    assert!(
        error.to_string().contains("execute block refused"),
        "{error}"
    );
}

#[test]
fn historical_unfinished_transaction_and_work_exhaustion_publish_no_fact() {
    for body in ["(call $begin)", "(loop $again (drop (call $get (i64.const 51539609536))) (local.set $n (i32.add (local.get $n) (i32.const 1))) (br_if $again (i32.lt_u (local.get $n) (i32.const 70000))))"] {
        let code = wasm("", body);
        let error = run(&job(&code, |_| {})).expect_err("unfinished or unbounded storage work accepted");
        assert!(error.to_string().contains("unfinished transaction") || error.to_string().contains("storage work bound"), "{error}");
    }
}

#[test]
fn historical_public_decoder_refuses_unknown_trailing_duplicate_and_bounded_inputs() {
    let code = wasm("", "");
    let valid = job(&code, |_| {});
    let mut raw: serde_json::Value = serde_json::to_value(&valid).unwrap();
    raw["invented_fee_verified"] = serde_json::Value::Bool(true);
    assert!(replay_historical_json(&serde_json::to_vec(&raw).unwrap()).is_err());
    for fault in [
        "trailing-header",
        "trailing-extrinsic",
        "duplicate-node",
        "node-count",
        "schema",
        "memory",
    ] {
        let mut changed = valid.clone();
        match fault {
            "trailing-header" => changed.parent_header_hex.push_str("00"),
            "trailing-extrinsic" => changed.extrinsics_hex[0].push_str("00"),
            "duplicate-node" => changed
                .proof_nodes_hex
                .push(changed.proof_nodes_hex[0].clone()),
            "node-count" => changed.proof_nodes_hex = vec!["0x01".to_owned(); 8193],
            "schema" => changed.schema.push('x'),
            "memory" => {
                assert!(
                    memory_bound(&wat::parse_str("(module (memory 2048))").unwrap(), None).is_err()
                );
                continue;
            }
            _ => unreachable!(),
        }
        assert!(run(&changed).is_err(), "malformed input accepted: {fault}");
    }
}
