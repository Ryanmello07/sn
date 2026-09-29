# MG-03 historical receipt/native fee-context qualification

**Independent source qualification passed; shared integration is pending.**
The exact qualified server candidate is
`4152738039880408ade55edec3e42c1ecf2e40ec`, tree
`c685d73209c643c4137a1c3fe0546809059b34af`, based on qualified native finality
capture `5ff7bf0264b775920049910896c23ec58862429f`. This receipt does not claim
shared-source integration, a remote push or a passing composed release.

The [offline fee-context contract](https://github.com/urnetwork/server/blob/4152738039880408ade55edec3e42c1ecf2e40ec/strecovery/NATIVE-FEE-CONTEXTS.md)
replays the existing pinned archive, receipt collection, checkpoint and GRANDPA
proof before deriving an exact historical context for each proved receipt
block. It joins the EVM block hash to every supported native Frontier commitment
within the original checkpoint-to-collection interval. It preserves the EVM
intermediate root, native child root and exact linked parent root separately.
Native and EVM heights never select a mapping.

Missing native mappings, multiple matching commitments and a checkpoint with
an unseen parent remain explicitly unresolved. A V2 certified descendant can
establish the original boundary's finality; its later headers cannot supply an
earlier receipt's execution context. The original collection and every archived
original/replacement/cancellation signature, origin and nonce history survive.

The mapped-account output is only the reviewed source-profile computation
`blake2_256("evm:" || signed_sender_H160)`. It supplies no executing-runtime or
payer admission. Native extrinsic indices and actual withdrawal, refund and
gas debit amounts remain null, as do nested actual gas fees. Runtime, payer,
fee, checkpoint/genesis, finality, accounting and spending authority remain
false. The command reads pinned local files only; it adds no RPC, signer,
broadcast, source-custody write or self-approval interface.

## Qualification and integration boundaries

Sol qualified all ten new roots and all 122 affected recovery/CLI roots in
normal and race modes. Each affected mode comprises 106 non-database recovery
roots, 13 CLI roots and three existing database roots under separately owned
disposable PostgreSQL/Redis services. Every selected root started and passed,
with package PASS, process exit 0 and no missing, extra, failed or skipped root.
Both database runs report command, cleanup, source-graph and module-graph exits
of 0, with no owned containers remaining.

All six causal controls compiled and failed their exact named semantic
assertions in normal and race: 12/12 selected root FAIL, package FAIL and process
exit 1. They cover substituted parent roots, missing/ambiguous mappings,
checkpoint parent coverage, post-boundary descendant mappings and changed
account-derivation namespace. Each actual mutant diff byte-matches the frozen
expected diff. Raw JSON, stderr, command arguments and mutant checkouts survive.

| Retained receipt | SHA256 |
| --- | --- |
| [Sol result](/mnt/data/sn-testnet/qualification/mg03-native-fee-context-sol-20260929/frozen-4152/SOL-RESULT.md) | `6eef830f6a92161cb08532942987db00da65d224679a0e0cf1dd8dc4285dd5fc` |
| [95-file raw manifest](/mnt/data/sn-testnet/qualification/mg03-native-fee-context-sol-20260929/frozen-4152/SHA256SUMS) | `7d5ef1f079d1289f609291dfbe03e5c6fcf48c788e5159627b81e46b1ca1d1d8` |
| [31-file disposable-service manifest](/mnt/data/sn-testnet/qualification/mg03-native-fee-context-sol-20260929/fixture/FIXTURE-SHA256SUMS) | `56710f200f0f35e4f201c58e7a586891592490c7a0a987223d11dcacc0be051f` |
| [Independent receipt audit](/mnt/data/sn-testnet/qualification/mg03-fee-context-integration-preflight-20260929/INDEPENDENT-RECEIPT-AUDIT.json) | `d02e4b4fac2cca18c2f534090e62a72e4915ef770611b90d496cba997ae62469` |

Astra independently verified all 95 raw, 31 fixture and 69 author manifest
entries, exact per-root raw outcomes, all causal assertions and diffs, and
both owned-service cleanup receipts. The clean exact source and eight physical
local dependency heads/trees remain pinned. Resolved module JSON before and
after testing byte-matches the author graph, SHA256
`8ec05bbede7e1b78960e6423f7e7e70ab41116a225ceb23fa507b78187856632`.
No author behavioral rerun was used for this audit.

Author checks passed compile-only builds for both packages, vet, formatting,
source/module fences and compilation of all six causal mutants. No behavioral
test body was executed by the author. The [frozen handoff](/mnt/data/sn-testnet/qualification/mg03-native-fee-evidence-20260929/HANDOFF.md)
has SHA256 `a64fb0a83247f19f1e977c0c2ac793fe1b84bb69eb22086d37799ea624c5a33e`;
its [69-artifact manifest](/mnt/data/sn-testnet/qualification/mg03-native-fee-evidence-20260929/SHA256SUMS)
has SHA256 `b41fcb1a606eee311b522dab60073b823b8141a2a3994f401e41b53048c70815`.
These author artifacts are not an independent behavioral receipt.

The planned source order is qualified native StorageProof child
`6201504ec18cd42b54083681c5dc5ca62fdcdb41`, then this fee-context source.
The two candidates modify disjoint server paths. A metadata merge preflight
checks every resulting candidate blob and unchanged `go.mod`/`go.sum`;
it does not qualify their composed behavior. The predicted tree is
`1fd1f1155abfcfa600bd1891ef7bc56081f6c346`. Shared integration remains held until
storage qualification and its preceding integration complete; composed smoke
must attest the exact resulting source and physical graph separately.

## Historical storage-proof dependency and gate impact

The separate StorageProof candidate accepts raw reads only at the exact
original collection-boundary native root. An earlier receipt's native child
root and linked parent root can both differ from that boundary. Therefore the
two current slices do not yet supply complete historical fee-storage evidence.

A later bounded interface must replay qualified fee-context derivation and
bind each raw read to the exact certified native header identity, derived root
and purpose: child events/state or parent executing `:code` and runtime. A
witness-supplied root is not authority; ambiguous mappings cannot be resolved
by selecting a convenient root. Checkpoint-parent absence stays unresolved.
That extension requires its own implementation and qualification.

Proved raw reads still need exact execution Wasm, runtime version, admitted
metadata/source correspondence and native transaction placement. General
actual fee attribution then requires bounded historical execution replay or
an admitted dedicated runtime fee record. Generic balance events share phases
with native call effects, and best-effort refunds can fail or be partial;
receipt gas, reported prices and block balance deltas do not establish the
actual native gas debit. A future event cannot repair historical receipts.

MG-03/PF-03 remain in progress. Actual fees stay null. Independent
checkpoint/genesis/runtime admission, historical native reads and fee
attribution, account nonce authority, service adoption, release composition and
live custody/restart remain separate requirements. Offline source qualification
does not approve a live checkpoint, authorize a spend or close mainnet gates.
