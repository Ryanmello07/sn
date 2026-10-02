# Server intake and durable storage progress, October 2

This is a checkpoint of completed, bounded observations. The broader server
model gate and durable-storage adoption are still running or unfinished. It
does not seal either active qualification workspace or qualify deployment.
The frozen SN `258e25b4` / server `0aa1e244` release and its receipts remain
unchanged.

## Server intake

The static review of server `025802a50e2dc56dc56c3cb749db7375aa9f72be`,
tree `adf05777aca3ad4fb97c4befd2163a74064d635b`, is complete. It compared
eleven changed files against `0aa1e244`; no module, migration, runtime,
contract or protocol-wire change was found. The review found no additional
production blocker in that bounded delta. It identified retained escrow/debt
guards and a production ProberBootstrap error-delay cap as the relevant changes.
This remains a source-review conclusion with separate test scope.

Independent focused qualification uses that exact server with SN
`6cfc4773038fae8b1de07807eeeb4a97c32d07d3`. Its completed observations are:

| Scope | Observed result | Limit |
| --- | --- | --- |
| Server model/task/taskworker/work selectors | 33 roots pass normal and race; all four package vets pass | Selected roots with private synthetic PostgreSQL/Redis, not a whole-model pass |
| SN cleanup composition | Seven roots pass normal and race | Synthetic composition seams, not deployed ingestion |
| SN `sim-testnet` vet | Fails at `evidence_relay_provisional_continuation_test.go:139` and `:218` because test fixtures copy a mutex-bearing runtime | These two fixture findings stay open until a separately qualified correction |
| Historical server controls | Three roots pass normal and race | Preserves the historical scope only |
| Old-source causal controls | Four intended failures across normal/race | Lost retained live grant and excessive Bootstrap retry delay on frozen older source |
| Broader server model | Pending at this checkpoint | No result inherited from the focused subset |

The effective production maintenance pool still needs at least two connections
plus unrelated owned work. The five-minute retry cap starts at failed-attempt
finalization; it is not a five-minute wall-clock cleanup guarantee. Fleet
capacity, real ingestion, alerts, production pool/claim configuration and a new
exact release composition remain open.

Retained source-review root:
`/mnt/data/sn-testnet/server-mainnet-integration-review-20261002/evidence`.
`REVIEW.md` SHA-256 is
`c47b77741062f051d77066625b48835c4f9e73e352ae2b51309c4543436475fa`;
`scope.json` is
`05923fe6a4a7e64a8696d33a24051c99f58021f11cf367d29b6a9da67cad89d2`.
The independent workspace is
`/mnt/data/sn-testnet/sol-server025-integration-independent-20261002`.
Its completed `focused-results.json` at this checkpoint hashes to
`12f5a2857cc02e016c3380e410109f897679726f712c4f43f514f61e3f3813eb`;
the exact failing `sim-testnet-vet.log` hashes to
`e46199c293c949f98ffb1eb1bf6c452df1d963f391c8c803569d3760de46b628`.
Historical phase logs are `historical-three-normal.jsonl` and
`historical-three-race.jsonl`; their active wrapper's `early-results.json`
is not presented as a final receipt.

The later, separate [completed-scope receipt](server025-focused-causal-20261002.json), retained byte-for-byte from `focused-causal-receipt.json`,
hashes to `484ada95842abdc5eed1b24a57bc0460d541c16c90d61884648ccb3055b9c31d`.
It binds the 40 focused server/SN roots in both modes, four successful server
vets, the two SN mutex-copy findings, three historical roots in both modes
and four intended old-source failures. It makes no whole-model pass claim.

## Durable storage source work

PH-09 is in progress across miner fleet/claim, monitor, validator,
bootstrap/root and the explicit server local-blob backend. PostgreSQL/Redis
and remote MinIO remain separate storage/backup scopes. External deployment
declarations and hashes stay outside previously signed protocol objects.

The unchanged SN `e7543dc7` baseline reproduced three concrete failures:
missing fleet and claim directories became fresh custody, and the standalone
monitor checkpoint published into a replacement parent. The retained causal
log is `baseline-three.stdout.jsonl` under
`/mnt/data/sn-testnet/mainnet-durable-volume-20261002/evidence`.

The shared Connect peer package checkpoint is
`5930a97086733acd9ba5b09ab891dcd1b03be651`, tree
`4decadaced2af5327db835c7c08d49e5eb7cd247`. Its parent `9df4fd87`
introduced exact external declarations, filesystem UUID/mount/marker checks,
no-follow ancestry revalidation, protected precreated roots, independent
pre-provisioned per-root leases and concurrency-safe descriptor joining.
Root-local snapshot admission returns typed busy without blocking an unrelated
root's writer. The Connect parent does not import the new peer.

The successor distinguishes unavailable observations/reserve from proven
identity loss. Two test overlays fail causally on `9df4fd87`: an unavailable
mount observation was permanently classified as identity loss, and a missing
retained descendant was not classified as identity loss. The fixed controls
cover same-owner pressure/observation recovery, permanent descendant poisoning
after restoration and invalid caller inputs. The 22-root author race log
`guard-taxonomy-race.jsonl` hashes to
`3e31a4e4df7d56e21a5f3e3a8c4cd891451152884b6cc1b09f9c6b9894e7201a`;
the two-root old-source failure log `guard-taxonomy-baseline-overlay.jsonl`
hashes to
`af5417461e10f765faf2ae3ad0e466d11abecac270db98f9cadbc83b5f0a0303`.
An earlier pre-overlay invocation selected no tests; it is retained separately
as `guard-taxonomy-baseline.jsonl` and is not a causal pass.

The [independently sealed core receipt](durable-core-independent-20261002.json) is retained byte-for-byte from
`/mnt/data/sn-testnet/sol-connect-durable-core-independent-20261002/receipt.json`,
SHA-256 `dcbe525ec9fd39d556a8b7d441bb58981d1b56a2866e2edab082d2c5cfdb61fc`.
It verifies exact clean `5930a970`, 22 normal and 22 race roots plus vet, and
both old-source causal failures in each mode. Its initial scratch-mode failure
is retained separately; the retry changed fixture preparation, not source.

Core integration commit `6d0eca1e77cc7f84ebd9615ec43cc876201a1f92`, tree
`be4115102ff64241afcbaeeb7e6312788aaa0d42`, merges that qualified core into
current Connect `37153b2b` without replacing unrelated changes. Only the five
`durablevolume` files differ from `37153b2b`; that subtree, `go.mod` and
`go.sum` are identical to `5930a970`. The exact merged head passes another
22 normal/22 race roots and vet. Its nonstandard dependency census contains
only the peer itself. Go 1.26.6 compiler SHA-256 is
`29e6e0b8be61beb1489ceae62b304343566de8a1dc700af74bde7aeb9c80ad45`.
The separate [author integration receipt](durable-core-integration-20261002.json), retained byte-for-byte from
`/mnt/data/sn-testnet/mainnet-durable-volume-20261002/evidence/core-integration-receipt.json`
hashes to `647a5f507c34abc594239252313a62f687bc1ea58cb0ff6839851ed350cb26a6`.
It binds the independent core receipt and exact module/compiler files. It does
not qualify the rest of Connect, additive inventory or downstream adopters.

The [independent integration join](durable-core-integration-join-20261002.json)
confirms peer/module byte identity and binds the independent core receipt.
Compiler binding resides in the separate author integration receipt; this is
not independent compiler verification. The four small receipts linked here preserve their original bytes;
the larger raw logs and referenced source workspaces remain external retained
evidence at the paths recorded in those receipts. This repository copy does not
claim to contain all raw test output or reproduce the compiler independently.

Pre-publication refusal and uncertain publication have different recovery
paths. Missing reserve before a write retains the prior checkpoint; a failed
acknowledgement after write/rename may mean bytes already reached storage.
Keep original signed, pending and completed bytes, join only the affected
owner, and reconcile the retained checkpoint before continuing. Do not reset
the whole campaign or call a copied root an approved replacement generation.

All-owner adoption, finite inventory/restore verification, external
former-writer fences, deployment-unit/config integration and independent
composed qualification remain unfinished. A guarded snapshot alone cannot
prove that older unguarded writers stopped. Local byte verification cannot
prove cross-host or database restoration, authorize signed physical-root
rebinding, or grant restart. No signing, broadcast, live mount, deployment or
production service start is part of this source work.
