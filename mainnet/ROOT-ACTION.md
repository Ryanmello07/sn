# Existing-seat root action owner

The root role now has an offline-qualified **single-action ownership core** in
[root_action.go](root_action.go), a private durable store in
[root_action_store.go](root_action_store.go), and a bounded root basket-call
encoder in [root_signing.go](root_signing.go). There is no production signer,
authority adapter, canonical receipt adapter, `root-service` command or active
root weight publisher. This increment does not claim mainnet readiness.

The current proposed strategy remains `accumulate_in_place`: a retained root
seat requires **no periodic native transaction** for that strategy. The observer
continues to monitor membership and custody. There is no manufactured heartbeat,
automatic root registration, staking, re-registration, delegation, claim or
take change. `set_root_weights` changes the basket strategy and requires its own
explicit approval and complete eligibility qualification. The user's subnet
90% owner-recycle decision supplies no root basket or custody authority.

## Implemented boundary

One action artifact has `schema: urnetwork-mainnet-root-action-v1` and binds:

- The separate `bittensor-root-validator` role, netuid 0, native chain,
  independently approved nonzero genesis and EVM chain ID 964. ID 945 is rejected.
- Full runtime version, `:code` and raw metadata hashes, plus the inspected
  source profile `67dcf7f791dc495064c293f080a0702cb433e51e`.
- Exact hotkey, coldkey, existing UID and registration block. No observation or
  hardcoded slot is treated as proof that an approved seat exists.
- Independent policy and approval artifact hashes, custody ID, canonical private
  state-file path, and the separately selected `explicit_root_weights` strategy.
- Sorted unique destination netuids, positive u16 weights, one nonce, a finalized
  mortal anchor, a period of 4–256 blocks, one finite approval expiry, one local
  fee reservation, and at most eight same-byte broadcast attempts.

The profile checks metadata14/extrinsic4, the selected AccountId32 and sr25519
variants, the complete signed-extension order and consumed wire shapes, and the
root call's two `Vec<u16>` parameters. The inspected payment wrapper is explicit;
unknown or reordered extensions are refused. No UR scoring vector can be sent
through this core without a separately approved root action artifact.

Every action is one-shot for its entire lifetime. Dispatch failure, expiry or
an exhausted broadcast count does not reset the signature/count allowance. A
future action needs a new approved artifact and authoritative hotkey/nonce
ownership reconciliation. Automatically opening another state directory is not
a legitimate way to obtain another allowance.

The encoder uses a mortal Substrate era and zero tip. All allowed periods use
unit quantization, so the supplied finalized anchor is the exact era birth.
Payloads longer than 256 bytes use Substrate's Blake2b-256 rule before sr25519
verification. The core verifies the returned public signature and builds only
the exact retained call, nonce, era and signer bytes. Existing immortal UR/native
helpers remain separate; their recovery assumptions are unchanged.

The local policy/runtime hashes bind the request identity and admission checks.
They are **not additional fields in the native signed payload**. The protocol
signature binds genesis, spec/transaction versions and mortal checkpoint; a
same-version code replacement is not cryptographically excluded by that payload.
The metadata-hash extension is explicitly disabled in this profile. Production
must assess that residual runtime-upgrade exposure rather than claim that a
local exact-code check is an on-chain lock.

## Durable transitions and restart behavior

| Retained phase | Permitted next work | Retained obligation |
| --- | --- | --- |
| `reserved` | Authenticate current chain, generation, nonce and complete independent authority; persist `signing` before custody | One exact action and its local fee reservation |
| `signing` | Recover custody's original signature; an authoritative never-issued response may re-enter current admission before `signOnce` | Same request ID, nonce and allowance; no blind replacement signature |
| `signed` | Reconcile complete finalized history before any submission | Exact signed bytes and transaction hash |
| `pending` | Reconcile first, then optionally retry identical bytes within count/era bounds | Ambiguous sends keep ownership and reservation |
| `finalized`, `dispatch-failed`, `fee-overrun`, `runtime-deviation` | Retain exact receipt, dispatch, execution-runtime and actual-fee evidence | Completed action consumes this one-shot artifact permanently |
| `expired` | Retain complete finalized absence through the mortal window and unchanged account nonce | No implicit new action or signature allowance |

Each `step` makes at most one custody signing/recovery operation or submission;
there is no internal unbounded sender loop. Reconciliation precedes every
broadcast. Its caller owns the finite retry schedule and context. RPC timeouts,
dropped/usurped pool notifications and lost acknowledgements are not terminal
dispatch outcomes. The production chain port must use bounded read retry of at
least 60 seconds, normally 300 seconds, and join its workers on cancellation.

If custody reports an unknown outcome, the action stays pending. Only a fenced,
authoritative custody response that this request has **never** issued a signature
may authorize another `signOnce` invocation, after fresh domain/eligibility checks.
A generic not-found, missing file or timeout is not that proof. The custody
service must itself be idempotent for the request ID, including concurrent or
lost-response calls, and must retain its signature receipt before replying.

Signed intent and broadcast count are saved before their external effect. A
durability error poisons the open owner: it cannot perform another side effect
until reopen determines which complete record survived. A post-rename error may
already be durable and is never treated as a safe rollback. A crash after a
broadcast-intent write may consume an attempt without sending; that conservative
count does not create a new signature or lose subsequent receipt reconciliation.

A runtime/seat/nonce change blocks new signing and broadcast but leaves old
receipt reconciliation available. A genuine receipt from before an upgrade is
retained under its original execution runtime. A transaction executed under a
different code, metadata or full runtime version is stored as `runtime-deviation`
with its actual result and fee; the observer cannot erase that financial event.

Expiry requires finalized coverage from birth+1 through death−1, revalidation
of the original anchor, a finalized head at or beyond death, and an unchanged
hotkey nonce. A wall clock, best head, missing subscription notification or
unexplained consumed nonce cannot release the reservation. If the signature
itself remains unavailable from custody, this increment conservatively retains
the unresolved action; automated unsigned-intent retirement is not implemented.

The store requires a precreated private directory without symlink traversal,
private regular files, an exclusive process lock, strict bounded JSON and content
hash checks. An immutable local marker binds the original request. Explicit
creation refuses an existing marker even when the record is missing or empty.
Atomic writes sync the file, rename it and sync its containing directory. No
completed or partial record is interpreted as an empty unused allowance.

This is local ownership, not distributed custody security. A checksum can be
recomputed; a hostile host can replace both marker and state or restore an old
directory. Custody must durably fence the whole hotkey across hosts, enforce
request/count limits independently and reject rollback/replayed authorization.
The local store is not a hardware signer or an independently authenticated
transaction log.

## Production adapter contracts and remaining gates

The core's three private interfaces have **no production implementation**. They
cannot be activated by a policy boolean, a successful `root-preview`, or a
testnet allowance. Before adding a signing CLI/service, supply and qualify:

1. **Independent mainnet authority.** Approved genesis, owned route, exact
   metadata/code/full runtime and source-to-Wasm provenance; approved existing
   root hotkey/coldkey, registration receipt/generation, policy, operator and
   custody identity, finite mainnet limits, signer role and state ownership.
   The Snow observation must not approve its own genesis. No actual mainnet pins
   or keys are included here.
2. **Action and eligibility authority.** An explicitly selected root basket
   strategy and full effective eligibility at a finalized view: current seat,
   root enablement, rate limit, owner exception, inherited parent/child stake,
   fixed-point TAO weighting, live destinations, minimum diversity and
   concentration. Raw stake and the observer's `read_only_ready` are insufficient.
   Preserve existing basket/delegation and all-staker rights before an economic
   transition; current-network census does not settle retired-network history.
3. **Protected custody and cost.** Approve a globally fenced hotkey signing
   service with durable idempotent receipts, one nonce lane, rollback protection,
   bounded request/count/expiry and recovery credentials. Online observers do not
   receive coldkeys. The local `fee_reserve_rao` is accounting only: native zero
   tip and a fee quote are not a hard maximum-fee argument. Qualify the actual
   payer, failure fees and enforceable exposure mechanism or obtain explicit
   approval of the bounded residual fee exposure. Do not label this implementation
   max-fee protected. Fresh registration remains separately blocked because the
   inspected root-registration call has no maximum-burn argument.
4. **Canonical chain/receipt adapter.** Read correct runtime-specific native
   account layout and pending nonce custody. Authenticate finality/ancestry,
   complete block bodies and extrinsic roots, exact signed bytes/index, dispatch
   and actual fee events, execution runtime from the proper parent state, and
   root-weight state readback. Scan the entire mortal interval with no skipped
   blocks; a failed read is never absence. The owner checks returned coverage and
   correspondence but does not verify storage tries or consensus itself. EVM
   receipts and UR validator evidence do not replace native root receipts.
5. **Supervisor and operations.** A finite action queue with its own globally
   reserved limits, bounded cadence/backoff, cancellation/join, alerts and named
   on-call ownership. Reconcile old actions when admission changes; never loop
   around a blocked action by changing directories, nonce, era or approval hash.
   Rehearse actual signer crash/host failover, lost responses, same-version code
   change and archive loss using the exact release before activation.

The tested core is a concrete step toward running the separately requested root
role. MG-08 remains blocked on these production adapters and approvals. UR
validators, root membership and substrate administrative Root origin remain
three distinct authorities.

## Qualification scope

`root_action_test.go` uses real generated test-only sr25519 keys, a public metadata
fixture explicitly adapted to a synthetic payment-wrapper profile, private
disposable files, deterministic durability failures and in-memory authority/chain
ports. Tests cover signature/domain replay, mortality, extension/shape drift,
missing authority, lost signer/send responses, both sides of durable writes,
exact-byte retry, fee/dispatch/runtime deviations, expiry gaps, unexpected nonce
consumption, stale seat generation and runtime, finality rollback, local lock
ownership and missing/empty/rehashed state. There is no live RPC or transaction.

Run normal/race/vet qualification with the composed workspace source lock:

```
go test ./mainnet -count=1
go test -race ./mainnet -count=1
go vet ./mainnet
```

Source semantics inspected at the pinned commit: `runtime/src/lib.rs`
(`SystemTxExtension`, `CustomTxExtension`, `TxExtension`),
`runtime/src/check_mortality.rs`, `runtime/src/check_nonce.rs`,
`runtime/src/transaction_payment_wrapper.rs`, `runtime/src/fee_filters.rs`, and
`pallets/subtensor/src/macros/dispatches.rs` (`set_root_weights`). Their local
inspection is not evidence that current mainnet runs those bytes.
