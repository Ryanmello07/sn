# Root service decision and intent owner

[root_service.go](root_service.go) supplies one bounded existing-seat lifecycle:
observe a separately approved root basket, retain its decision and original
native intent atomically, and drive the [action owner](ROOT-ACTION.md) through
independent custody, current-authority and submission ports. The finite `Run`
supervisor joins every operation before returning. This closes the missing
decision-to-action ownership layer; it does not provide an activated validator.

There is no `root-service` command, native secret loader, signing device
transport, production mutation-authority adapter, RPC submitter or deployment.
The canonical chain's existing `submit` method remains disabled. A successful
decision, adapter return or supervisor completion is never activation authority;
every service event keeps `activation_ready: false`.

## Approved existing-seat scope

The independently provisioned `rootServiceConfig` contains a verified
[offline custody packet and trust](ROOT-OFFLINE-CUSTODY.md), plus a lifetime
allowance of 1–10,000 observations. That packet binds one native mainnet action:
EVM chain ID **964**, exact genesis/runtime/source artifacts, owned root
hotkey/coldkey and generation, nonce, mortal checkpoint, positive basket, fee
reservation, bounded broadcast count and private action-state path. EVM945 and
unapproved routes remain inadmissible. Configuration is supplied independently;
the journal cannot approve its own replacement configuration or allowance.

This layer supports only an independently approved `explicit_root_weights`
action. The proposed `accumulate_in_place` strategy still needs no periodic
native transaction. Root weights allocate a basket over destination **netuids**;
they are not SN25 miner scores or a reset mechanism. The majority SN25 validator
continues to use the standard `sn/validator` binary and its evidence-based
weights. That role supplies neither root basket approval nor chain Root origin.

## Finalized decision

[root_weight_observer.go](root_weight_observer.go) extends the approved read-only
canonical chain with an exact-block weight view. It first authenticates the
finalized ancestry back to the approved mortal checkpoint, current runtime,
root ownership/generation and nonce through the existing receipt port. At that
same finalized hash it then reads a complete bounded `NetworksAdded` census,
the selected root UID's weights, last update, setter enablement, rate and cap.
Only entries whose stored boolean is true count toward the runtime's active
network count. Absent weight rows use the authenticated metadata default; absent
or short last-update vectors use the pinned runtime's zero fallback.

The whole operation has a 15-minute deadline, bounded RPC retries and joined
storage workers. It repeats canonical block and network checks after reading.
Any contradiction, incomplete census or interruption returns no weight view.
The typed view and storage-evidence digest are retained in the decision. The
digest is not a storage proof. The port still trusts the approved owned node for
finality and storage; it does not implement GRANDPA or storage-trie verification.

[root_weight_decision.go](root_weight_decision.go) compares the target with the
runtime's max-upscaled stored weights, accounting for vector order and ignored
zero entries. It returns `target-observed` for a stored match, otherwise a hold
for disabled setter, rate limit, destination count/existence or concentration
cap, or `intent` when these necessary checks pass. A stored match is only a
comparison of the observed vector; it is not current submission eligibility.
Both fixed-point normalization branches and positive rounding match the pinned
Rust source. Offline replay covers every input value for maxima 32768, 32769 and
65535, a total of 131,075 two-element vectors.

The reviewed source is Subtensor
`67dcf7f791dc495064c293f080a0702cb433e51e`:

- `pallets/subtensor/src/subnets/weights.rs:881–994`: root setter checks and
  max-upscaled storage; `1144–1163`: last-update rate rule.
- `pallets/subtensor/src/subnets/subnet.rs:55`: true active network enumeration;
  `pallets/subtensor/src/utils/misc.rs:293`: missing last-update slot is zero.
- `pallets/subtensor/src/epoch/math.rs:78–130`: max-upscale conversion, with
  `substrate-fixed` commit `d5f70362f2e05b5f33fb51cd7baa825323e4e6c5`.
- `pallets/subtensor/src/lib.rs:3373`: effective-stake/owner-UID eligibility,
  which this decision layer does **not** authenticate.

These are reviewed source semantics, not a live mainnet Wasm attestation.
Complete stake attribution, economic-transition approval and current custody
still belong to the separately qualified authority before every fresh effect.

## One durable owner

`openRootServiceStore(config, create)` uses the action's approved state path for
one composite journal. The journal contains configuration, observation count,
pending-observation marker, last complete decision and the complete original
action record. It persists a consumed observation attempt before contacting the
reader. An interruption consumes that attempt while preserving the last complete
decision; a restart cannot replenish the observation allowance.

An `intent` decision and its reserved native action are one atomic write. Only
then does the local phase become `active`. That phase describes local ownership,
not a running or authorized mainnet service. All subsequent steps reconcile the
same action; they never sample a replacement basket, nonce, era or generation.
Finalized success, dispatch failure, fee overrun, runtime deviation or supported
expiry are retained with service completion in the same atomic write. Completion
never creates another action allowance.

The private store uses strict bounded JSON, private regular files, no symlink
traversal, exclusive process locking, a one-shot configuration marker and synced
atomic replacement. An ambiguous write or integrity failure poisons the owner
until reopen. Missing, malformed or foreign state cannot become fresh state.
There is no implicit migration from a standalone action journal: retain and
reconcile that journal with its original action owner. Creating another path is
not permission to duplicate the same hotkey's nonce or spend allowance.

Operations serialize through context-aware ownership. Canceled waiters do not
enter; active synchronous ports must return/join before `step` or `Run` returns.
The externally driven supervisor takes 1–10,000 steps and a 1-second to 1-hour
cadence, stops on terminal/blocked state, and retains intent when publication
fails. It starts no detached worker. The caller joins all use before closing the
store. Local files/process locks do not supply hostile-host rollback resistance
or cross-host custody exclusion.

## Independent capability boundary

Observation, canonical reconciliation, current authority, native custody and
submission are separate objects. Missing authority, signer or submitter blocks
before a fresh signature or broadcast attempt is reserved. This does not block
lookup of an already issued signature or canonical reconciliation of an old
receipt: those use their original immutable request even after authority changes.
Local absence of a signature never becomes a custody-issued never-signed proof.

The signing bridge requires the exact durable `signing` intent. The submission
bridge requires exact verified signed bytes and their durable `pending` attempt.
Its independent `submitRoot` port receives the approved packet, service-config
hash, broadcast-attempt number and exact extrinsic/hash. A future transport must
authenticate its own approval, route, fence and attempt identity, and handle that
identity idempotently. Those fields correlate a request; they do not grant
authority or prove a custody device's behavior.

Production still needs qualified current effective eligibility, global hotkey
and nonce fencing, device-side durable idempotency/recovery, independently
admitted submission and owned RPC identity, and enforceable payment exposure.
The native signature does not bind registration generation, source/code hashes
or maximum fee. Finalized observations cannot prevent an inclusion-time seat
change or same-version upgrade. Pending-seat exclusion or authenticated incident
reconciliation, original-byte recovery and the fee/runtime limits in
[ROOT-ACTION.md](ROOT-ACTION.md) remain mandatory. New root registration and
protection are outside this existing-seat owner.

## Qualification

`root_service_test.go` and `root_weight_observer_test.go` use synthetic approval
and native keys, private local journals, local read-only HTTP fixtures and
explicit cancellation barriers. They cover durable decision-before-effects,
rehashed intent substitution, finalized rollback/fork, missing/denied ports,
lost signer acknowledgement, same-byte attempt limits, terminal fee evidence,
interrupted observation budgets, both sides of ambiguous writes, restart,
concurrent callers and rejected special/public/foreign state. The reader tests
cover metadata defaults, true network census and changed runtime/code/metadata,
network/finality and canceled final rechecks. Exact qualification is retained in
[the service owner evidence](evidence/root-service-owner-20260927.md). No live
key, RPC, transaction, activation or deployment is used.
