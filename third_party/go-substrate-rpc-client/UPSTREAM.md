# Substrate RPC transport and metadata decoder corrections

This local module preserves the runtime Go sources, module manifests, README,
and license files from `github.com/centrifuge/go-substrate-rpc-client/v4`
`v4.2.2-0.20240919131012-e3b938563803`, commit
`e3b938563803c6a71043c4e7ba2e0d27c400f514`. The upstream repository HEAD was still
that commit when checked on October 1, 2026. The original module download has
sum `h1:eOBL15BXZnM4LODDmAgjOJ9Y1eehk6ABRIrEkHxmKs4=` and go.mod sum
`h1:k61SBXqYmnZO4frAJyH3iuqjolYrYsq79r8EstmklDY=`.

The upstream module occupies about 5.5 MiB on disk. This import omits upstream
`*_test.go`, testdata, automation, and development files; its 212 imported files
occupy about 4.5 MiB with the local tests. `UPSTREAM.sha256` records the original
bytes of every imported file. The owned runtime changes are in
`gethrpc/client.go`, `gethrpc/handler.go`, `scale/codec.go`, `scale/limits.go` and
`types/metadataV14.go`; `UPSTREAM.patch` records them, including the new limits
file. Reversing that patch
in a temporary copy makes every entry in `UPSTREAM.sha256` match. The root
Apache 2.0 license and the gethrpc LGPLv3 `COPYING`, `COPYING.LESSER`, and `AUTHORS`
files are retained without changes.

The transport can observe a read disconnect before the current write reports
completion. Upstream exempts that request from disconnect cancellation because
it may be reconnecting, but also deletes its response waiter. A successful write
then clears the current request, leaving it blocked until its caller expires.
The correction retains the waiter until write completion, transfer to a new
connection, or client shutdown. A write that completed on the failed connection
returns the original disconnect; it is never replayed automatically. Already
completed replies and acknowledged subscriptions keep their existing owners.
Buffered batch replies are consumed before reading the terminal error through
the closed channel, which also synchronizes access to that error.

The parent SN module selects this copy through its go.mod replacement. Its
300-second allowlisted read retry policy and 60-second read attempt timeout are
unchanged. The release builder resolves this dependency as a local module under
the pinned SN repository, using the tracked nested go.mod and SN commit rather
than claiming the original remote module checksum covers patched bytes. A
consumer that builds SN as a dependency must also select this replacement; Go
does not inherit a dependency's replacements.

Run the owned transport tests from the SN module root:

```sh
go test -mod=readonly -vet=off -count=1 github.com/centrifuge/go-substrate-rpc-client/v4/gethrpc
go test -mod=readonly -vet=off -race -count=1 github.com/centrifuge/go-substrate-rpc-client/v4/gethrpc
go test -mod=readonly -count=1 ./crv4 ./miner
go test -mod=readonly -race -count=1 ./crv4 ./miner
```

Only the dependency package commands disable vet: upstream's unused server-side
`Notifier.send` already converts its numeric subscription ID to a one-rune
string, which vet rejects. That unrelated behavior is unchanged. Upstream's
original gethrpc tests also reference removed subscription APIs and do not
compile. The owned tests force dispatch ordering for calls, batches,
subscriptions, write errors, cancellation, shutdown, and both reconnect orders.
SN's `TestSubmitRawReturnsDisconnectBeforeWriteCompletion` exercises the public
production submission path and fails deterministically if this replacement is
removed. Existing miner recovery fixtures still drop acknowledgments immediately.

The opt-in SCALE decoder owns finite collection, requested-allocation, work and
depth budgets. All value copies share one pointer-owned budget; a resource
failure is sticky. Reflected backing allocations, string copies, compact
integer temporaries and the v14 derived lookup map reserve capacity before
allocation. Custom decoders must explicitly reserve other derived allocations;
this is not a sandbox or an exact process-memory bound. Existing unbounded
callers keep their decoding policy, while malformed compact lengths and absent
option discriminants now return their read errors. Oversized compact lengths
cannot truncate through `Uint64`, and custom fixed arrays use array holders.

SN's central `crv4.DecodeRuntimeMetadata` selects the bounded decoder after an
8 MiB raw-size check on the encoded string, before hex allocation. Each metadata
collection count is bounded by the raw input length, with 64 MiB requested
storage, 2,097,152 decoded values and depth 64 as independent finite limits.
Full input consumption and exact raw hashing remain required. This does not
authenticate metadata or bound the earlier HTTP JSON-RPC response buffering.
The generic metadata API still supports its existing v4 and v7–v14 variants;
the separately pinned native SDK owns the existing v15 signing-metadata path.

Owned decoder tests, run from the SN module root:

```sh
go test -mod=readonly -count=1 github.com/centrifuge/go-substrate-rpc-client/v4/scale github.com/centrifuge/go-substrate-rpc-client/v4/types
go test -mod=readonly -race -count=1 github.com/centrifuge/go-substrate-rpc-client/v4/scale github.com/centrifuge/go-substrate-rpc-client/v4/types
```

The [qualification receipt](../../mainnet/evidence/runtime-metadata-bounds-20261001.md)
also covers shared CRV4 and self-sealed discovery inputs, valid runtime470,
independently pinned owner/root metadata and the offline SDK-v15 controls.
