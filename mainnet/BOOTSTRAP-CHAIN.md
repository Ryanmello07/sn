# Offline chain preparation

`sn-mainnet bootstrap-chain plan/apply/resume` binds the retained owner-trim
review, two protected UR role inputs, signed contract phase and signed root
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
| `schema` | `urnetwork-mainnet-bootstrap-chain-config-v1` |
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
`registration_block`, and `config: {path, sha256}`. The IDs, hotkeys, config
paths and config byte hashes must differ. Each exact registration generation
must appear under `ur-validator` in the retained trim policy's protection set
and in the selected plan's exact surviving generations.
The separate root hotkey cannot count as either UR validator or appear in the
requested trim removals. The planned reserve hotkey is also excluded from
requested removals. The root's exact UID, hotkey, coldkey and registration block
must match the retained excluded-root census.

UR config files are bounded, byte-pinned inputs only. This command does not
parse their producer semantics, verify their credentials or sign production
approvals. Their declared role identities still need to agree with the actual
standard schema-3 validator configs at [production admission](VALIDATOR-PRODUCTION-RUNTIME.md).
The result explicitly reports
`two-protected-role-inputs-pinned-production-admission-pending`; arbitrary or
incomplete config bytes cannot earn service admission here. Permit/effective
stake, two healthy operators and initial production readiness remain pending.

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
configs 1 MiB, contract/UR configs 2 MiB, retained trim plans 32 MiB. The
contract artifact catalog retains its existing separate bound. The same bytes
that pass a pin are decoded, without reopening the root config pathname.
The resulting preparation plan is bounded to 512 KiB before any journal opens.

```sh
sn-mainnet bootstrap-chain plan --config /secure/ur-mainnet/chain-preparation.json
sn-mainnet bootstrap-chain apply --config /secure/ur-mainnet/chain-preparation.json --run-dir /secure/ur-mainnet/run --accept-plan-hash "$CHAIN_PREPARATION_HASH"
sn-mainnet bootstrap-chain resume --config /secure/ur-mainnet/chain-preparation.json --run-dir /secure/ur-mainnet/run --accept-plan-hash "$CHAIN_PREPARATION_HASH"
```

Plan is read-only and deterministic. The dedicated
`urnetwork-mainnet-bootstrap-chain-preparation-v1` seal is SHA256 over that
schema, a zero byte and canonical Go JSON with an empty `content_hash`.
Neither the blocked review graph hash nor an individual child plan hash is
accepted as the local composition confirmation.

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

Deterministic tests cover real command dispatch and child custody owners,
original signature preservation, interrupted claim/child/publication boundaries,
lost output, exclusive ownership, missing completed journals, stale transitive
inputs, duplicate/root-conflicting roles and resealed trim selection. The
byte-based root-plan regression replaces its pathname between read and decode
and checks both the pinned result and the distinct reopened control. These are
synthetic local fixtures. Test execution and causal qualification are assigned
to the separate Sol test owner; this implementation does not claim those gates
have passed before their retained results exist.
