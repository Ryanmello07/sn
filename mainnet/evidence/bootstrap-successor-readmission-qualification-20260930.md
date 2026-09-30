# Successor admission after provenance verification

**Independent qualification pending.** The affected adapter root passes normal
and race, and the command root passes normal. The remaining command race,
adjacent roots, two causal control pairs and final source/evidence seal are
pending. This note does not claim qualification or permission to submit.

Frozen source is `cd4261a8ae67cb326dd38f786cfeba03b26886ee`, tree
`b88238a937b12e6a9cd6fdc6a59bde2774ab2149`, based on qualified `a7186754`.
The author worktree is
`/home/by/urnetwork/sn-successor-canonical-readmission-owner-20260930`.
The earlier [canonical qualification receipt](bootstrap-successor-canonical-qualification-20260930.md)
remains unchanged; its ten focused, twenty-two adjacent and fourteen/six control
counts are evidence for that earlier source, not reruns of this increment.

The change authenticates provenance at the pinned finalized snapshot before
every scoped pending observation and the final canonical/runtime/native-window
checks. A synchronous fixture barrier mutates the actual Safe nonce, relayer
nonce or later runtime during proof; each refresh must reject that changed
authority. Every new observation also invalidates earlier admission at entry.
After a canceled refresh, a previously admitted counted attempt cannot send
until another complete observation succeeds.

The signed provenance policy, public command boundary and missing production
authenticator are unchanged. Public `--submit` remains unavailable before
custody loading, networking or attempt reservation. The real local EVM fixtures
use an explicitly synthetic history capability; these results do not prove a
deployed Safe's complete history or authorize a live transaction.

## Evidence and test scope

Sol medium runs behavioral tests; Astra max implements and independently audits
the evidence. Astra's gofmt, compile-only `go test -c -p 1 -vet=off ./mainnet` and
`go vet -p 1 ./mainnet` pass, including both isolated causal patches. No author
behavioral test is run. Raw independent streams are retained at
`/home/by/urnetwork/temp/canonical-readmission-validation-cd4261a8`.

The separate author handoff is
`/tmp/successor-canonical-readmission-handoff-20260930`. Its thirteen-file
`SHA256SUMS` hashes to
`2b9bd48de9821bdcbdc273d483f5ca6753f8a2a59b650bd3b2d79821bba88d26`;
`CONTROLS.json` hashes to
`2c388db08e493dc8e2b031321a0a0605674fddd109f4c91eb5bcf518e6b517f2`.
Final independent manifest and source/dependency fence: pending.

| Stream | Roots | Result / package duration | SHA-256 |
| --- | --- | --- | --- |
| `adapter-normal.log` | 1/1 | PASS / 47.296s | `a649cdb96b5f13a76992ccf91ca76a008bbe12ca00f6bcc0eb76e5d578bfb6b4` |
| `adapter-race.log` | 1/1 | PASS / 308.905s | `1ab83b26f0130e6537514bfc62bfd1962a99a6a198da37138d29b210eab6c1db` |
| `command-normal.log` | 1/1 | PASS / 51.171s | `39a291bbf885f5ad8b7e488c48d50c84ab6228f71c5ddc8e329e91c6576a6f11` |
| Command race | Pending | Pending | Pending |
| Adjacent normal | Pending | Pending | Pending |
| Adjacent race | Pending | Pending | Pending |

The two heavy roots run separately:

- `TestBootstrapSuccessorCanonicalAdapterBoundaries`
- `TestBootstrapSuccessorCanonicalCommandExecutesAndRecovers`

Requested adjacent roots are:

- `TestBootstrapSuccessorCanonicalOrphanAuthorityCannotEnableSubmission`
- `TestBootstrapSuccessorExecutionRechecksAfterReservation`
- `TestBootstrapSuccessorExecutionCancellationAndMissingClaimFailClosed`

The handoff uses `GOMAXPROCS=2 GOPROXY=off`, `-p 1 -count=1 -v`, separate
twenty-minute normal and thirty-minute heavy race package limits, and bounded
process limits. The production read/send deadlines do not change.

Both causal controls target `TestBootstrapSuccessorCanonicalAdapterBoundaries`:

| Control | Restored defect | Required named assertion | Normal / race |
| --- | --- | --- | --- |
| `provenance_after_pending` | Move proof back after pending/runtime/window admission. | `canonical admission reused pending state from before provenance Safe nonce` | Pending / pending |
| `stale_admission_after_failed_refresh` | Invalidate old admission only after a potentially failing checkpoint. | `failed canonical refresh reused earlier send admission` | Pending / pending |

Each patch must run alone, reach its intended root and assertion with process
exit one and package FAIL, and have no build error, panic, timeout or race report.
The author control worktree is restored clean at the frozen source. Final raw
control review and source/dependency comparison remain pending.

## Limits

This increment addresses the order of current admission around an expensive
history operation. The genuine Safe provenance implementation is still absent;
complete storage authority and independent signer cutover remain live gates.
Additive independently signed runtime revisions preserving all original receipts,
signed bytes, nonce claims, counted attempts and liabilities are the next
separate P0. No live RPC, signing, transaction, installation, validator activation
or native 10/90 acceptance is established.
