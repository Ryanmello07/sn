# Production startup continuation diagnostic candidate

This is an unqualified source/fixture checkpoint, not an acceptance result.
The prior continuation and native HTTP candidate scopes remain independently
qualified or pending in their own immutable captures.

The actual `RunRelease` path now initializes native observation at the signed
activation block, replays real activation/disk/original intent authority, then
reconciles retained native liability before requesting current preparation.
Fresh UID/stake and real settlement publication still gate every new intent and
trail worker. Local semantic history uses caller cancellation rather than the
five-minute read deadline; actual RPC/HTTP owners retain finite read budgets.

The first diagnostic root is:

```sh
go test ./validator -run '^TestProductionStartupRunReleaseReconcilesBeforeCurrentPreparation$' -count=1 -timeout=300s
```

It builds independently signed zero-price continuation inputs, selects genuine
activation digests before compact sealing, preserves real M8 proof/journal replay,
serves exact native receipt/events/application rows and journal ABI state, and
starts the public binary path with both actual local HTTP/session/upload owners.
An explicit current-EVM request barrier must occur after the original pending
intent reaches its actual applied row. Latest metadata is unavailable throughout.
Exactly one original send is retained; the test does not claim paid capture.

Compile-only `go test ./crv4 ./validator -run '^$'` passed before this checkpoint.
No author-lane test body has run. Terra must diagnose the exact frozen revision.
Remaining checks include full-root outage/restart and fresh-deployment startup,
parallel mixed read-cause composition, cancellation/integrity controls, and normal
and race qualification. Both live operator server-key/public-object/session routes
remain startup dependencies; this candidate does not claim offline operator recovery.
Bounded durable receipt-prefix reuse and historical-only foreign-nonce resolution
remain separate open work.

The first normal diagnostic at `1ffa1f34` failed in fixture construction: its
native route used HTTP, while production native submission uses WebSocket
`author_submitAndWatchExtrinsic`. The corrected fixture serves genuine native
JSON-RPC over an explicitly signed WebSocket route. No config/approval gate was
widened, no route is inferred or converted by production code, and HTTP read
constructor support still grants no subscription/signing authority. Both operator
API routes and the EVM route remain actual HTTP endpoints. This fixture correction
requires its own diagnostic; the original failed capture remains evidence.

The WebSocket diagnostic at `af2a6603` passed the route boundary but refused the
fixture's config signature after YAML loading. Fixture input now follows the
existing production-runtime fixture discipline: round-trip and normalize the
public YAML representation before the independent approval is signed. Production
hashing, signature checking and retained-authority rules remain unchanged.
