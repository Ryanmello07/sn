# Server-model diagnostic collection

The original run ended at **2026-09-28 00:38:30 UTC** on its configured
90-minute package timeout. Its terminal package result is failure, with
`5400.081s` reported duration. The wallet test active at that deadline did not
report a new assertion failure. This is incomplete diagnostic collection,
not a passing model suite or frozen composed-release qualification.

Observed root events: **1,048 passed, eight failed, seven skipped, 1,070
started**. Six roots had paused and the final wallet root had not completed.
Unstarted tests are not counted as passing or failing. The test and runner exit
codes are both 1; owned disposable-service cleanup exited 0. The original
server checkout remained clean at `b6f49bdbe6a61ef0cca3392b3a3ee47fe4806ec2`.

| Failed roots | Disposition |
| --- | --- |
| `TestPlanPaymentsMaxDuration`, `TestPlanPaymentsMaxDurationLoop` | Historical custody fixtures corrected in server `4468a696`; affected normal/race qualification retained. |
| `TestProbeDueQueueIgnoresTheEgressHealthGate` | Probe-health fixture corrected in `4468a696`; affected normal/race qualification retained. |
| `TestRemoveStragglerContracts`, `TestBackfillContractReapTime` | Terminal/sweep fixtures corrected in `4468a696`; affected normal/race qualification retained. |
| `TestGetProviderEgressLocationDueOrderingIsStableAcrossLimits` | Fixture timestamp ordering repair passed the affected normal/race tests; causal controls and source sealing are separate work. |
| `TestRemoveContractBatchesDrainsDuplicateCandidates`, `TestAssignStragglerReapTimeRespectsBudget` | Adjacent fixtures omit required terminal custody fields. Repair and qualification are pending; production custody guards remain intact. |

Seven skips require optional inputs absent from this capture: one GeoLite2
place-list test, five pro/referral configuration tests, and one onboarding
configuration-repository test. They remain uncovered scopes.

The [raw capture](/mnt/data/sn-testnet/evidence/mainnet-server-model-full-20260927)
retains the executable, JSON events, exit files and `PROVENANCE.md`. Its actual
Go replacements resolved active sibling checkouts, so intended source pins do
not establish a frozen graph. The independently frozen full body under
`/mnt/data/sn-testnet/evidence/server-model-final-20260927/model-run` continues
with its own 120-minute deadline and completion census. Preserve its completed
work and qualify fixture repairs separately instead of restarting it.
