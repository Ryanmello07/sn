# Offline owner recycle transition

The subnet owner can change SN25 from Burn to Recycle on the inspected runtime
470. This resolves the authority question behind MG-06; it does not establish
that a transition occurred. The public observations at blocks 9,186,298 and
9,187,604 both found absent storage, which means **Burn**. Launch remains blocked
until an approved owner operation has an exact finalized receipt and Recycle
readback, followed by the separate native 10% provider / 90% recycle evidence.

## Exact authority and call

At official source `923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d`,
`AdminUtils.sudo_set_recycle_or_burn` is call **80**. Its native arguments are
SCALE `NetUid(u16)` and `RecycleOrBurnEnum`: Burn is 0, Recycle is 1. The planner
authenticates the pallet index and argument types from independently pinned
metadata and constructs exactly `pallet_index || 50 || 1900 || 01`. It supports
only the direct owner call for netuid 25 and Recycle. No batch, proxy, Root or EVM
wrapper is constructed. [Pinned dispatch][setter]

The retained exact official metadata resolves AdminUtils to pallet 19, so the
unsigned call bytes are `0x1350190001`. A runtime number or that byte string alone
does not authorize use; independent code/metadata pins and owner approval remain
required.

`ensure_sn_owner_or_root_with_limits` accepts the signed `SubnetOwner[25]` or
chain Root. The owner path checks `Hyperparameter::RecycleOrBurn` (variant 24).
The last update is stored **per subnet/hyperparameter**, not per coldkey, in
`LastRateLimitedBlock[OwnerHyperparamUpdate(25, RecycleOrBurn)]`; its SCALE key
argument is `01 19 00 18`. The owner must wait at least
`Tempo[25] * OwnerHyperparamRateLimit` blocks after a nonzero last update.
The default global limit is two tempos. Successful owner dispatch records the
current block. Root bypasses this owner rate limit. [Origin check][origin],
[rate rule and key][rate]

Both owner and Root encounter the admin window. With nonzero tempo, a future
`PendingEpochAt[25]` freezes the whole pending countdown; otherwise the call is
prohibited when `max(LastEpochBlock[25] + Tempo[25] - current_block, 0)` is less
than `AdminFreezeWindow` (default 10). Tempo 0 bypasses this freeze. The planner
requires all possible inclusion blocks in its original mortal era to pass the
observed predicates. This conservative test cannot prevent later owner or Root
changes. [Admin window][origin], [scheduler][scheduler]

## Offline workflow

No `owner-recycle` command signs, broadcasts, starts a service or loads a
private key. All custody files must be regular, private files in a canonical
absolute owner-private directory. Keep the journal and permanent `.lock` marker
together, and use one globally exclusive owner-key custody process across hosts.
Local file locking alone cannot provide that global exclusion.

1. Supply an independently reviewed [recycle policy](ECONOMIC-GATE.md), an
   independently pinned raw owner AccountId32 and an explicit owned RPC route.
   Run `owner-recycle observe --policy FILE --owner-account-id HEX --rpc URL`.
   It reads one finalized header and exact runtime/artifacts, owner, registration
   block, nonce/free balance, mode, rate record and admin-window rows at that hash.
   It rechecks network and finalized mapping. The result is a sealed observation,
   not an approval or a GRANDPA/storage proof. Reads have a finite 60–900s window.
2. Prepare the private JSON `ownerRecyclePlanInput` described below and run
   `owner-recycle plan --input FILE`. The output is an **unsigned** config.
   Independently review its exact action, artifact/source provenance, route,
   custody and fee exposure, then obtain a fresh Ed25519 approval for the
   `urnetwork-mainnet-owner-recycle-execution-v1` domain. Trim, bootstrap and
   read-policy approvals cannot substitute. The approval message is the schema,
   a NUL byte, then the canonical Go JSON config with its signature field empty.
3. Run `owner-recycle reserve --config FILE --approval-key HEX
   --accept-action-hash HASH`. This claims one permanent journal.
4. Run `owner-recycle export` with those same flags plus `--metadata FILE`
   (canonical metadata14 hex), and `--ledger-metadata FILE` for Ed25519. It syncs
   the original portable request **before** writing it to stdout. A lost output
   can re-export only that identical request; it cannot renew the nonce or era.
5. On the owner's separate computer, use `owner-recycle inspect-request
   --request FILE --accept-request-hash HASH --approval-key HEX
   --owner-account-id HEX --expected-genesis HEX`. These pins must come from
   independent review. Embedded Snow paths and routes are inert. For Ledger,
   `ledger-plan` accepts the same flags plus `--metadata-proof FILE
   --metadata-proof-sha256 HASH` and emits an APDU transcript, not a device call.
6. After a separately qualified owner device/custody process returns the
   original public signature, run `owner-recycle import` with the common
   config/approval/action flags plus `--accept-request-hash HASH --signature FILE
   --signature-file-sha256 HASH`. `--ledger-response` requires exactly variant 0
   plus 64 Ed25519 bytes, excluding APDU status words. Without it, import requires
   a raw 64-byte signature in hex. The owner signature is verified over the exact
   native payload; the full signed extrinsic and its hash become durable.
7. `owner-recycle status` with the common flags returns the retained public
   request and exact signed extrinsic. No transmission command is installed.
   Any separately authorized external transmission must use those exact bytes,
   the approved route and original era, with qualified global owner custody and
   bounded attempt/fee policy. Never regenerate the action after ambiguous send.
8. `owner-recycle reconcile` with the common flags reads only the approved route.
   It scans every canonical body from birth+1 through the earlier of finalized
   head and era-death−1. It retains the exact inclusion, phase-bound dispatch and
   fee, then reads the inclusion-block mode/rate/generation. Repeat read-only
   reconciliation after an unresolved result; never replace original bytes.

An era is a power of two from 4 through 256 blocks, anchored at an authenticated
u32 birth height/hash; tip is zero. Archive recovery is bounded to 4096 blocks
after birth and 15 minutes per pass. A longer gap needs separate reviewed archive
recovery, not a new request. Read retries never become write retries.

## Plan input and signing profiles

The private plan JSON contains `action`, the full `observation`,
`runtime_metadata_scale`, optional `ledger_metadata_scale`, and `owned_route`.
The action has the following exact scope:

| Field | Required value |
| --- | --- |
| `schema` | `urnetwork-mainnet-owner-recycle-action-v1` |
| `policy` | Complete independently reviewed recycle policy; inspected node-subtensor 470/transaction 1/state 1 source only |
| `birth_observation_hash` | The complete retained observation's content hash |
| `independent_review_hash` | SHA-256 seal of the independent transition review |
| `custody_id`, `state_path` | Explicit custody label; absolute private path ending `owner-recycle-action.json` |
| `owner_account_id`, `subnet_registration_block` | Exact observed owner and original subnet registration |
| `nonce`, `birth_block`, `birth_hash` | Exact observed nonce and finalized anchor; nonce must not be u32 maximum |
| `mortal_period`, `fee_reserve_rao` | Approved bounded era and nonzero reserve supported by observed free balance |
| `signature_scheme` | `sr25519` or `ed25519` |
| `check_metadata_hash`, `signer_derivation_path`, `ledger_metadata_blake2b_256` | Required for Ed25519; absent for Sr25519 |
| `call_index`, `call_scale`, `payload_scale`, `request_hash` | Derived and sealed by the planner; input values confer no authority |

`owned_route` uses the existing explicit RPC URL and bounded read/send timeout
grammar; accepting it does not enable sending. The observation must contain
Burn, the same owner/registration/nonce/anchor and sufficient free balance.
An observed reserve is not a runtime maximum-fee field or fee enforcement.

Sr25519 uses `MultiSignature::Sr25519` (variant 1) and disabled metadata-hash
mode. Ledger uses native Ed25519 (variant 0), enabled `CheckMetadataHash`, an
independently approved RFC78 digest, raw metadata15 hash and canonical five-word
derivation path. A 64-byte adapter cannot accept native ECDSA65.

The existing pinned generic Polkadot app framing is reused without changing
trim's wire bytes: address confirmation, little-endian path words, bounded
payload length and 250-byte chunks. Payload plus shortened proof is at most 16 KiB.
The transcript explicitly reports `device_qualified: false`,
`metadata_proof_verified: false`, `signing: false`, `network_effects: false`.
The owner's actual device/platform, current metadata15/RFC78 digest and proof,
source-to-Wasm exception and one-request device journal still require independent
qualification. This increment does not connect the existing trim-only device
signer to a new action type. [Owner signing boundaries](OWNER-SIGNING.md)

## Recovery and outcome

The durable phases are `reserved`, `exported`, `signed`, `finalized`,
`finalized-readback-pending`, `finalized-state-conflict`, `dispatch-failed`,
`fee-overrun`, `expired` and `nonce-conflict`. A returned signature may be
imported repeatedly only if it is byte-for-byte identical. A different valid
randomized Sr25519 signature is a replacement and is refused. No signed/terminal
record can be exported for fresh signing. Ambiguous durability poisons the open
instance; reopen the same config/key and original journal.

An exported request without its signature remains unresolved even after era
death. The signature may already exist outside this process; missing bytes are
not evidence that no signature was issued. Completed marker plus missing,
truncated, foreign or symlinked state fails closed.

Mode alone is not a transaction receipt. Successful dispatch plus inclusion-block
Recycle, the original subnet registration and the owner's rate record at the
inclusion height qualify the transition's observed state. A same-block later
Burn or changed registration is a state conflict. A failed mode read keeps the
financial receipt in `finalized-readback-pending`; a later read can enrich only
that original receipt. Another account's nonce consumption is a conflict; expiry
requires complete canonical absence and an unchanged available nonce.

Every result keeps `activation_ready: false`. This setter chooses disposal mode;
it is **not a 10% payout setter**, does not construct weights and does not prove
Yuma outcomes. Repeat the existing mode gate at actual activation/recovery and
complete the signed economic policy, owner-hotkey census, drain boundary and
native interval 10/90 qualification. No alternate burning policy is authorized.

## Qualification scope

The [current metadata fixture](testdata/runtime470-recycle-codec.md) contains only
protocol schema; account/state/key material is synthetic. Tests cover typed native
Sr25519/Ed25519 encoding, metadata/call/rate-key drift, default Burn and window
boundaries, independent portable trust, bounded inert Ledger transcripts, real
post-rename export/import failures, local exclusion and missing-state refusal,
public CLI round trips, Ledger response framing, complete canonical expiry,
foreign nonce, exact receipt/readback gaps and immutable recovery continuations.

Author evidence is retained under
`/mnt/data/sn-testnet/astra-owner-recycle-20261001/`. It includes the exact full
official 470 metadata qualification, ordinary/race/vet logs and a causal
readback-conflict counterfactual. Independent qualification remains separate.
The SN `6c801a25` / server `720e7c61` release cannot cover this source; selecting it
requires a successor release. No live owner signature, transaction or service
was used to qualify this implementation.

[setter]: https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/admin-utils/src/lib.rs#L1572-L1605
[origin]: https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/utils/misc.rs#L10-L109
[rate]: https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/utils/rate_limiting.rs#L37-L130
[scheduler]: https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/coinbase/run_coinbase.rs#L1231-L1238
