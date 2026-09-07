# Release 1.0 active work index

Updated 2026-09-07 17:27 UTC. Read this short index first; use
[FINALIZE-COMPLETE.md](FINALIZE-COMPLETE.md) for detailed history and evidence.
This is not a source freeze, full-gate certificate or live acceptance report.

Previous primary checkpoint: SN `eb6f148`, server `9b582d91` (not pushed).
This SN checkpoint adds the independently reviewed closed-census evidence
publisher and its17 tests; use `git log -1` for the new identity. Other integration changes
below remain in the temporary candidate unless explicitly marked promoted.
The user subsequently approved **on-chain hashes + API/MinIO proof bytes** on
2026-09-07 UTC. Section10.1 of the complete handoff now records that decision.
Implement independent immutable validator/operator evidence slots, including
no-payout windows and later audits; do not use a payout-root-only substitute.

## Objective and current state

Complete WHITEPAPER1.0 on public Bittensor testnet, netuid521:1,000 miners,
independent top200 selection, operator pools and fair demand deposits,
validator/miner/operator payments, all proofs/history, and concurrent
adversarial actors. Final acceptance still requires both full gates, the
approved live RC and three final epochs, investigation of every anomaly, and
independently replayable on-chain evidence in `FINAL.md`.

- Primary SN checkpoint before this change is `eb6f148`. Qualified Head/EMA53, Stats28,
  Gate10, Terminal33 and ordinary retained-authority3 are now promoted; those
  last stages and subsequent primary evidence work are included in this checkpoint. Newer native/runtime29
  and shutdown fixes remain. Historical native schedule2 is now promoted, with
  exact9 normal/race tests independently confirmed from the primary tree. Closed-census
  publisher2 is now promoted, independently reviewed, and confirmed from primary
  with exact17 new and27 retained replica tests passing normal/race.
- Server checkpoint is `9b582d91`, with qualified private-service/profiler changes. The
  disposable two-pair PostgreSQL/Redis Docker smoke passed; shared services
  were not changed. No new live campaign or transaction is claimed here.
- One integration candidate: `temp/sn-integration-xOgvEe/sn`. Its outside-source
  `INTEGRATION.md` identifies exact stages, artifacts and ownership.

## Active owners and next actions

The user approved two Astra max implementation/fix agents, one Terra max
executor driving concurrent isolated jobs, and root integration. Carry owner:
`astra_canonical_resume_v2`; startup/new failures: `astra_startup_and_fixes_v2`;
executor: `terra_qualification_resume_v2`. Former isolation execution owner
completed its handoff and is idle, with no abandoned live job or source reader.

| Work | Owner | Current boundary |
| --- | --- | --- |
| Gate composition | Root promotion complete; Terra max evidence | Original63 failed the274/276 census; exact repair adds two rows and widens existing omission controls. Focus3 and widened63 pass normal/race; generator11 passes both. All10 gate/census paths promoted; no full-gate claim |
| Go qualification runner | Astra max review/repair; Terra max execution | Module-alias repair and parent-owned fixture correction pass exact54 N/R (219 events each). Root read both summaries; causal1 matches its expected failure. Configured roots remain physical and aliases resolve only to declared sources. Shared simulator compilation now passes N/R within normal300/race360, using retained caches. Service-backed matrices and real whole-matrix smoke remain unqualified |
| Terminal V2 | Root promotion complete; Terra max evidence | Exact123 passes normal/race in groups26/37/30/30, plus Stats/ordinary99, affected+Head51, focused transport1, simulator4/2. Root verified137-source fence, exact preimages and event summaries; promoted33 formatted files. Original causal/full-package obligations remain separate |
| Explicit production V2 config + ordinary binding | Terra max qualification complete; root composition | Ordinary5/widened50/SIM2 pass N/R; old-production causal5 is1PASS/4FAIL both modes; root promoted3. Config47 is now fully matched N/R: simulator6+20 in repair-v4, validator14+7 in repair-v5. Private fixtures are inert; nine retained cases use a plain loop without dropping assertions. Root independently read all four v5 checker summaries and source fence0. Temporary RunRelease guard remains unfinished startup |
| Actual startup-to-submission | Astra startup implementation/fixes; root integration; Terra qualification | Bootstrap88, historical UID35 and causal1 pass N/R. History57 passes normal and the exact selected race union3+3+3+48; retain the original aggregate3m timeout. Disk-owner2/17 passes N/R, and all6 alias regressions reproduce expected failures against the original production file N/R. Historical native schedule2/9 passes candidate and primary N/R and is promoted. Startup inventory/exact-image custody6 now passes exact10+9 N/R, with all4 original-journal causal failures matched N/R. Semantic history/current-cursor recovery, bounded intent/artifact lineage, ordinary input/EMA, V2 submission, public replicas and capacity remain open. Root proof-store startup join2/9 is a separate unqualified draft. RunRelease guard remains |
| On-chain evidence hashes | Terra max Solidity execution; Astra max failures | Full Solidity17 suites/199 tests PASS, zero failures/skips, exact census and source/artifact fences. Gencontracts24 N/R pass after normalizer5 repairs the14-type graph. Current private-graph v4 generation/check/wrapper0; exact generated output62e173ac is composed with only artifact/layout hashes changed, no ABI or bytecode change. Generator causal1 and simulator causal1 reproduce13-versus14 lost types N/R; repaired simulator layout4 passes N/R. Strict build retains coordinator24492/84 spare and append-only layout |
| Evidence Go bindings/readback | Root composition; Terra max concurrent execution | Evidence22 stabi20/reader12 N/R pass. Primary RPC4 new19/union54 N/R pass with causal1PASS/5 expectedFAIL. Combined validator76 N/R now passes exact root/event/source checks after assertion-preserving plain-loop repairs. First75 normal body passed but checker refused positive subtests; malformed selector/outcome attempts remain recorded. Miner/simulator HTTP siblings remain open |
| Simulator companion installation | Astra carry implementation/fixes; root composition; Terra execution | Installer25 previously passed N/R after the reviewed v11 fixture ownership fix. Carry13 plus FIFO/alias successor are now composed as14 paths. Current repaired70 is NOT qualified: normal57 PASS/7 FAIL then2m timeout; race12 PASS then2m timeout. All six compiles passed; original-upgrade causal1 and original-opener causal2 match exact expected failures N/R. Astra owns root-cause and adjacent repairs; preserve every failed capture. Public-mode Independent* head clones remain required |
| Closed-census publication | Root promotion complete; Astra independent review; Terra primary confirmation | New2/17 and retained replica27 pass exact N/R in candidate and primary, with source/dependency/binary fences. Independent review found no blocking issue. It owns the complete signed closure and keys, replays both actual public origins concurrently, publishes content-addressed payload/census/consent metadata with readback, then returns dual-signed ABI calldata. Production authenticated upload provisioning, durable relay/submission and finalized readback remain incomplete; no live publication is claimed |

The candidate is held only while its admitted readers run. Prepare later deltas
outside it. New Go runner source has separate ownership and does not hold the
candidate. All test execution stays with Terra max; failures go to Astra max
with deterministic root/adjacent regressions following `connect/CODESTYLE.md`.

The checkpoint is not a release-ready certificate. The Go runner's reviewed
54-test result supersedes its earlier48-test-only status, but it does not yet
start private services. Existing qualified ownership adapters remain until an
equivalently tested Go replacement. Preserve actual pre-Go launcher failures;
they are not product test failures or passes. Reuse the frozen explicit-root
adapter's literal path, not a path guessed from a new capture directory.

Next production work is still substantive: promote the qualified evidence
installer, complete and qualify authenticated existing-journal carry, and lock the final
bytecode. The real generated simulator payload is composed into the candidate,
not yet primary. Its earlier current-graph check FAILED on the evidence artifact
hash; the repaired generator now passes the actual v4 regeneration/check and the
exact output is composed; simulator layout/causal qualification is now complete.
The current214-path candidate manifest is5db18d14; startup custody10+9 N/R
and journal causal4 N/R matched on the earlier207-path manifestbb7e9537.
Current carry70 has assertion failures and timeouts and must not be promoted
as qualified. Its readers are released; source changes still require an owned
repair handoff. Old source/binary
identities remain preserved, not silently reused. The producer-gate selector
also needs explicit coverage of the newer evidence/bootstrap/history/boundary/
census/chain-evidence groups, plus installer/protocol/stabi coverage and omission
controls; aggregate normal package coverage is not a substitute for that gate.
Provision/authenticate real activations and both public proof replicas.
The source-reviewed transport direction is typed client-authenticated uploads
using the existing release API sessions, server-owned immutable storage and
explicit upload capacity limits. Do not lend operator artifact keys or MinIO
credentials to validators, or treat mutable API credentials as historical
validator authority. Use a separately funded permissionless relay for the
dual-signed evidence calldata and finalized inclusion checks; this transport
and relay are not implemented yet. Wire V2 startup through native submission, and prove all-pair
capacity. Then both full gates, source freeze, live RC/final epochs and FINAL.md.
No new live campaign or testnet transaction was performed in this work phase.

## Evidence shortcuts

- Head integration: `temp/sn-integration-xOgvEe/capture/`.
- Stats integration: `temp/sn-integration-xOgvEe/capture-stats-v1/`:validator99/
  simulator4 plus affected39+Head12 and simulator2 pass both modes; exact event
  verification and source/binary fences pass. Root verified and promoted28.
- Gate components: ACK `temp/sn-gate-ack-deadline-repair-qualification-v2-jn07pj/
  capture-formatted-v1`; repair4/combined29 and both causal3 controls complete.
- Terminal integration and runner48: `temp/sn-integration-xOgvEe/capture-terminal-v1/`.
- Config/ordinary current: `temp/sn-integration-xOgvEe/capture-config-ordinary-v1/`.
- Contracts: `temp/sn-integration-xOgvEe/capture-evidence-contract-v1/recapture-mutability-v2/`.
- Reader repaired12: `temp/sn-integration-xOgvEe/capture-chain-evidence-absence-v1/`;
  causal preimage/handoff `temp/sn-evidence-reader-absence-v1-THR4lcvj/`.
- RPC4 handoff: `temp/sn-chain-http-bound-v1-DPhza2qn/CHAIN-HTTP-HANDOFF-v1.md`.
- Evidence22 integration: `temp/sn-integration-xOgvEe/EVIDENCE-PREREQUISITES-INTEGRATION-v1.md`.
- Current candidate21: `temp/sn-integration-xOgvEe/INSTALLER-RPC-ACTIVATION-INTEGRATION-v1.md`; live capture `capture-installer-rpc-activation-v1`.
- Successor: `temp/sn-integration-xOgvEe/capture-installer-rpc-activation-v2/successor` (validator76 N/R complete); `capture-storage-layout-normalizer-v2` (gencontracts24 N/R complete).
- Normalizer5/fixture2/adjacent-loop1: `temp/sn-evidence-install-v1-TUBN0C6f/{storage-layout-next,fixture-next,native-timeout-next}`. All8 paths composed; preserve their exact preimages and failed captures.
- Bootstrap2/12: `temp/sn-release-activation-v2-rqGDGa/startup-next/BOOTSTRAP-V2-HANDOFF.md`; exact88 N/R complete in `capture-bootstrap-v2/successor`.
- Regenerated v4: contracts capture `recapture-expectation-order-v1/generated-contracts-current-v4`; generator causal1: `capture-storage-layout-normalizer-causal-v1`. Retain both preflight failures and real body results.
- Installer reviewed source: `temp/sn-evidence-install-v1-TUBN0C6f/EVIDENCE-INSTALL-REVIEW-HANDOFF-v1.md`.
- Activation3 source: `temp/sn-release-activation-v2-rqGDGa/validator/`; exact12 roots in the integration handoff's `ACTIVATION12.tsv`.
- Historical UID35: `temp/sn-integration-xOgvEe/capture-historical-uid-v1`; history57/causal2: `capture-activation-history-v1`. Keep the first assertion failure and race timeout.
- Runner54: `temp/sn-integration-xOgvEe/capture-qualification-runner-module-v3/body-reuse-v1`; causal corrected analysis: `capture-qualification-runner-module-causal1-v3/corrected-analysis-v1`. Reuse the qualified checker, not new ad-hoc census variants.
- Shared simulator binaries/normalizer failure: `temp/sn-integration-xOgvEe/capture-shared-sim-binary-v3`; installer3/25: `capture-installer-shared-v3`.
- History/installer fixture successors: `temp/sn-integration-xOgvEe/capture-history-installer-fixture-v1`; normal57 and installer25 N/R matched. Exact race57 union: `shards-v2/manifests/summary.txt`; all4 groups passed, original full57 race timeout remains recorded.
- Disk2/17 + original-production causal6 N/R: `temp/sn-integration-xOgvEe/capture-disk-state-v2/manifests/summary.txt`; handoff `temp/sn-release-state-v2-tGo6X9hX/DISK-STATE-HANDOFF-v2.md`.
- Closed publication2/17 and retained replica27: `temp/sn-integration-xOgvEe/capture-census-schedule-v1/manifests/summary.txt`; primary confirmation `temp/sn-primary-census-publication-v1/manifests/summary.txt` (exact N/R matched, independent review complete, promoted).
- Native schedule2/9: the same candidate summary and `temp/sn-primary-schedule-confirm-v1/manifests/summary.txt` (primary N/R matched, promoted).
- Simulator layout causal1: `temp/sn-integration-xOgvEe/capture-storage-layout-causal-sim-v2/manifests/summary.txt` (exact expected failure N/R, original-production overlay only).
- Startup custody6: `temp/sn-startup-history-v2-NC2FVF55/STARTUP-CUSTODY-HANDOFF-v1.md`; `temp/sn-integration-xOgvEe/capture-startup-custody-v1/manifests/summary.txt` (exact10+9 and original-journal causal4 matched N/R). The same Astra lane owns semantic history/current-cursor recovery and the selector-coverage repair.
- Carry14: `temp/sn-evidence-install-v1-TUBN0C6f/carry-next/CARRY-HANDOFF-v1.md` plus `carry-custody-next/CARRY-CUSTODY-HANDOFF-v2.md`; `temp/sn-integration-xOgvEe/capture-carry-custody-v2/manifests/summary.corrected.txt` (repaired70 failed; causal1+2 matched N/R). Four initial pre-body proof-path failures and a metadata-recording error remain retained; corrected neutral executions reused the original compiler-issued binaries and absolute proofs, independently reviewed.
- Adjacent campaign reader2: `temp/sn-campaign-evidence-custody-v1-eDT2Hz8j/CAMPAIGN-CUSTODY-HANDOFF-v1.md` (released, not composed or qualified; focused5, retained16 and causal3).
- Root proof-store join2/9: `temp/sn-release-proof-state-v2-6Wn19mJS/validator/` (unqualified draft; semantic startup integration and independent review remain).

Use compact machine-verified stage summaries for ordinary updates. Retain full
logs, failed captures and immutable command/source identities on disk; open
them for failures, suspicious signals and independent review. Do not repeatedly
reload or restate the entire historical handoff. Neither compact reporting nor
development sharding replaces complete release-gate or on-chain evidence.
