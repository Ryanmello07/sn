# Production owner-recycle transition

The standard validator's V2 `RunRelease` path now accepts an independently
approved production configuration. It measures the unchanged provider workload,
derives a 10% provider / 90% recognized-owner weight row, prepares the real CRv4
atomic source-and-weights transaction, and retains a separately signed production
sidecar in the existing durable intent. This is an initial producer and restart
path for the same approved configuration. No mainnet identity, key, approval,
transaction or economic outcome was supplied by this implementation.
Local deterministic qualification is recorded in the
[producer evidence](evidence/owner-recycle-production-qualification-20260927.md)
and [runtime evidence](evidence/production-runtime-qualification-20260927.md).

The old observation approvals and unsigned v1/v2 measured capsules remain
read-only. They cannot select this path. The new schema-3 config requires the
separate v2 production approval and signature domain described in
[production runtime admission](VALIDATOR-PRODUCTION-RUNTIME.md).

The external approver must select the complete resolved config and exact
runtime/source review, finite native block/epoch windows, actual validator
hotkey, sorted validator and recognized-owner hotkey sets, a bounded native
activity age, and `production.activation_native_hash`. The latter is the exact
canonical finalized block at `valid_from_native_block`; its subnet epoch must
equal `first_native_epoch` and `PendingServerEmission` must be zero. A later
decision retains that original activation instead of pretending later rewards
were earned under the new policy. The first durable intent must use that first
native epoch; a later epoch requires its authenticated production predecessor.
The validator still requires complete real
activation, operator API/key/payout and proof-history inputs through the existing
V2 startup. Production approval does not grant registration or staking authority.

Before a decision can be signed, actual native reads authenticate every approved
validator's forward/reverse registration, coldkey, weighted stake and permit.
Coldkeys must be distinct. Bounded `LastUpdate` or registration freshness allows
the first submission without requiring that same submission as a prerequisite.
This is evidence of native eligibility/activity, not a remote process heartbeat.
The minimum validator and operator counts, self/controlled masks and signed
weight cap remain unchanged. Exact canonical coordinator reads and full provider
proof replay authenticate operator, pool, binding, deposit and source-root facts.

The provider artifact and its V2 envelope retain their original bytes and
meaning. A bounded production proof records the original signed approval,
owner census, operator observations, validator eligibility, drain boundary and
derived row. A distinct native source-hash domain commits both that proof and
the provider bytes before preparation. A second sr25519 seal binds the proof,
provider envelope and exact prepared extrinsic. The existing atomic intent
writer retains all of this before any submission. The submission gate requires
a private grant for those exact prepared bytes, issued only after the real V2
intent verifier replays the sources and row; a signed config alone cannot send
an unchanged parent row. Pending recovery keeps the original signed bytes and
uses existing receipt, nonce, epoch and runtime checks.

Archives capture the additional owner/validator/drain native reads and preserve
the sidecar in the original intent. Production archive replay requires fresh
independent source observation as well as the provider proof and both signatures.
Advancing finality witnesses are not embedded as immutable decision facts, so
the same historical decision reproduces after the head advances or metadata
caches are lost. A different signed config cannot reinterpret this proof.

The measured row is the transaction's input, not proof of its final economic
effect. Finalized inclusion, reveal/application, Yuma incentives, actual native
miner allocation, recycled value and source-derived rounding tolerance remain
monitored postconditions. Do not report the requested 10% native-miner outcome
before those observations exist. There is no reserve credit from recycling.

## Next transition: durable original production authority

Runtime artifact history is insufficient to migrate an already-running
production config: old sidecars name the old complete config and approval.
The current implementation deliberately refuses their reinterpretation under
a new approval, and its fixed retained approval cannot be overwritten.

The next fix must retain bounded, content-addressed bundles of each original
normalized schema-3 config and its signed approval. A new externally approved
config must select an append-only finite history of those exact bundle hashes,
with explicit predecessor links and nonoverlapping activation domains. Select
historical authority by the sidecar's original approval/config identity, then
check its native block and runtime against that original approval and the
independently approved runtime history. A tuple-only runtime document cannot
grant economic or signing authority.

Migration must inventory and resolve every retained sidecar before opening a
writer; retain existing proof cursors and original pending bytes. Reconcile an
old pending transaction against its original receipt before allowing a new
decision, and make any replay authority explicit. Publish the new selected
authority durably without replacing the original bundles. Test restart and
cache eviction, source-file loss, missing/changed bundles, conflicting ranges,
same-runtime config changes, signature/domain substitution and uncertain sends.
This remaining gate is MG-04/RT-04 work; the initial producer qualification does
not claim automatic compatibility across arbitrary configuration changes.
