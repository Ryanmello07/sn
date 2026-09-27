# Finalized native/EVM observation

`finalized-mapping` captures an owned RPC's finalized native header and the
EVM header committed in its Frontier digest. It authenticates both header
hashes and corroborates canonicality without assuming equal block numbers.
It is signer-free and always emits `admission: unapproved_observation`.

```sh
sn-mainnet finalized-mapping --rpc http://rpc.example:9944
```

Supply `--expected-chain`, `--expected-genesis` and `--expected-evm-chain-id`
together to check independently approved network identity before EVM reads.
The command never obtains approval from the endpoint. The default retry budget
is 300 seconds for the entire observation; the configurable bound is 60 seconds
to 15 minutes. Transient transport/overload/timeouts reuse the shared bounded
read policy. Cancellation or error publishes no partial proof.

## Verification and retained evidence

1. Read the finalized native hash, complete header and runtime identity. Hash
   the native SCALE header and confirm its canonical native height.
2. Re-read that exact header, normalize equivalent hex spelling and require
   the same full header commitment. Parse exactly one `Consensus(fron, PostLog)`
   digest. The supported payloads are variant1 `Hashes { block_hash,
   transaction_hashes: Vec<H256> }` and variant3 `BlockHash(H256)`. Both compact
   lengths and all payload bytes are consumed; duplicate logs or malformed
   vectors fail. Other variants remain mapping-specific compatibility gates.
3. Fetch `debug_getRawHeader` with the object selector
   `{"blockHash":"<digest EVM hash>","requireCanonical":true}`. Require exact
   Keccak-256 of the retained RLP to equal the digest's block hash, then decode
   the reviewed fifteen-field Frontier header and obtain its own EVM number.
4. Query `eth_getBlockByNumber(<decoded EVM number>, false)` and require the
   same EVM hash/number. Variant1 also requires the complete ordered transaction
   hash vector to agree with the native commitment. Recheck native genesis,
   native canonical hash and both network names/IDs, then repeat the EVM
   canonical lookup to catch a changed index or proxy view during the sample.
5. Retain the complete native header/digests, exact raw EVM RLP hex, both block
   numbers/hashes, full observed runtime tuple and a domain-separated SHA256
   envelope. An independent reader can reproduce the native and EVM hashes.

No native-height lookup selects an EVM candidate. The deterministic fixture
uses native height 100 and EVM height 37 to enforce that distinction.

The raw EVM header is essential: Frontier's JSON rendering divides its stored
millisecond timestamp by 1000 and supplies a runtime-derived `baseFeePerGas`.
Hashing that rendered JSON as an ordinary Ethereum header can produce a
different hash. The verifier does not attempt to reconstruct lost timestamp
precision, drop fields until a hash matches, or fall back from missing raw RLP.

## Exact reviewed source

Codec reference: Subtensor commit
`67dcf7f791dc495064c293f080a0702cb433e51e`:

- [Frontier consensus log definitions](https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/vendor/frontier/primitives/consensus/src/lib.rs):
  engine `fron`, PostLog indices 1/3, ordered transaction-hash vector and log
  uniqueness.
- [Ethereum pallet](https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/vendor/frontier/frame/ethereum/src/lib.rs):
  `on_finalize` calls `store_block`, which stores the Ethereum block and emits
  its corresponding post-log.
- [Raw debug RPC](https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/vendor/frontier/client/rpc/src/debug.rs):
  `raw_header` returns the stored header's `rlp_bytes()`.
- [Ethereum JSON renderer](https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/vendor/frontier/client/rpc/src/eth/mod.rs):
  `rich_block_build` converts timestamp units and adds the separate base fee.

This reference describes the parsed wire profile. It does **not** prove that
the observed runtime Wasm was compiled from that commit. The output explicitly
sets `runtime_source_proven: false`; source-to-Wasm qualification and runtime
approval remain separate mainnet launch gates.

## Availability and limits

The mapping-specific read profile alone permits `debug_getRawHeader` and
`eth_getBlockByNumber`. Neither method is added to ordinary identity/storage
reader admission. Mutation methods remain refused. RPC replies are bounded to
1 MiB, raw RLP to 64 KiB, and transaction hashes to 2048 with the existing tighter
64 KiB-per-digest limit. Retained native headers keep their existing count/byte
bounds.

Exit 0 means a complete unapproved observation; exit 2 is invalid arguments;
exit 3 is an independently supplied network mismatch; exit 4 means the mapping
is unavailable because the required raw-header capability, post-log profile
or canonical block is missing. Other integrity/read failures exit 1. There is
no JSON-header or guessed-number fallback when the raw method is absent,
rejects the object selector, or returns null.

The native finalized selection and canonical EVM lookup are still assertions
of the owned RPC. The proof authenticates the two linked header commitments;
it is not a GRANDPA proof, storage proof, transaction-body/receipt proof, source
attestation, or mainnet readiness claim. Separate canonical reads are not an
atomic consensus snapshot. A future Frontier post-log or EVM header codec can
block this mapping command without adding its unsupported profile to general
identity checks.

Owned Snow exposed the required methods on 2026-09-27, and the exact hash-object
selector returned hash-reproducible raw RLP. Its observed EVM chain remains 945
(testnet), so these successful checks grant no mainnet authority.
