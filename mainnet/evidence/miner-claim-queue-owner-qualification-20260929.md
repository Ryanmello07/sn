# Miner claim queue lifetime ownership qualification

The isolated production candidate `b7e84b2f2dd2bf8cdb98c656e6e37da6ad848061`
on `codex/mainnet-claim-queue-owner-20260929` holds one physical claim-queue
directory lock across retained state reads, publication, network workers and
joined shutdown. Duplicate processes, path aliases and replaced directories
cannot silently establish a second queue owner. The queue's existing bytes and
signed transaction format are unchanged.

Sol's [raw receipt](/mnt/data/sn-testnet/qualification/sol-claim-queue-owner-20260929/RESULT.md)
(SHA-256 `3d4892e80016f8ade4659f704b897d0e15761f6cb39d629b029c90a5d5a641fd`)
records 16/16 new and 70/70 affected roots passing normally and with race
detection, all 286 `./miner` roots passing in plain normal mode, `go vet` passing,
and three isolated causal mutations failing their six assigned roots at the
intended ownership/custody boundaries in both modes. Source and dependency
fences stayed clean. No live chain, signing device or deployment was used.

The original full-package `go test -json` attempt is retained as a test-mode
exception: Go's JSON runner aliases stderr to stdout before test bodies, while
the existing library-initialization test asserts that the two are distinct.
That exact test fails under JSON mode on both candidate and parent and passes
under plain mode on both. The full-package pass used plain `go test -v` with
separate streams.

This qualifies the isolated ownership fix, not a production deployment. An
adjacent bounded-size read/publication successor is being reviewed separately;
its qualification must preserve the ownership result before integration into
the mainnet release.
