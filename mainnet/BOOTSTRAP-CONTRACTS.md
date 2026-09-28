# Executable contract bootstrap: reserve CREATE

`bootstrap-contracts preview/plan/apply/resume` implements the first installation action:
an exact `STReserveSink` CREATE through retained public EVM custody and the owned
HTTP submission adapter. It does not install the remaining contracts, anchor
evidence, register miners, change emissions, activate validators, or complete
mainnet bootstrap. The candidate's test bodies await the separate Terra lane;
see [qualification scope](evidence/bootstrap-contract-create-20260928.md).

The release catalog comes from the existing generator:

```sh
go run ./sim-testnet/gencontracts --release-json evm/out sim-testnet/contracts_gen.go /secure/ur-mainnet/contracts.json
```

This additive export keeps the established source/layout/retained-bytecode
checks, emits only the five production artifacts, and does not compile Solidity
or modify the committed bindings/catalog. `STValidatorEvidence` is included;
simulator adversaries, probes and fleet helpers are excluded. The independently
signed phase pins the exact exported file. The reserve builder independently
recreates its constructor, predicted CREATE address, address-derived mapped
coldkey, semantic immutable words and five constructor getter results.

## Approval and custody

The `urnetwork-mainnet-contract-phase-config-v1` config contains an independently
provisioned Ed25519 approval key, the complete typed plan, and its signature over
`urnetwork-mainnet-contract-phase-approval-v1` followed by a zero byte and the
canonical Go JSON plan. The config/key must come from the trusted approval
channel; a key supplied by an untrusted journal or node is not independent
authority. `plan` verifies the signature and exact release file before emitting
the plan hash. `apply` and `resume` require that same accepted hash and directory.

`approval_signature_ed25519` is exactly **128 unprefixed lowercase hexadecimal
characters** encoding the 64-byte signature. The public-key field is separately
encoded as `0x` plus 64 lowercase hexadecimal characters. Prefixing the signature
with `0x`, uppercasing it, or accepting another spelling is invalid. This uses the
existing root custody signature parser without changing its wire contract.

`preview` accepts the same configuration with an empty
`approval_signature_ed25519`. It validates the finite graph, public-key format,
route syntax, amount bounds, exact artifact file and reserve constructor, then
exports the typed plan, plan hash, predicted address/runtime hash, and exact
domain-separated approval bytes as `approval_signing_message_hex` (unprefixed
lowercase hexadecimal). `approval_signing_message_sha256` identifies those
bytes. The exported `approval_verified` value is always false. A supplied
signature is rejected by preview; `plan` is the signed approval review command.

Preview reads only the draft and artifact files. It does not inspect or create
the future run directory, acquire its journal lock, load a key, or open a network
route. A valid preview does not authenticate externally supplied runtime,
custody, cutover or network claims. Provide those exact inputs through the
independent approval workflow; no live values are inferred by this command.

Prepare the bounded draft, review the exported plan and exact payload, then
have the independent approval signer sign the **decoded message bytes**, not
the SHA-256 digest or the text containing hexadecimal characters:

```sh
umask 077
sn-mainnet bootstrap-contracts preview --config /secure/ur-mainnet/contract-phase.unsigned.json > /secure/ur-mainnet/contract-phase.preview.json
jq -r '.approval_signing_message_hex' /secure/ur-mainnet/contract-phase.preview.json | xxd -r -p > /secure/ur-mainnet/contract-phase.approval.bin
```

Put the returned 64-byte signature's canonical 128-character encoding into the
same config's `approval_signature_ed25519` field. Preserve the exact typed plan
and independently provisioned public key. JSON whitespace may change; any
plan-field change requires a fresh review and signature. `plan`, `apply` and
`resume` all verify this signature before any journal or RPC owner can open.
Preview accepts no execution, signed-transaction, run-directory or accepted-hash
flags. Its successful exit only means local structural/artifact review passed.

The phase binds the mainnet EVM chain ID 964, independently approved native chain
and genesis, exact runtime version/code/metadata and inspected source commit,
native starting checkpoint and finite send window, fixed owned IP/port route,
source-lock/cutover/custody-fence evidence hashes, custody identifier, artifact
file, journal directory, finite attempts, total value plus maximum gas liability,
and every prepared action's sender, nonce, target, calldata, value, gas and fees.
HTTPS also requires normal certificate validation and the approved SPKI pin.
No DNS, proxy, redirect, endpoint fallback or automatic write retry is admitted.

The bounded graph may contain the ordered prefix of one through nine actions.
One approval covers all supplied action reservations; no per-step confirmation
is invented. Only action zero is executable in this candidate. Later actions
remain sealed reservations, not claims that their semantic builders or Safe
executor exist. Extending a prefix changes authority and cannot silently amend
an existing journal. The complete later phase must prepare all nine actions
before its single graph approval if it is to progress without a new approval.

Only empty-access-list EIP-1559 envelopes are admitted here. A separate signer
returns the original **binary signed transaction**, not a key or a signing
callback. Import verifies canonical bytes, recovered sender, chain and every
approved unsigned field. Subsequent imports cannot substitute another signature.
Offline `apply/resume` perform no RPC reads or writes and request no private key.

```sh
sn-mainnet bootstrap-contracts plan --config /secure/ur-mainnet/contract-phase.json
sn-mainnet bootstrap-contracts apply --config /secure/ur-mainnet/contract-phase.json --run-dir /secure/ur-mainnet/run --accept-plan-hash "$CONTRACT_PHASE_HASH"
sn-mainnet bootstrap-contracts resume --config /secure/ur-mainnet/contract-phase.json --run-dir /secure/ur-mainnet/run --accept-plan-hash "$CONTRACT_PHASE_HASH" --signed-transaction /secure/ur-mainnet/reserve.signed.bin --signed-transaction-hash "$SIGNED_FILE_HASH"
```

The result distinguishes `signature-awaiting-import`, `signed-custody-complete`,
uncertain/pending chain work, `reserve-created`, and a reverted CREATE with its
nonce consumed. `installation_complete` and `activation_ready` remain false.
An offline reopen preserves either completed status and the original receipt,
with `receipt_observation: "retained"`. A successful online receipt audit emits
`receipt_observation: "revalidated-online"`. Retained completion is historical
local evidence; it does not claim a new observation of canonical chain state.
An online observation requires explicit `--online` on `resume`; adding `--submit`
permits at most one originally approved attempt after reconciliation and fresh
admission. The command never loads a key, generates a transaction replacement,
changes nonce, reprices fees, or redeploys automatically.

## Durable recovery and canonical postconditions

`reserve-create.json` is authoritative for the original signed bytes/hash, finite
attempt count, scanned native checkpoint and completed receipt. Its private
regular-file marker is held with a local flock. Atomic publication syncs file
and parent directory before acknowledging state. A failed publication poisons
the owner; reopen reloads the actual complete record. Only the exact unfinished
initial marker with absent or untouched unsigned state can recover. A completed
marker with a missing/corrupt journal cannot become a new allowance.

Reconciliation always precedes submission. A durable attempt is consumed before
the single HTTP write, including an interrupted or uncertain write. Later sends
use the same signed bytes and remaining attempt count. A pending or consumed
nonce without the canonical receipt remains unresolved; it never authorizes a
new nonce. Missing CLI output is recovered from the child journal.

For a receipt, the adapter authenticates its required fields, gas/fee bounds,
original transaction at the reported block/index, native header ancestry,
Frontier digest/raw EVM header/canonical EVM lookup, and the exact native
`Ethereum.BlockHash` first insertion using the independently approved runtime
at both execution and parent. Native and EVM heights are distinct. A successful
CREATE additionally requires byte-for-byte patched release runtime and all five
constructor getters at the canonical inclusion hash. Status 1 alone cannot
complete the phase. Every online completed resume rechecks the receipt.

Each invocation scans at most 128 new native headers. Completed chunks are
durable; an interrupted current chunk is repeated, at most 128 headers. A failed
candidate read cannot advance past that candidate. Archive unavailability
remains a read error, not evidence of a successful-value mismatch. Healthy head
advancement refreshes current runtime/nonce/balance/collision admission within
the same bounded operation rather than requiring a stationary chain. Changed
ancestry, runtime or custody observations still stop the write.

## Remaining live trust and graph work

The local flock is not a distributed deployer-key fence. Signed hashes attest
externally reviewed evidence; this command does not independently prove the
cutover, global custody exclusivity, source-to-Wasm provenance or ongoing runtime
governance freeze. Owned RPC finality/state remain assertions, not GRANDPA or
storage proofs. All live identity, route, funding, runtime and custody approvals
remain required inputs. No live signing, RPC write or deployment qualified this
candidate.

The [offline recovery and approval-preview follow-up](evidence/bootstrap-contract-preview-20260928.md)
has separate command regressions and pending Terra qualification. Its local
signature fixture does not supply live approval authority.

EVM signatures have **no native block expiry**. The finite native window only
stops new local submissions. Retained bytes remain a liability until a canonical
nonce outcome is reconciled; approval expiry must never release that reservation.
A later unrelated runtime upgrade does not prevent authenticating an inclusion
under the originally approved historical runtime. Inclusion under an unapproved
execution/parent runtime remains unresolved and needs separately qualified
authority migration, not a fresh journal or an inferred compatibility waiver.

The remaining graph is vault CREATE, coordinator implementation CREATE,
`registerEscrow` with bounded rao-to-wei value, atomic initialized proxy CREATE,
reserve recorder binding, vault coordinator binding, `STValidatorEvidence`
CREATE, and the separately authorized Safe `fixValidatorEvidence` call. The
last action needs exact Safe digest/signature/nonce ownership and the outer
relayer's distinct nonce/fee reservation. Both the Safe inner success and the
canonical coordinator getter must agree; outer EVM status 1 is insufficient.
Do not treat the existing `Deploy.s.sol` sequence as complete evidence anchoring.
