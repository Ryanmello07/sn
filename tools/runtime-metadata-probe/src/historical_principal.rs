//! Read opening stake through the original parent runtime and strict backend.
//! Every query has an isolated overlay, shares the finite execution budget and
//! refuses writes. Returned stake is opening stock, not newly earned income.

use super::*;
use parity_scale_codec::Compact;
use sp_core::traits::CodeExecutor;

pub const STAKE_API: &str = "StakeInfoRuntimeApi_get_stake_info_for_hotkey_coldkey_netuid";

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq, PartialOrd, Ord)]
#[serde(deny_unknown_fields)]
pub struct PrincipalQuery {
    pub hotkey: [u8; 32],
    pub coldkey: [u8; 32],
    pub netuid: u16,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct PrincipalObservation {
    pub query: PrincipalQuery,
    pub result_hex: String,
    pub opening_stake_alpha: Option<String>,
    pub registered: Option<bool>,
}

// This exact SCALE layout has a separately reviewable source identity. The
// caller still admits that layout and runtime API in its independent policy.
#[derive(Decode, Encode)]
struct StakeInfo {
    hotkey: [u8; 32],
    coldkey: [u8; 32],
    netuid: Compact<u16>,
    stake: Compact<u64>,
    locked: Compact<u64>,
    emission: Compact<u64>,
    tao_emission: Compact<u64>,
    drain: Compact<u64>,
    registered: bool,
}

pub(super) fn validate(queries: &Option<Vec<PrincipalQuery>>) -> Result<(), ProbeError> {
    if let Some(queries) = queries {
        if queries.is_empty()
            || queries.len() > 4096
            || queries.iter().any(|query| {
                query.netuid == 0 || query.hotkey == [0; 32] || query.coldkey == [0; 32]
            })
            || queries.windows(2).any(|pair| pair[0] >= pair[1])
        {
            return Err(ProbeError::new(
                "historical principal census is empty, unordered, repeated or exceeds bound",
            ));
        }
    }
    Ok(())
}

pub(super) fn observe<B, E>(
    queries: &Option<Vec<PrincipalQuery>>,
    backend: &B,
    executor: &E,
    extensions: &mut Extensions,
    runtime: &RuntimeCode,
    parent: sp_core::H256,
) -> Result<Option<Vec<PrincipalObservation>>, ProbeError>
where
    B: Backend<Blake2Hasher>,
    E: CodeExecutor + Clone + 'static,
{
    validate(queries)?;
    let Some(queries) = queries else {
        return Ok(None);
    };
    let mut results = Vec::with_capacity(queries.len());
    for query in queries {
        let args = (query.hotkey, query.coldkey, query.netuid).encode();
        let mut overlay = OverlayedChanges::<Blake2Hasher>::default();
        extensions
            .get_mut(TypeId::of::<hosts::HistoricalBudget>())
            .and_then(|value| value.downcast_mut::<hosts::HistoricalBudget>())
            .ok_or_else(|| ProbeError::new("historical principal budget absent"))?
            .0
            .read_only = true;
        let execution = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            StateMachine::new(
                backend,
                &mut overlay,
                executor,
                STAKE_API,
                &args,
                extensions,
                runtime,
                CallContext::Onchain,
            )
            .set_parent_hash(parent)
            .execute()
        }));
        extensions
            .get_mut(TypeId::of::<hosts::HistoricalBudget>())
            .and_then(|value| value.downcast_mut::<hosts::HistoricalBudget>())
            .ok_or_else(|| ProbeError::new("historical principal budget absent"))?
            .0
            .read_only = false;
        let raw = execution
            .map_err(|_| {
                ProbeError::new("historical opening principal query panicked on parent proof")
            })?
            .map_err(|e| ProbeError::new(format!("historical opening principal query: {e}")))?;
        if raw.len() > 256
            || overlay.changes().next().is_some()
            || overlay.children().next().is_some()
            || overlay.transaction_depth() != 0
            || extensions
                .get_mut(TypeId::of::<hosts::HistoricalBudget>())
                .and_then(|value| value.downcast_mut::<hosts::HistoricalBudget>())
                .is_none_or(|value| value.0.depth != 0)
        {
            return Err(ProbeError::new("historical opening principal query changed state, left a transaction or exceeded bound"));
        }
        let value: Option<StakeInfo> = scale_exact("opening stake API result", &raw)?;
        if value.as_ref().is_some_and(|value| {
            value.hotkey != query.hotkey
                || value.coldkey != query.coldkey
                || value.netuid.0 != query.netuid
        }) {
            return Err(ProbeError::new(
                "historical opening principal API substituted requested identity",
            ));
        }
        if value.encode() != raw {
            return Err(ProbeError::new(
                "historical opening principal API returned noncanonical SCALE",
            ));
        }
        results.push(PrincipalObservation {
            query: query.clone(),
            result_hex: format!("0x{}", hex::encode(raw)),
            opening_stake_alpha: value.as_ref().map(|value| value.stake.0.to_string()),
            registered: value.map(|value| value.registered),
        });
    }
    Ok(Some(results))
}
