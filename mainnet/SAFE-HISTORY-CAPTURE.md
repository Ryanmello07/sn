# Safe archive census

`sn-mainnet safe-history-capture` is a signer-free production archive reader.
It captures one through sixteen explicitly pinned consecutive native blocks,
including unrelated native and EVM traffic. It checks complete native bodies
against their headers' ordered-trie roots, follows each native Frontier digest,
and uses the server receipt collector's existing decoder to check exact EVM
headers, complete transaction and receipt roots, positions, types and cumulative
gas. Both Frontier post-log hash variants are supported; native and EVM heights
need not match.

This is an evidence input for the missing history authenticator. It does **not**
implement `bootstrapSuccessorSafeProvenanceAuthenticator`, discharge the signed
complete-history policy, or enable successor submission. Original receipts,
authority, signed bytes, nonce claims, counted attempts and liabilities are not
opened or changed. [Independent offline qualification](evidence/safe-history-census-qualification-20260930.md)
is sealed on paired SN `36fea176` and server `d21492c3`: 68 positive root
executions pass normal/race, ten normal controls and five selected race controls
are causal, with exact source/dependency and raw-evidence fences. One normal
control explicitly checks the published-Safe fixture oracle.

## Capture and retention

Supply the independently established chain identity and exact interval; values
below are placeholders, not mainnet evidence. Precreate an owner-private output
directory. The command never creates or overwrites an authority or custody file.

```sh
sn-mainnet safe-history-capture \
  --rpc "$OWNED_ARCHIVE_RPC" \
  --expected-chain "$APPROVED_NATIVE_CHAIN" \
  --expected-genesis "$APPROVED_GENESIS_HASH" \
  --expected-evm-chain-id 964 \
  --safe "$LOWERCASE_SAFE_ADDRESS" \
  --from-number "$FIRST_NATIVE_NUMBER" --from-hash "$FIRST_NATIVE_HASH" \
  --through-number "$LAST_NATIVE_NUMBER" --through-hash "$LAST_NATIVE_HASH" \
  --retry-window 300s --output /private/archive/capture.json
```

The output retains every encoded native extrinsic with its SCALE length prefix,
the exact native header fields and digest bytes, raw EVM header/block RLP, and
every raw receipt. It also projects direct calls/top-level CREATEs targeting the
Safe and every committed log at its address, without guessing event semantics.
Failed direct calls remain visible; a zero-length Safe projection is not a
healthy-authority verdict. The schema-domain SHA256 digest covers the complete
capture. It is a content seal, not an approval signature.

Publication uses an owner-private directory, private temporary file, file sync,
create-only rename, and directory sync. Existing files and final-name symlinks
are refused. A crash can leave an unreferenced temporary file; a failure after
rename can leave a complete but unacknowledged capture. Inspect and retain it;
use a new output name for another attempt. No incomplete interval is published
as successful. Standard output repeats the retained envelope only after sync;
a standard-output error does not delete the retained file. Directory replacement
by another privileged host actor is outside this local publication boundary.

Reads use `system_chain`, `eth_chainId`, `chain_getFinalizedHead`,
`chain_getHeader`, `chain_getBlockHash`, `chain_getBlock`, `debug_getRawHeader`,
`debug_getRawBlock`, and `debug_getRawReceipts`. Raw reads require the exact EVM
block hash with `requireCanonical=true`. Unsupported/null raw capabilities
produce exit4; wrong network produces exit3, invalid input exit2, and other
failures exit1. Exit0 certifies only the described census checks. No tracing,
transaction-pool, signing, or send method is admitted.

Each RPC receives its own 60–900 second retry window under caller cancellation.
Native bodies are limited to 65,536 extrinsics and 10MiB decoded bytes. EVM
vectors retain the collector's 2,048-transaction, RLP15, transaction types 0–2
profile; raw block/receipt replies each have a 16MiB bound. The reusable server
decoder limits combined encoded input to 32MiB, and the capture shares 64MiB of
encoded witness budget across its entire interval. Only one bounded block is
decoded at a time; retained witnesses/projections occupy additional memory.
Large intervals can be captured as separate explicitly pinned chunks, but this
does not automatically join them or establish a complete deployment history.

The initial and later finalized observations remain separate from exact witness
blocks. Normal head advancement requires no restart or relabeling. Native/EVM
parent continuity is checked within the interval; the owned route must still
return both the original finalized hash and captured endpoint as canonical after
the reads. Finality/canonicality remain **owned-RPC assertions**, not GRANDPA
proofs or an independently provisioned route comparison.

## Remaining production boundary

Every successful report keeps `runtime_source_proven`,
`native_execution_verified`, `internal_execution_verified`,
`deployment_history_verified`, `complete_pending_verified`, and
`send_authorized` false. Body/receipt commitments prove bytes and positions;
they do not prove execution of those bytes, the relation of individual native
extrinsics to EVM transactions, or absence of effects from native hooks.

The pinned runtime's raw debug API supplies blocks and receipts, not complete
EVM execution traces. A published Safe's `simulateAndRevert` can delegatecall,
modify a Safe storage slot and emit a log, then revert the slot and log. A
complete receipt vector and finalized storage snapshots cannot reveal that
history. A deterministic fixture executes this behavior against the actual
pinned 1.4.1 Safe proxy/singleton. Native scheduled/privileged execution and
runtime hooks add another gap beyond direct top-level EVM calls.

The unchanged signed policy requires clean deployment/initialization and
complete delegatecall/storage-mutation history, including orphan owner/module
entries. Closing it needs qualified historical execution evidence that includes
internal/reverted EVM actions and native hooks, tied to authenticated parent
state and exact reviewed runtime artifacts, plus final pending re-admission.
No such production trace/replay adapter is available here. The existing bounded
Wasm semantic replay worker does not implement the required full host surface
or EVM internal tracing. A narrower fresh-Safe invariant or current-storage
policy would require a distinct independent policy approval and qualification;
neither is silently substituted. Public `--submit` remains closed.
