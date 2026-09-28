# Offline chain preparation

`sn-mainnet bootstrap-chain plan/apply/resume` binds the retained owner-trim
review, two protected UR schema-3 configs, signed contract phase and signed root
phase to one durable local preparation. Apply prepares the existing reserve
CREATE custody owner and [root custody owners](BOOTSTRAP-ROOT.md). It does not
open an RPC connection, import a signature, issue a transaction or start a
service. The command has no online or submission option.

This closes the missing composition and restart boundary between the separate
preparation commands. Every result retains `activation_ready: false`,
`network_effects: false` and the outstanding chain phases. A successful local
preparation does not establish a safe executed trim, full contract installation,
two eligible validators, a running root role or realized native economics.

## Inputs and review

The strict JSON configuration has these fields:

| Field | Required value |
| --- | --- |
| `schema` | `urnetwork-mainnet-bootstrap-chain-config-v2` for new preparation |
| `deployment_id` | Same independently chosen deployment as both child plans |
| `netuid` | `25` |
| `network` | Independently provisioned `native_chain`, `genesis_hash`, `evm_chain_id: 964` |
| `run_directory` | Precreated canonical absolute owner-private directory shared by both children |
| `owner_trim_policy` | `{path, sha256}` for the exact original census policy |
| `owner_trim_plan` | `{path, sha256}` for the retained safe partial owner-trim plan |
| `contracts` | `{path, sha256}` for the signed [contract phase config](BOOTSTRAP-CONTRACTS.md) |
| `root` | `{path, sha256}` for the [root bootstrap config](BOOTSTRAP-ROOT.md) |
| `ur_validators` | Exactly two public role declarations described below |

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

V2 decodes the exact pinned config bytes through the standard validator's
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

The root and contract approval signatures are verified through their existing
validators. Their exact network, runtime/source tuple, deployment and state
paths must agree with the preparation and retained trim policy. Approval keys
and expected chain identity must be independently provisioned; the command
cannot establish real authority from a supplied self-signed configuration.

The trim selection is rebuilt from the retained census to reject a resealed
different removal set. This is offline internal consistency, not authenticated
live state or an execution-time invariant. All original execution blockers and
the original plan hash remain in the preparation record. The existing owned
route recheck/qualification and actual safe execution are still required.

All inputs and transitive root-service/contract-artifact references retain
their exact byte pins. Each input must be a bounded owner-private regular file
with a physical owner-private parent directory; canonical absolute paths must
not traverse symlinks. The existing loaders enforce their bounds: chain/root
configs 1 MiB, contract/UR configs 2 MiB, production approvals 64 KiB, retained
trim plans 32 MiB. The
contract artifact catalog retains its existing separate bound. The same bytes
that pass a pin are decoded, without reopening root or validator config paths.
The resulting preparation plan is bounded to 512 KiB before any journal opens.

```sh
sn-mainnet bootstrap-chain plan --config /secure/ur-mainnet/chain-preparation.json
sn-mainnet bootstrap-chain apply --config /secure/ur-mainnet/chain-preparation.json --run-dir /secure/ur-mainnet/run --accept-plan-hash "$CHAIN_PREPARATION_HASH"
sn-mainnet bootstrap-chain resume --config /secure/ur-mainnet/chain-preparation.json --run-dir /secure/ur-mainnet/run --accept-plan-hash "$CHAIN_PREPARATION_HASH"
```

Plan is read-only and deterministic. The dedicated
`urnetwork-mainnet-bootstrap-chain-preparation-v2` seal is SHA256 over that
schema, a zero byte and canonical Go JSON with an empty `content_hash`.
Neither the blocked review graph hash nor an individual child plan hash is
accepted as the local composition confirmation.

Existing v1 configs and journals remain readable and resumable under their
original v1 domain and exact hash. Their role config bytes remain opaque and
their result continues to say
`two-protected-role-inputs-pinned-production-admission-pending`; no inspection
facts or verified flag are added. New `apply` requires v2. A new v2 plan cannot
adopt or upgrade already claimed v1 custody, even with a newly accepted hash.

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
