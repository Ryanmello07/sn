# Historical proof replay backend

`runtime-historical-proof --historical-proof-replay-v1` reads one bounded JSON
job from stdin and writes a report only after complete execution and post-state
comparison. The process owner must impose a finite total deadline (300 seconds
by default), bounded stdout/stderr capture, and kill/join on cancellation. This
worker does not yet have a production monitor caller.

The job binds canonical SCALE parent/child headers, their exact hashes, the
complete extrinsic vector, exact parent runtime code, and raw `StorageProof`
nodes. Parent linkage, child number, extrinsics root, proved `:code`, proved
`:heappages`, executing state version, and reproduced child state root are
separate checks. Consensus seals are retained in the original header identity
and removed only from the runtime execution input. No seal/finality verifier or
source-to-code admission is supplied by this backend.

The executor and trie implementation are pinned to SDK
`cacb4310f20c7cac83eb3ccd8ed5a5ad4212608a`. A strict adapter runs fallible
`delta_trie_root` and `child_delta_trie_root` before the SDK root writer. This is
necessary because the pinned `trie_backend_essence.rs` writer logs an incomplete
trie error and can return the old root. Root equality alone would therefore not
prove that all required write paths were present. Missing reads, child roots,
child nodes and write paths refuse; authenticated absence remains distinct.

Current resource limits are 96 MiB JSON, 8192 distinct proof nodes / 24 MiB total
decoded proof, 8 MiB runtime code before and after decompression, 8 MiB block
body, 16384 extrinsics, 64 KiB headers, 64 digest items, 64 MiB Wasm memory,
65536 storage operations and 64 MiB cumulative storage I/O. Rolled-back work
still counts. A bounded original job is never retried with a skipped state or a
replacement root when a limit is reached.

The current host profile supports bounded top/child reads, writes, deletion,
iteration, append, roots and balanced transactions, plus allocator, logging,
hash and trie helpers. Other imports trap if invoked. Crypto, prefix deletion,
offchain, keystore, runtime spawning and other missing hosts require separate
implementation/qualification before a runtime using them can execute. The
synthetic positive fixtures are not evidence that a production runtime is
admitted or that its complete host surface is supported.

Every successful report states `anchor_authority=caller-supplied-unapproved`,
`runtime_admitted=false`, `production_selection=false`,
`native_fee_withdrawal_refund_observed=false`, and `native_fee_debit=null`.
Historical fee verification still requires independently admitted original
code and a fee-specific execution observation mechanism proving actual payer
withdrawal and actual refund, including zero, partial and failed refunds.
Receipt gas, generic same-phase balance events and balance deltas do not supply
that witness. Full historical replay and MG03 remain open beyond this milestone.

The deterministic tests construct complete parent state using SDK
`TestExternalities`, retain every raw top/child/value node, and independently
construct the expected child map/root. They exercise the public JSON decoder,
real Wasm storage hosts, missing read/write/child paths, unchanged-root fallback,
body/header/code substitution, unsupported hosts, transactions and bounds.
They do not use an actual admitted Subtensor runtime or claim fee attribution.
