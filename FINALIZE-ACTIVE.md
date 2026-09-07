# Release 1.0 active work index

Updated 2026-09-07 07:32 UTC. Read this short index first; use
[FINALIZE-COMPLETE.md](FINALIZE-COMPLETE.md) for detailed history and evidence.
This is not a source freeze, full-gate certificate or live acceptance report.

## Objective and current state

Complete WHITEPAPER1.0 on public Bittensor testnet, netuid521:1,000 miners,
independent top200 selection, operator pools and fair demand deposits,
validator/miner/operator payments, all proofs/history, and concurrent
adversarial actors. Final acceptance still requires both full gates, the
approved live RC and three final epochs, investigation of every anomaly, and
independently replayable on-chain evidence in `FINAL.md`.

- Primary SN is `9197468` plus working changes. Qualified Head/EMA53 and
  Stats28 are now promoted; newer native/runtime29 and shutdown fixes remain.
- Server is `f478db80` plus qualified private-service/profiler changes. The
  disposable two-pair PostgreSQL/Redis Docker smoke passed; shared services
  were not changed. No new live campaign or transaction is claimed here.
- One integration candidate: `temp/sn-integration-xOgvEe/sn`. Its outside-source
  `INTEGRATION.md` identifies exact stages, artifacts and ownership.

## Active owners and next actions

| Work | Owner | Current boundary |
| --- | --- | --- |
| Gate9 composition | Terra max execution, Astra max repair | Candidate only;109 concrete source paths (108 status rows). Simulator63 failed both modes at the semantic-integrity census: 274 recorded vs 276 selected (two native-stake guards). Source/binary fences clean; generator11 qualification pending; do not promote |
| Go qualification runner | Root + Astra max review/repair; Terra max isolated execution | `scripts/qualification`; initial21 deterministic tests pass normal/race after a compiler type fix; review unfinished; no new Python tooling |
| Terminal V2 | Root next-stage integration; Terra max qualification | Donor exact123 selected union passes normal/race: repair123 PASS, causal109 PASS/14 owning failures; original full timeouts and original99 causal obligation remain |
| Explicit production V2 config | Astra source handoff; root integration | `temp/sn-runtime-evidence-config-v1-OsyKm1m1/RUNTIME-EVIDENCE-CONFIG-HANDOFF-v1.md`;47-test map; requires actual Terminal types before compile; do not promote temporary startup refusal as completed wiring |
| Actual startup-to-submission | Root + subsequent bounded Astra work | Still open: authenticated activation/history, bounded recovery/settlement, ordinary input/EMA, V2 signing/submission, public replay and capacity |

The candidate is held only while its admitted readers run. Prepare later deltas
outside it. New Go runner source has separate ownership and does not hold the
candidate. All test execution stays with Terra max; failures go to Astra max
with deterministic root/adjacent regressions following `connect/CODESTYLE.md`.

The requested checkpoint includes the unfinished Go runner, not a release-ready
tool. Source writes are paused at a coherent boundary for the commit. Initial
21-test qualification passed in `temp/sn-integration-xOgvEe/capture-go-tool-checkpoint-v2`;
the original compiler failure is retained in v1. Remaining review: final-capture error
reporting, complete input/toolchain/package provenance, bounded-output copy
handling and deterministic review regressions still need completion. The runner
does not yet start private services, so it is not approved for service-backed
test matrices. Existing qualified ownership adapters remain until an
equivalently tested Go replacement.

## Evidence shortcuts

- Head integration: `temp/sn-integration-xOgvEe/capture/`.
- Stats integration: `temp/sn-integration-xOgvEe/capture-stats-v1/`:validator99/
  simulator4 plus affected39+Head12 and simulator2 pass both modes; exact event
  verification and source/binary fences pass. Root verified and promoted28.
- Gate components: ACK `temp/sn-gate-ack-deadline-repair-qualification-v2-jn07pj/
  capture-formatted-v1`; repair4/combined29 and both causal3 controls complete.
- Terminal: `temp/sn-terminal-borrowed-capacity-qualification-v5-JURTOB/capture/`.

Use compact machine-verified stage summaries for ordinary updates. Retain full
logs, failed captures and immutable command/source identities on disk; open
them for failures, suspicious signals and independent review. Do not repeatedly
reload or restate the entire historical handoff. Neither compact reporting nor
development sharding replaces complete release-gate or on-chain evidence.
