# One finalized observation for launch evidence

`finalized-snapshot` collects runtime artifacts and native/EVM mapping from
one authenticated native finalized identity. Use this combined observation
when a launch review needs both kinds of evidence at the same block.

```sh
sn-mainnet finalized-snapshot --rpc http://rpc.example:9944
```

The command selects `chain_getFinalizedHead` exactly once. Shared readers then
receive that in-memory identity and pin every native artifact/header read to
its exact hash. Runtime artifacts are read first. The mapping follows the
Frontier digest to exact raw EVM RLP and performs its final native, network and
EVM canonical rechecks after both components have been collected. Advancing
the finalized tip does not discard still-canonical evidence at the selected
older hash.

The new JSON has `finalized_hash` and `finalized_number` at the top level, along
with complete `runtime` and `mapping` records. It retains code/metadata bytes,
the native header, Frontier payload and raw EVM RLP. A domain-separated SHA256
digest covers the combined record, excluding the added `content_hash` field.
Sealing rejects mismatched identities, runtime tuples or native commitments.

The existing `runtime-snapshot` and `finalized-mapping` commands keep their
existing output schemas and remain usable independently. Their separately
sampled output files cannot be assumed to share a finalized block. There is
no file-joining CLI or imported identity authority in the combined path.

Supply `--expected-chain`, `--expected-genesis` and `--expected-evm-chain-id`
together to check independently approved network identity before artifact
reads. These values are never inferred as approval from the endpoint. The
operation uses one total retry deadline: 300 seconds by default, with a
configurable 60-second to 15-minute range. Helpers cannot restart that budget.
An error or cancellation returns no combined artifact. Missing mapping
capability retains exit 4; an expected-network mismatch retains exit 3.

The result always remains `unapproved_observation`. It inherits the
[runtime artifact observation](runtime_snapshot.go) and
[mapping proof boundaries](FINALIZED-MAPPING.md): header commitments are
verified, while finality/canonicality remain owned-RPC assertions. This is not
a GRANDPA/storage proof, source-to-Wasm attestation, independently approved
runtime identity or permission to sign. Canonical rechecks are not an atomic
consensus transaction; a coherently false RPC requires stronger independent
proofs to detect.
