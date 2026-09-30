# Current production release composition

`go run -mod=readonly ./scripts/mainnet-release-build --config /absolute/config.json`
builds a local Linux/amd64 candidate from explicit clean Git commits and physical
checkout paths. It never starts an application, contacts a chain, signs, pushes
an image or deploys. The output directory must be new and outside the source
workspace. Keep scratch, compiler caches and output on a capacity-checked build
volume, such as `/mnt/data`; do not consume the small system volume implicitly.

This is the current role census, rather than the historical v11 seven-binary
selection:

| Source | Command packages | Retained outputs |
| --- | --- | --- |
| SN | `mainnet`, `cli/miner`, `cli/validator`, `cli/snclaim` | Four executables, their build settings, hashes and command receipts. The mainnet executable also contains root-service, root-monitor, operator-monitor, owner-signing, bootstrap-chain, bootstrap-contracts, activate-validators and repair-validator. |
| Server services | `cli/api`, `cli/taskworker`, `cli/proxy`, `cli/connect`, `cli/alt`, `cli/gossip`, `cli/mcp`, `cli/competitionworker` | Eight executables and eight binary-bearing image contexts. |
| Server maintenance | `cli/monitor`, `cli/strecovery`, `cli/competitiondbinit`, `cli/competitionpatch`, `cli/geolite2export` | Five executables and their build receipts. |
| Contracts | ReserveSink, SettlementVault, Coordinator, ValidatorEvidence, ERC1967Proxy | All five selected deployment artifacts beside freshly compiled Foundry artifacts and the unchanged historical catalogue; probe/drill dependencies are checked by the existing generator. |

The config schema is `urnetwork-mainnet-release-build-v1`. Required fields are
`candidate_id`, `workspace`, `output`, `version`, positive `source_date_epoch`,
the `go`, `forge`, `solc` and `git` tool objects (`path`, `sha256`), and
`repositories`. Each repository object supplies `name`, workspace-relative
`path`, full Git `commit` and `tree`. The exact repository census is SN, server,
proxy, glog, goidenticons, userwireguard, warp, and the three Solidity libraries
forge-std, openzeppelin-contracts and openzeppelin-contracts-upgradeable. The
first seven use their repository name as path; libraries use
`sn/evm/lib/<name>`. Tool paths must identify real executable files, not symlinks.
The caller must provision reviewed module/compiler dependencies before the
offline build. The manifest retains the complete config, input hashes, tool
version logs, compiler commands and exits.

`contract_catalog` selects the contract bytes explicitly. Omission or `retained`
keeps the checked-in `sim-testnet/contracts_gen.go` catalogue. `fresh` exports
the exact current compiler bytes for review as the first unsigned mainnet plan's
catalogue. Unknown values fail closed. Both modes keep the existing schema
`urnetwork-contract-release-artifacts-v1` and select the same five contracts.

Fresh mode retains `inputs/contracts_gen.go` and
`inputs/contracts-retained.json`, generates a separate
`inputs/contracts-fresh-binding.go`, and writes the selected
`inputs/contracts-release.json`. All four files are hashed in the manifest.
The generator's complete semantic check still runs against the checked-in
catalogue before selection. Fresh capture additionally requires unchanged ABI
(including constructors), normalized storage-layout hash and semantic immutable
references, and exact creation/runtime equality with every Foundry artifact.
Source files and historical output directories are never rewritten.

Each contract entry records `retained_*`, `selected_*` and `rebuilt_*` hashes.
`exact_bytes` and aggregate `source_to_bytecode_exact` compare selected bytes
with compiled bytes. Fresh mode refuses any nonexact contract; retained mode
refuses any selected identity that differs from the historical catalogue. The
manifest's `contract_catalog` is always explicit, including when config omits it.

The current server API mismatch was reproduced before repair: four of thirteen
server commands compiled and nine failed. SN's four commands compiled. Server
`898dc8f3b211d1e2fca1b0a0c970f7673b36fd7b` replaces stale sibling SDK/Connect
overrides with the same reviewed replacements already selected by SN:

| Module | Effective immutable version |
| --- | --- |
| SDK | `v0.0.0-20260928100458-516521fb16da` |
| Connect | `v0.0.0-20260928101830-b163f9dd9ac3` |
| `github.com/pion/sctp` | `github.com/urnetwork/connect/sctp v0.0.0-20260928101830-b163f9dd9ac3` |

Both main modules resolve independently. The builder records their effective
module graphs, exact module/go.mod sums, local module Git ownership and module
file hashes. It retains the three API-bearing module zip files and SHA256
hashes, and runs `go mod verify` before and after compilation. `GOWORK=off`,
empty `GOFLAGS`, `GOENV=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`
and fixed compiler/platform settings prevent ambient workspace overrides or
automatic toolchain/module downloads. Binary build info must independently
match the selected source revision, clean state, package and Linux/amd64 target.
Source identities, module graphs, tool hashes and output bytes are rechecked
before the final domain-separated manifest seal.

The first builder invocation is retained as a failed attempt: Go's lazy module
graph includes unused tool dependencies without `GoMod`/`GoModSum` fields. The
complete follow-up census found 634 SN nodes and 644 server nodes. Both graphs
have 347 nodes missing go.mod metadata (346 entirely lazy plus one unused cached
body); another fourteen SN and twenty-five server nodes have metadata without a
source body. The successor retains every node, explicit unqualified fields and
these counts. Every dependency actually linked into any of the seventeen
executables must have authenticated materialized source, matching identity and
body/go.mod sums; a graph-only node cannot qualify. Local build-info versions
use `(devel)`, normalized only for an exact pinned Git repository. The original
failed exit and its config are not replaced by the successor capture.

The [independent server qualification](/mnt/data/sn-testnet/mainnet-release-composition-sol-20260930/evidence/RESULT.md)
reproduced all thirteen current server builds and all three source-graph roots
in normal and race modes, with vet. Its SHA256SUMS digest is
`16f069e0a8c62785a1e54c6a206454b6c2c9f4ff5e9d4d61daa2a0ab32f1aed8`.
That receipt covers server `898dc8f3`, not this builder or an approved composed
release. The author's corrected seventeen-command census is separate compile
evidence. Independent builder qualification and the first sealed builder output
are recorded in the release handoff; no behavioral-test pass is inferred here.

Every image context retains its production Dockerfile verbatim and a fresh copy
of the selected binary, with both copies' hashes joined to the binary manifest.
The package lock, all eight original image Makefiles, and database/signal
migration source files are retained. Proposed local build arguments use a
fixed source epoch and timestamp rewriting for all eight services, including
the scratch competition worker. The builder does not invoke Docker. These
contexts have no runnable OCI digest or embedded-binary/rootfs verification;
`missing_images` lists all eight and `source_to_image_verified` stays false.
Image package-archive/attestation policy and environment selection remain open.

The September 30 retained-catalogue full-source Foundry build used solc 0.8.24, Cancun, optimizer
200 and via IR, and completed successfully. ReserveSink, SettlementVault and
ERC1967Proxy match retained creation/runtime bytes exactly. Coordinator and
ValidatorEvidence retain their prior deployment bytes, while current coherent
source compilation changes their metadata digest. Comparing available compiler
metadata identifies the changed imported `src/STSettlementVault.sol` content
from `3b39de98`; settings and paths match. The generator's complete semantic
check passes: executable and constructor bytes outside the narrowly recognized
Solidity metadata digest, ABI, normalized storage layout and semantic immutable
offsets match. This is not byte-for-byte source-to-deployment reproduction.
Coordinator runtime is 24,564 bytes, twelve below the 24,576-byte deployment
limit; retained and fresh sizes remain checked. The retained-mode manifest
records both compiled and historical hashes and cannot turn metadata tolerance
into exact equality. Fresh mode selects all five current compiler artifacts
explicitly and records their exact equality separately from historical bytes.

No signed mainnet deployment plan has been evidenced. The checked-in catalogue
is historical release/testnet material, not proof of an immutable mainnet
commitment. The preferred first-plan path is explicit fresh selection, review
and independent qualification of that catalogue, then binding its exact path
and SHA256 in the unsigned bootstrap contract specification's `artifacts`
reference. The
existing loader in `mainnet/contract_artifacts.go` already consumes that schema;
it does not require a source-code catalogue replacement.

Before signing the first mainnet plan, the release owner must check for any
externally held signed artifact, plan or transaction commitment. Absence from
this repository is not proof that none exists. Any such commitment must remain
unchanged and be reconciled under its actual authority before a different
catalogue is selected. Historical source reconstruction or a separately
approved metadata-equivalence exception remains a conditional fallback; this
fresh path selects neither exception nor authority to change signed bytes.
No plan is signed and no contract is deployed by the builder.

`source_to_bytecode_exact` is calculated per artifact and in aggregate;
`reproducibility_verified`, `release_complete` and `deployment_approved` remain
false. Further gates include an independent second build, complete compiler
installation attestation, current OCI builds and readback, arm64 qualification,
approved runtime config/policy and migrations, and production-path qualification.
This increment supplies reviewable current source and binary evidence; MG-02
remains open.
