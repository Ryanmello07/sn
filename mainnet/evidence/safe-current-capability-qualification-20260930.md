# Safe current-policy native capability qualification

**Qualification pending.** This isolated note records the required matrix and
integration boundary. Independent behavioral results, causal controls and the
final evidence seal remain pending. Compile/vet success does not establish
behavioral qualification or approve the policy. Public submission remains closed.

Frozen source is `95a905d46eb8d0325851ab7f63f39262708d4aae`, tree
`62a67ba33b19b97703c3032e709aa950592572a6`, based on frozen custody source
`3f88a9484c639e999081cf8a2b8a8f2cf14ed536`. The implementation worktree is
`/home/by/urnetwork/sn-successor-safe-current-capability-owner-20260930`.
Custody qualification/integration finished first at shared commit
`7aa87dfa02c1c794759f1e1b00dfbcd961fe100a`, tree
`7c905e93782a6b8380e9af1bb8595f45a157b5b7`. Its
[sealed receipt](safe-current-custody-qualification-20260930.md) is retained
byte-for-byte. This documentation child
must preserve every non-Markdown byte of the exact capability source and all
earlier immutable qualification receipts.

## Concrete scope and remaining policy gates

The [capability](../BOOTSTRAP-SUCCESSOR-SAFE-CURRENT-CAPABILITY.md) connects
completed independently signed current-policy acceptance to the existing native
proof collector. Selection requires the exact complete retained runtime tip,
original custody and separate history/current capability domains. No proposal
file, acceptance signature or public flag installs the route. The public
constructor supplies route zero; `--submit` refuses before custody loading or
reservation. Public online `--safe-current-revision` import only retains local
authority and supports historical reconciliation.

The internal native route obtains a SCALE-authenticated header and complete
Safe account-storage-prefix proof, with exact runtime/proxy/singleton code and
native code-metadata binding. Actual orphan owner/module storage refuses. All
expensive historical, prefix-proof and artifact work precedes the last scoped
pending Safe/relayer nonce, authority, funding and exact-known-transaction checks.
Cheap identity, code hash and continuity checks then re-admit the native window.
The proof's exact block/hash/root and its pending observation hash remain
separate from any later admission head; moving finality cannot relabel evidence.
A durable counted reservation is followed by a second complete observation before
the single exact transport send. Ambiguous replies retain their counted attempt
and exact bytes for later canonical receipt reconciliation.

The original signed history statement stays retained and unproven. Current
proof does not satisfy its complete-history authenticator. Finality remains an
assertion by the original owned RPC. Its non-atomic pending calls cannot exclude
an unknown pending orphan or changes between calls; the separately accepted
policy owns that assumption together with all-signer/relayer cutover. Proof and
pending observations keep history, complete pending storage and independent send
authorization false. Explicit policy approval and qualified public-route
installation are separate P0 gates, as is automatic compatible runtime admission.
No live RPC, real signing or mainnet transaction belongs to this qualification.

## Author checks and independent proof fixture

Astra max owns implementation, debugging and fixes, formatting, compile-only
checks and vet. Sol medium owns behavioral tests and causal controls. Author
compile-only/vet pass on the exact candidate and all eight isolated mutation
patches; the author mutation worktree was restored clean. No author behavioral
test was run.

The unchanged SDK proof oracle hashes to
`b875b9eb1aa497233f68bb4cae02b4331320fcaeeaaefbefe867658f2aebaecd`.
All eleven prior proof roots are selected again, including the independent Rust
SDK commitment census, omitted intersecting branches, malformed proofs, hidden
owner/module mappings, extra storage, code/metadata/root/header substitutions,
canonical snapshot continuity and explicit pending limits. The new full command
fixture uses published Safe bytecode, the original eight-action local graph,
synthetic independent signatures and the SDK-cross-checked native trie builder.
It deterministically covers moving-head evidence, a true canonical mismatch,
actual hidden mappings, late Safe/relayer nonce and same-version artifact changes,
proof-time mutation after reservation, one exact lost-reply send and historical
public read-only recovery without changing custody, nonces or liabilities.

The handoff is `/tmp/safe-current-capability-handoff-20260930`. Its immutable
14-file `PAYLOAD.SHA256SUMS` hashes to
`fdfd7d2f2016fc3c1a6f6b0b10004db8e18e582228973b5d99e8272b09d718c8`.
The supplementary 32-file `SHA256SUMS` hashes to
`c616cce661873c70de836bdd9937cd1223c9fd393a25e90b022ef8ace57a424d`.
`CONTROLS.json` hashes to
`c1e3373b098d867c9d58a86b18120db8e6527a2de2a98410119948d3a20df92f`;
`TEST-PLAN.json` hashes to
`1a2e45a235881f8d5a53f36277700dc91d5cdef9ef4ad336e3dfd082c25ee0b8`;
`SOURCE-FENCE.json` hashes to
`01e4b2e78caa9b6d2d48d383e61b5fb6d03eafb3196663187b026a7a56f67cc9`.
The separate read-only author source audit for both custody and capability is
`/tmp/safe-current-custody-capability-source-audit-20260930.json`, SHA-256
`326067033a951705fceb97902512acc0750c8f64c0b6bbc0a6f96ba12cc74501`.
It verifies clean exact commits/trees, changed-file hashes, Go module graph,
all six local replacement commits/trees, SDK oracle and both handoff seals.
It is a source audit, not behavioral evidence.

## Independent matrix awaiting final seal

Required positives are one new light route root and one new full-graph command
root, thirty-four adjacent light roots and one existing canonical adapter heavy
root, each normal and race: 74 root executions when complete. The light and
heavy groups have separate bounded harness limits; production retry windows
are unchanged. Environment is `GOMAXPROCS=2 GOPROXY=off`, `-p 1 -count=1 -v`.
Every selected top-level root needs exact RUN/PASS census and package PASS,
without a skip, failure, panic, timeout, build or race confounder.

| Control | Barrier | Normal | Race |
| --- | --- | --- | --- |
| `uninstalled_route_gate` | A signed acceptance cannot install a public route. | Pending | Pending |
| `runtime_policy_scope` | Later runtimes require a later current-policy acceptance. | Pending | Pending |
| `separate_history_gate` | Current proof cannot impersonate complete history. | Pending | Pending |
| `original_custody_ready` | Capability selection cannot infer original adapter custody. | Pending | Pending |
| `orphan_prefix_authority` | Full proof refuses actual hidden owner/module storage. | Pending | Not selected |
| `final_pending_order` | Expensive work precedes the last pending admission. | Pending | Pending |
| `proof_head_identity` | An advancing head cannot relabel the proven snapshot. | Pending | Not selected |
| `final_runtime_code` | Same-version changed runtime bytes still refuse. | Pending | Not selected |

Exactly eight normal and five selected race mutations are required. Each must
reach its exact named assertion, selected root/package FAIL and process exit one,
with no unrelated build, panic, timeout or race failure. Four light mutations and
the heavy ordering mutation run under race. The three other heavy mutations are
normal-only; the complete positive heavy fixture supplies race coverage. No
result is claimed for unselected race mutations. The final independent manifest,
raw stream hashes and complete author read-only audit must replace pending
claims before integration.

### Prospective control-oracle corrections awaiting fresh evidence

The original normal `final_pending_order` and `final_runtime_code` mutations
reached the existing fixture's earlier reservation assertion, rather than their
sealed command-exit assertion. Both original attempts remain **UNRESOLVED** under
their original oracles. Moving readmission early or removing the last code-hash
check permits the first observation to consume a durable attempt; the independent
second observation still refuses the changed nonce or runtime artifact. Command
exit therefore stays one. The fixture correctly detects reservation before fresh
admission at line 304; the handoff incorrectly expected the line-301 assertion.

Two additive corrections change only those expected assertions. Source, tests,
mutation patches, selectors and the mode matrix remain unchanged; their existing
compile/vet evidence still applies. The correction seals are
`/tmp/safe-current-capability-control-correction-01-20260930/SHA256SUMS`, SHA-256
`23de71558b121fc6b971bad09294e190e0ac6862b36a40c68ec1e7e627082369`, and
`/tmp/safe-current-capability-control-correction-02-20260930/SHA256SUMS`, SHA-256
`d6b955947e214a56aefb139057363da1f04fc18ce49050550c1001183f98cb0f`.
Fresh ordering normal/race and final-code normal reproductions are pending.
Original raw streams and result ledgers must remain intact in the final seal;
they cannot be retrospectively relabeled causal. A completed matrix would record
eight logical normal and five selected race controls plus these two unresolved
original invocations. These mutations test admission before reservation, without
claiming that either mutation defeats the independent second observation.

## Ordered integration plan

1. Completed: the exact `3f88a948` custody matrix and independent raw/source audit
   are sealed. Its MAINNET/PRELAUNCH note records 78 positive executions, eight
   normal and five selected race causal controls. Shared commit `7aa87dfa`
   integrates that exact non-Markdown source with documentation, and was pushed
   non-force with clean matching shared HEAD/origin. Earlier receipts are intact.
2. After Sol seals this exact `95a905d4` capability source, audit its complete raw
   positives and selected controls independently, including source/modules/local
   dependencies and static/supplemental handoffs. A failed or noncausal case
   remains unresolved; any reported root cause belongs to Astra for a separate
   corrected candidate and new qualification.
3. Compose the capability documentation child with the integrated custody docs,
   preserving that earlier qualification receipt byte-for-byte. Replace every
   pending capability result with its exact sealed result and update MAINNET and
   PRELAUNCH consistently. Verify all non-Markdown bytes against `95a905d4`, the
   unchanged SDK oracle/module files, and every earlier immutable receipt.
4. Require a clean shared branch and verify the exact source/documentation diff.
   Integrate the qualified composition and push non-force; verify shared HEAD,
   origin and the source fence afterward. A changed shared base is reconciled
   without discarding source or rewriting evidence.

Neither integration installs the public submit route, approves the current-only
policy, proves original history or permits a mainnet transaction.
