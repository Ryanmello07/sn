# Receipt prefix fixture correction

This successor preserves all production source from candidate `22aca4ccf6ad8671baf2117dd06fc98e3969a2b6`. It corrects three test files after Terra's frozen normal capture found five failures. Normal/race qualification of this successor is pending; author execution was compile-only.

The four original-authority pending-loop tests now serve a complete 56-byte Subtensor `System.Account` row using the actual metadata-derived hotkey key. Its nonce equals the original prepared transaction's nonce. The fixture refuses any nonce read outside block 101, the exact authenticated complete-body boundary. Existing hard-error, cancellation, epoch-crossing and visible-wait assertions are unchanged.

The long chunk/restart/eviction test separately approves blocks 100–230 before the complete production config, source envelope and original intent are signed. Its late receipt at 230 is therefore within the independently signed read window. The default fixture still approves only through block 200. No approval is changed after signing, and no runtime gate is relaxed.

Compile-only passed with `GOWORK=off GOMAXPROCS=2 GOCACHE=/mnt/data/sn-testnet/gocache`, a physical capture `TMPDIR`, and `go test ./validator -run '^$' -count=1 -timeout=300s`. `gofmt` and `git diff --check` passed. No author test bodies ran.

Terra should preserve the original failed capture at `/mnt/data/sn-testnet/qualification/receipt-prefix-20260928/terra-22aca4cc/validator-normal.jsonl` and rerun only these five affected roots normally and with race detection:

```text
^TestProduction(AuthorityHistoryPending(WaitKeepsLoopAlive|WaitReconcilesNextEpoch|WaitPreservesHardFailures|ReceiptTimeoutKeepsObservation)|ReceiptChunkResumesDurableIntentAfterOutageAndEviction)$
```

Reuse completed original-candidate positives and their causal controls. Production bytes and the diagnostic callback interface are unchanged, so the separately authored bounded-output composition remains valid.
