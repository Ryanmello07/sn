# Chain preparation and read-only readiness

`sn-mainnet bootstrap-chain plan/apply/resume` binds the retained owner-trim
review, two protected UR schema-3 configs, separately approved root-service
config, signed contract phase and signed root phase to one durable local
preparation. Apply prepares the existing reserve
CREATE custody owner and [root custody owners](BOOTSTRAP-ROOT.md). It does not
open an RPC connection, import a signature, issue a transaction or start a
service. These three preparation modes have no online or submission option.
The separate `readiness` mode observes current finalized role prerequisites
from an explicitly selected route after checking the original v3 custody.

This closes the missing composition and restart boundary between the separate
preparation commands. Every result retains `activation_ready: false`,
`network_effects: false` and the outstanding chain phases. A successful local
preparation does not establish a safe executed trim, full contract installation,
two eligible validators, a running root role or realized native economics.

## Read-only current prerequisites

After local preparation, run:

```sh
sn-mainnet bootstrap-chain readiness --config /private/chain.json \
  --run-dir /private/custody --accept-plan-hash sha256:REVIEWED_DIGEST \
  --rpc https://owned-rpc.example --retry-window 300s
```

The command reloads the original independently pinned approvals and requires
the same accepted v3 plan and run directory. It opens all five existing markers
read-only, takes nonblocking shared locks, and verifies the complete original
journals and their signature lineage. Missing or interrupted state is unresolved;
readiness never creates, repairs, signs, imports or advances it. Concurrent
custody writers must finish before observation starts. V1/v2 journals retain
their original resume behavior and cannot acquire v3 readiness scope.

One bounded census at one authenticated finalized block checks the approved
network, runtime code and metadata, subnet owner/generation, both UR hotkey,
coldkey and registration generations, current activity and validator permits,
signed native block windows, and the signed maximum subnet census count. The
separate netuid-0 role checks its exact approved uid and registration generation,
mortal action window and canonical era checkpoint. A canonical block recheck
after the checkpoint read detects a changed finalized mapping. Original trim
removal requests remain review evidence; completing these role checks does not
claim the old miners were removed or that the trim is executable.

Results use `urnetwork-mainnet-bootstrap-chain-readiness-v1`, bind the accepted
plan, the five retained journal hashes, the complete census envelope and distinct
majority/secondary/root blockers. `observed-prerequisites` means only the listed
read-only checks passed; `blocked` retains a complete observation with explicit
conflicts. An unavailable route, unsupported or changed runtime, incomplete
census or changed finalized mapping returns `unresolved`, with no partial role
success. HTTP 502 cannot become a retained-census fallback. Each invocation
observes again; it never reuses an earlier readiness result as authority.

`current_authority_verified`, `native_signing`, `network_effects` and
`activation_ready` remain false. Each role separately retains its missing
activation prerequisites. These include the UR native epoch window and signed
activation checkpoint, production admission/operator health, deployed contract
verification, effective stake majority, and signing-device/global custody
fencing. Root effective delegated stake, eligibility, nonce/weight requirements,
independent current authority and actual service activation remain unresolved.
All five original pending chain phases remain in every result.

Exit 0 means the bounded observed prerequisites passed. Exit 3 means an
observed blocker, integrity refusal or old-scope refusal; exit 1 means unresolved
transport/custody/cancellation or output failure; exit 2 means invalid flags,
route or independently approved input. Only completed input admission produces
the readiness result. A route supplies observations, never submission approval.

The September 29 source increment includes deterministic command/restart,
stale-generation, permit/activity, signed-window, checkpoint conflict, reorg,
route/runtime failure, missing-state and lock-release regressions. Its
[Sol qualification](evidence/bootstrap-readiness-qualification-20260929.md)
passes all ten new roots and the exact 148-root adjacent scope in normal/race
modes, plus vet and four causal controls. This qualifies the recorded source
and module graph; live eligibility and activation remain unresolved.

## Inputs and review

The strict JSON configuration has these fields:

| Field | Required value |
| --- | --- |
| `schema` | `urnetwork-mainnet-bootstrap-chain-config-v3` for new preparation |
| `deployment_id` | Same independently chosen deployment as both child plans |
| `netuid` | `25` |
| `network` | Independently provisioned `native_chain`, `genesis_hash`, `evm_chain_id: 964` |
| `run_directory` | Precreated canonical absolute owner-private directory shared by both children |
| `owner_trim_policy` | `{path, sha256}` for the exact original census policy |
| `owner_trim_plan` | `{path, sha256}` for the retained safe partial owner-trim plan |
| `contracts` | `{path, sha256}` for the signed [contract phase config](BOOTSTRAP-CONTRACTS.md) |
| `root` | `{path, sha256}` for the [root bootstrap config](BOOTSTRAP-ROOT.md) |
| `ur_validators` | Exactly two public role declarations described below |
| `root_validator` | Independent netuid-0 role, action/config approval keys and pinned config approval described below |

Each role declares `validator_id`, `hotkey_account_id`, `coldkey_account_id`,
`registration_block`, `config: {path, sha256}`, `role`, `implementation`, and
`approval_public_key_ed25519`. The roles are ordered `majority`, then
`secondary`; both declare `implementation: "sn/validator"`. Each independent
approval signer is a canonical nonzero `0x`-prefixed 32-byte Ed25519 public key.
The IDs, hotkeys, config
paths and config byte hashes must differ. Each exact registration generation
must appear under `ur-validator` in the retained trim policy's protection set
and in the selected plan's exact surviving generations.
The separate root hotkey cannot count as either UR validator or appear in the
requested trim removals. The planned reserve hotkey is also excluded from
requested removals. The root's exact UID, hotkey, coldkey and registration block
must match the retained excluded-root census.

V2 and v3 decode the exact pinned config bytes through the standard validator's
strict grammar and full initial schema-3 validation, then verifies the declared
content-addressed production approval and its domain-separated signature.
Signed hotkey, validator ID, network and deployment must match the independently
selected role. Runtime version, code/metadata and reviewed source must match
the trim and signed child scopes; the approved subnet owner must match the trim
policy. Both approvals include both protected UR hotkeys and agree on their
initial policy, runtime, census, activation window and contract declarations.
Distinct role custody namespaces cannot overlap other roles or bootstrap
inputs, journals or lock markers.

The accepted plan retains these public signed facts under
`ur_validator_config_inspections`. The result reports
`ur_validator_configs_verified: true` and
`two-signed-production-configs-verified-live-admission-pending`. This verifies
config admission only. The intended majority label does not prove current
stake, permit, key ownership, binary identity or a healthy running service.
Coldkey and registration generation are matched against the retained protected
census; the producer approval itself signs the hotkey, not its live generation.
Actual generation, current signed window, deployed contract addresses/code,
operator evidence and health, runtime capability and two live validators remain
pending. No credential or operator evidence contents are opened and no signature
is created. The later [production path](VALIDATOR-PRODUCTION-RUNTIME.md) must
perform its own current admission before starting a writer.

This initial-bootstrap check rejects runtime/authority history and schema-2
observation inputs. It reads the declared approval source on every invocation;
unlike an existing producer's restart loader, it cannot replace a missing or
changed source with retained-state approval bytes. Approved renewal/history
requires the existing producer workflow and is outside this composition.

The root action and contract approval signatures are verified through their
existing validators. Their exact network, runtime/source tuple, deployment and
state paths must agree with the preparation and retained trim policy.

## Separate root config admission

V3 requires this independently provisioned `root_validator` declaration:

| Field | Required value |
| --- | --- |
| `role`, `netuid` | `bittensor-root-validator`, explicit `0` |
| `implementation` | `sn/mainnet/root-service` |
| `hotkey_account_id`, `coldkey_account_id` | Canonical nonzero AccountId32 hex, matching the signed root action and retained root census |
| `seat` | Exact `{uid, registration_block}` for the existing netuid-0 generation |
| `strategy` | `explicit_root_weights`, the existing supported service strategy |
| `action_approval_public_key_ed25519` | Independent public key pin for the existing root action approval |
| `approval_public_key_ed25519` | Independent public key pin for full service-config approval |
| `approval` | `{path, sha256}` for the external public service-config approval |

Both keys are canonical nonzero `0x`-prefixed 32-byte Ed25519 public keys. The
action key must match the root service's custody trust. The full-config key
verifies the additional approval below; the two approval keys may differ. They
must come from the operator's approved authority through the independent chain
input. Neither key is learned from the inspected root service or approval file,
and arbitrary self-signed input does not establish organizational authority.

The strict approval file has schema
`urnetwork-mainnet-root-service-approval-v1`, `deployment_id`, `root_plan_hash`,
`service_config_hash` and `approval_signature_ed25519`. The child
`bootstrap plan` command produces the read-only root plan whose `content_hash`
is approved. `service_config_hash` is `sha256:` followed by SHA256 of canonical
Go JSON for that plan's entire `service` field. Signature bytes are 64-byte
lowercase hex without `0x`. Ed25519 signs the approval schema string, a zero
byte, and canonical Go JSON of the approval with an empty signature, in the
field order listed above. Approval issuance stays with the external approver;
the command only verifies public signatures.

The child plan seals exact config/service file bytes, the complete action and
custody trust, network/runtime/source, deployment, run directory and state paths.
The additional signature therefore covers the full observation allowance and
service configuration, which the original action signature alone does not
approve. Changing whitespace in a pinned child file requires a new approval as
well as a new accepted preparation hash. A signed replacement action key still
must match the independent role pin. A new valid config approval cannot adopt
or renew any already claimed custody.

The v3 plan retains `root_validator_config_inspection`, containing the full child
root plan and config approval. The result reports
`root_validator_config_verified: true` and
`root_validator_status: signed-root-service-config-verified-live-authority-pending`.
These facts authenticate offline configuration only. Current root eligibility,
effective stake, seat retention, delegation and basket rights, source-to-Wasm
authority, live route, global custody fencing, fee exposure, key possession and
the actual service/binary remain unproved. No service loop or current-authority
adapter is attached. The root role remains separate from both UR validators.

V3 supports the existing explicit-root-weight service only. The read-only
observer's `accumulate_in_place` policy remains a separate workflow and requires
no periodic native transaction. This preparation does not change that policy.

## Exact offline inputs

The trim selection is rebuilt from the retained census to reject a resealed
different removal set. This is offline internal consistency, not authenticated
live state or an execution-time invariant. All original execution blockers and
the original plan hash remain in the preparation record. The existing owned
route recheck/qualification and actual safe execution are still required.

All inputs and transitive root-service/contract-artifact references retain
their exact byte pins. Each input must be a bounded owner-private regular file
with a physical owner-private parent directory; canonical absolute paths must
not traverse symlinks. The existing loaders enforce their bounds: chain/root
configs 1 MiB, contract/UR configs 2 MiB, UR production approvals 64 KiB, root
service-config approvals 16 KiB, retained trim plans 32 MiB. The
contract artifact catalog retains its existing separate bound. The same bytes
that pass a pin are decoded, without reopening root or validator config paths.
The resulting preparation plan, including its full root inspection, is bounded
to 512 KiB before any journal opens. The root approval file participates in all
input/journal and UR custody namespace separation checks. Every invocation reads
its exact source bytes; retained progress never replaces a missing approval.

```sh
sn-mainnet bootstrap-chain plan --config /secure/ur-mainnet/chain-preparation.json
sn-mainnet bootstrap-chain apply --config /secure/ur-mainnet/chain-preparation.json --run-dir /secure/ur-mainnet/run --accept-plan-hash "$CHAIN_PREPARATION_HASH"
sn-mainnet bootstrap-chain resume --config /secure/ur-mainnet/chain-preparation.json --run-dir /secure/ur-mainnet/run --accept-plan-hash "$CHAIN_PREPARATION_HASH"
```

Plan is read-only and deterministic. The dedicated
`urnetwork-mainnet-bootstrap-chain-preparation-v3` seal is SHA256 over that
schema, a zero byte and canonical Go JSON with an empty `content_hash`.
Neither the blocked review graph hash nor an individual child plan hash is
accepted as the local composition confirmation.

Existing v1 and v2 configs and journals remain readable and resumable under their
original domains and exact hashes. V1 role config bytes remain opaque and its
result continues to say
`two-protected-role-inputs-pinned-production-admission-pending`; no inspection
facts or verified flag are added. V2 retains its two verified UR configs and
original v2 result, without root-role inspection or verified fields. Neither old
schema accepts a root-role declaration or acquires root-config authority.
New `apply` requires v3. A new v3 plan cannot adopt or upgrade already claimed
v1/v2 custody, even with a newly accepted hash.

## Ownership and recovery

The shared run directory has five distinct journals and their `.lock` markers:
`bootstrap-chain.json`, `reserve-create.json`, `bootstrap-root.json`, and the
exact approved root custody/service state paths. Input files cannot alias any
journal, lock marker or another input. Apply requires all five destinations and
markers unused. It does not adopt an existing independent deployment under a
new preparation; retain those original child commands when custody already
exists without this parent journal.

The chain journal retains the full preparation plan, then advances from
`claimed` to `contracts-retained` to `prepared`. Those are local milestones,
not chain phases. Atomic file replacement and file/directory sync precede
acknowledgement; one local exclusive lock serializes the composition. Child
owners close before their progress is acknowledged. A failed publication
requires reopening the preparation owner.

Resume reloads every pinned input before opening the journal. It reopens and
checks real child state even after local completion. A child that completed
before its parent progress was published is reconciled in place. A missing or
invalid completed journal/marker is refused; lost custody cannot become an
unsigned fresh allowance. The one pre-child recovery case permits a complete
accepted-plan marker and absent/initial progress only while every child file
and marker is absent. Partial markers, advanced progress and ambiguous child
state fail closed.

The existing child commands remain the public signature-import interfaces.
Use their original child hashes and approved configs after this preparation;
chain resume then reports their exact retained signatures and consumed
allowances. It does not rewrite an approval, refresh an era or nonce, replenish
a limit, or rebroadcast. Observed signature hashes are consistency checks;
child journals remain authoritative. Local locking does not establish a
distributed custody fence or protect against hostile-host rollback.

Exit 0 means that the requested local plan/result was emitted. Malformed or
stale inputs exit 2; an unaccepted plan or refused retained ownership exits 3;
phase, cancellation, durability and output failures exit 1. Keep all journals
and markers after any failure.

## Qualification scope

V3 adds deterministic regressions for independently pinned root action/config
approvers, changed service allowances, exact approval domain and full child
scope, malformed/missing role inputs, stale or unavailable approval sources,
custody/input namespace overlap and refusal to renew existing custody. An
independent pre-v3 wire shape checks exact v2 canonical bytes, hash and result
scope; v1 recovery retains its existing compatibility check. Its separate
[v3 qualification](evidence/root-role-admission-qualification-20260928.md)
records 403 full normal roots and all 38 bootstrap-chain race roots. The
earlier v1/v2 receipts below retain their original narrower scope.

V2 adds deterministic controls for real signed two-role admission, absent or
wrong-domain signatures, independent signer/role/runtime/source disagreement,
stale approval bytes, reapproved-config restart refusal, namespace overlap and
exact v1 recovery. These additions and the shared validator decode extraction
have their own [Sol qualification](evidence/ur-bootstrap-admission-qualification-20260928.md):
all 588 root and 183 descendant executions pass on the frozen composed graph,
with six causal families in normal/race modes. Its retained refusals remain
separate from accepted evidence; every live chain/service gate stays open.
The following earlier receipt covers v1.

Deterministic tests cover real command dispatch and child custody owners,
original signature preservation, interrupted claim/child/publication boundaries,
lost output, exclusive ownership, missing completed journals, stale transitive
inputs, duplicate/root-conflicting roles and resealed trim selection. The
byte-based root-plan regression replaces its pathname between read and decode
and checks both the pinned result and the distinct reopened control. Sol medium's
[qualification receipt](evidence/bootstrap-chain-qualification-20260928.md)
records 369 full normal roots, all 87 adjacent race roots across six disjoint
shards, and four passing fixed controls with four intended mutant failures. It
retains the original aggregate race timeout and identifies the actual tested
dependency graph. These synthetic local results do not qualify a different
composed release or supply any live launch gate.
