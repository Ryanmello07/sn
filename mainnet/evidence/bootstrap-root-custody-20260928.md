# Bootstrap local root custody — component qualification

The executable `bootstrap plan/apply/resume` phase invokes the real root
custody and service owners, retains their original allowances and verifies an
imported public signature. A deterministic local command-to-owned-HTTP fixture
also drives the existing service through one exact-byte send, canonical
finalization and command resume. No live key, route or chain effect was used.
See [the command contract](../BOOTSTRAP-ROOT.md).

Source worktree:
`/mnt/data/sn-testnet/worktrees/mg08-bootstrap-custody-20260927/sn`, on
`codex/mg08-bootstrap-custody-20260927`, based on
`615a76753e57c11ad688f128642882b71e35559a`.
Retained raw evidence:
`/mnt/data/sn-testnet/evidence/mainnet-bootstrap-custody-20260927`.

## Completed checks and the final-source distinction

Before the initial-claim repair, the frozen source passed full mainnet normal
tests: **289 roots plus 41 subtests, 231.854s**. Its bootstrap selector passed
race detection: **18 roots, 132.436s**. Package vet, CLI build, deterministic
outline emission and source-hash/diff checks passed. The exact generated test
binaries, build information, JSON events, source patch and manifests are
preserved under `pre-initial-claim-recovery/`. These results are not described
as testing the later store repair.

The review then identified a crash between initial marker and progress
publication. The final source adds a synced claim-complete suffix before any
child can open. Resume may repair only the exact initializing marker, absent
child files/markers and missing or valid initial `claimed` progress. Completed
claims, corrupt/advanced progress and partial markers cannot obtain a fresh
allowance. Two deterministic roots cover both real interrupted write boundaries
and nine inadmissible recovery images.

The changed store passed **four focused roots, 7.357s**, with:

```sh
GOCACHE=/mnt/data/sn-testnet/gocache TMPDIR=/mnt/data/sn-testnet/evidence/mainnet-bootstrap-custody-20260927/tmp GOWORK=off GOMAXPROCS=2 go test -json ./mainnet -run '^TestBootstrapRoot(InitialClaim|DirectorySync|MissingState)' -count=1 -timeout=10m
```

Final-source package vet passed with the same environment. Replacing only the
initial-claim recovery decision with rejection makes
`TestBootstrapRootInitialClaimRecovery` fail at its intended resume assertion.
Four earlier causal overlays independently expose the absent command, circular
startup requirement, recreated completed child and late signature-continuity
check. Actual child writes precede the tested progress/output failures; tests
do not substitute a fake completed child or infer success from an empty index.

The final affected normal/race selector is **`^TestBootstrap`**, covering
20 roots (12 new command/owner/dependency roots and eight existing review-plan
roots). The retained `qualify.sh` runs both, five expected-failing causal
controls, vet, CLI build and source verification with explicit `GOCACHE`,
`TMPDIR`, `GOWORK=off` and `GOMAXPROCS=2`. The independent final-source runner is
pending at this candidate handoff; no final race pass is claimed here.

## Provenance and limits

`module-graph.json`, `package-graph.json`, `go-environment.json` and
`local-module-provenance.json` record actual resolved paths and Git state.
`consumed-local-source.sha256` pins 199 consumed local source files. The candidate
SN checkout contains the proposed change; sibling replacements resolve to
active clean repositories, including Connect/SCTP
`358cefaef9b058cdd06ba5e9c4feeadef64ae1fb` and server
`0633780cb5e97d29be423adfb19a8748a28e4895`. This is observed component provenance,
not an immutable complete release graph. Final release qualification must use
the separately frozen real-worktree module graph and verify every replacement.

Local custody completion remains distinct from signature issuance, qualified
production current authority, global signer/device fencing, owned-route live
approval and service activation. Owner trim, full contracts including the
evidence journal/anchor, UR production activation and realized native 10/90
outcomes remain further bootstrap phases. Review schema v2 records their
observed outcomes as postconditions rather than circular startup inputs.
