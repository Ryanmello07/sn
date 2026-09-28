# Production continuation candidate — 2026-09-28

Status: source candidate; **qualification pending**. No mainnet keys, calls,
submission, deployment, active simulator process or release lock was changed.
Astra authored the source and deterministic fixtures. Terra owns normal/race
test execution; compile-only checks are not a pass claim.

## Concrete failures addressed

1. `ReleaseSteerer.Run` asked for fresh signing/runtime/scheduler state before
   entering `SubmitOnce`; `submitOnceV2` asked for fresh EVM state before reopening
   retained work. Either could strand a valid pending original signature.
2. Production lacked the provisional loop's deferrals. Pure read exhaustion or
   missing receipt evidence could spend the fatal budget or let a new native
   epoch turn unfinished work into a startup failure.
3. Receipt search returned only found/not-found. Comparing that older negative
   scan with a newer finalized nonce could misclassify our own intervening
   inclusion as a foreign transaction.
4. A returned finalized head behind an already retained receipt is unavailable
   evidence, distinct from a successfully returned contradictory canonical hash.
5. The existing geth HTTP status classifier omitted ordinary HTTP 500 although
   the artifact HTTP reader already recognized all 5xx statuses.

## Source behavior

The production outer loop invokes the existing public `SubmitOnce` owner before
fresh scheduling. Compact submission first reopens the actual authenticated V2
intent. Pending receipts and finalized application rows are reconciled before
unrelated current decision reads. No production startup fence is weakened.

Read owners use 300-second total / 60-second attempt budgets and interruptible
delays. Pure typed transport/missing-result exhaustion is an observable wait;
malformed data, permanent application failures, local custody, real contradictory
identities, mixed errors and service cancellation retain their separate outcomes.
The real owner emits native/steering progress after classified operations. Store
begin/update/current observations use the qualified after-close/after-release
hooks. Waits do not clear a previously observed hard defect.

`FinalizedExtrinsicScan` retains a private witness for the exact complete-body
coverage boundary. Production reads canonical nonce and native epoch at that
same block. If finality advances before exact-byte rebroadcast admission, the
next poll extends the scan first. A newer head cannot retroactively widen absence.
Rebroadcast also retains the original deposit-audit publication/EMA duties.

Native prepared transactions currently have an **immortal era**. Passing an
epoch or local approval interval cannot revoke signed bytes. Later unresolved
epochs remain missed/pending; no replacement signature or manufactured native
success is authorized. Receipt or authenticated foreign nonce use resolves the
old liability. Original config-history authority stays distinct from current
signing authority. Existing historical-only renewal grants remain observation-only.
That historical-only branch currently returns pending after complete receipt
absence, before nonce reconciliation: foreign-nonce resolution under a renewed
historical grant remains open. Current-authority pending work uses the exact
same-boundary nonce proof implemented in this candidate.

## Qualification scope to run

All commands use `GOWORK=off GOMAXPROCS=2`, physical
`GOCACHE=/mnt/data/sn-testnet/gocache`, and a capture-specific physical `TMPDIR`.

- `go test ./crv4 -run '^Test(ReceiptScan|ReceiptBlock|ReceiptHeader|LocateFinalizedExtrinsic)' -count=1`
- `go test ./validator -run '^Test(ProductionContinuation|ProductionSteering|ProductionAuthorityHistoryPending|ProductionAuthorityHistoryReceipt|ReleaseSourceFinalityRead)' -count=1`
- The same affected selectors with `-race`, plus `go vet ./crv4 ./validator`.
- Enumerate the matching roots before execution; preserve each source HEAD,
  physical path, raw JSON log, exit code and source fence. Adjust timeout to the
  real M8/full-replay workload without lowering policy thresholds.

The nonempty fixture retains actual Stats detach journals, signed compact M8
records, deposit/binding replay, exact production sidecar, cryptographic native
preparation, envelope bytes, real private intent begin/current/update and reopen.
Its synthetic zero-price policy is independently selected before signing. It
does **not** qualify paid captures, 10% economic acceptance or live Yuma outcomes.
The lost-ack adapter records one actual original `SubmitPrepared` subscription;
later inclusion comes from complete canonical block bodies and original metadata
events, not an injected receipt verdict.

Required assertions include lost acknowledgement → read outage → native epoch
advance → fresh owner → original receipt → exact applied row; no new signature,
one broadcast, unchanged original age; intervening self-inclusion between scan
head and newer advertised head; real closed-descriptor error joined with a
timeout staying hard; behind-node recovery versus canonical contradiction; and
actual configured EVM HTTP 500/502 recovery.

The actual `Run` test proves routing to the startup owner before fresh scheduling.
It deliberately retains the startup refusal because this fixture does not model
full activation publication, normalized config and dual authenticated upload
sessions. Separate loop callback controls qualify classification only and never
count as restart/inclusion evidence.

## Causal controls

Freeze source before Terra runs. In a separate overlay, restore the prior
fresh-runtime/EVM-before-intent order: the retained receipt test must fail at its
named fresh-runtime/continuation assertion, not compile or setup. Restore nonce
selection from the newer advertised head: the intervening self-inclusion test
must reject that false foreign-nonce result. Restore the old production `Run`
scheduler branch: the actual routing test must observe the forbidden fresh
runtime read. Restore the behind-receipt hard error and HTTP500 omission for their
two focused controls. Keep successful full test bodies unchanged while correcting
an independent failed fixture.

## Fixture diagnostics retained

Terra's early single-root diagnostics are under
`/mnt/data/sn-testnet/qualification/production-continuation-owner-20260928`.
They exposed incomplete zero-price audit identity/deadline, a standalone seal's
generation differing from the real Stats cursor, a nonzero committer in an absent
source slot, and re-signing a randomized envelope after its hash was pinned.
Corrections retained the real physical cursor, entirely zero absent ABI tuple
and exact first signed envelope bytes. No acceptance rule was relaxed.
The corrected single-root `TestProductionContinuationDurableIntentOwner` passed
normally at fixture correction `28380c5f` (Terra, 18.089 seconds; isolated
cherry-pick `1d4c73eb`, `corrected-28380/normal.jsonl`). This is the fixture's
begin/current/update/restart result only; continuation and race qualification
remain pending.

## Explicit remaining work

- Full actual `RunRelease` fresh activation, normalized config, dual-upload and
  durable continuation composition is still required.
- Persist authenticated bounded receipt prefixes with a semantic proof version
  and original attempt binding. Current scan retains the terminal boundary only;
  a later failed read can still require prefix replay. Avoid per-block full
  journal writes and remove duplicate authenticated body/header reads where safe.
- Resolve the historical-only renewal branch's foreign nonce outcome without
  granting it fresh signing authority. Its present receipt-only wait is explicit.
- Miner recovery also needs bounded subrange checkpoints within its 4096-block
  range so a late outage cannot repeatedly discard thousands of successful reads.
- Configured native HTTP status/EOF physical-origin preservation is a separate
  author/qualification slice. Consume its exported strict classifier after
  integration; no diagnostic-text matching is introduced here.
- Automatic compatible runtime policy, skipped-epoch authority and real mainnet
  inputs/economic observations are not supplied by this patch.

The header fixture inventory is retained at
`/mnt/data/sn-testnet/qualification/production-continuation-20260928/header-fixture-inventory.txt`.
Complete-receipt consumers use the genuine full header/hash/wire helper. Remaining
SDK/number-only emitters in mainnet-runtime-history, production-runtime-history,
upload-authority, native identity/account/context and miner claim/capability tests
currently exercise different readers; deliberate malformed cases remain intact.
When those paths adopt complete receipt admission, update their independent
canonical fixture chain rather than weakening the production wire decoder.
