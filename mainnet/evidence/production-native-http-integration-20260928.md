# Production native HTTP integration — 2026-09-28

Status: source candidate; **qualification pending**. Astra authored source and
fixtures; Terra owns normal/race execution. Compile-only checks do not qualify
runtime behavior. No live endpoint, key, submission, deployment, simulator or
release lock was touched.

## Prerequisites and narrow changes

This slice follows continuation `89ebd498` and its receiver-only follow-up
`18ff77cb`. Native adapter `ec2bc584` was imported as `f35a3ab2`; the parent reports
Terra's original component normal/race and ten intended causal controls passed,
then integration as `790dacb0`. Those frozen candidates were not edited.

Production phase classification now consumes the configured native client's
strict typed origin. A separate presence query prevents generic URL/EOF
unwrapping from overriding the native owner's hard verdict on a mixed or local
custody error. Text, unowned EOF and permanent protocol/application errors do not
gain read authority.

The 32 MiB transport ceiling omitted JSON framing around a previously admitted
16 MiB event value, which becomes 32 MiB plus `0x` on wire. The finite ceiling now
derives from that same event bound plus 64 KiB of framing. This changes transport
admission only, without increasing the event decoder's bound or approving event
semantics. An oversized declared response remains hard.

The original adapter treated every HTTP response-body close failure as hard.
That was unnecessary for a pure typed connection timeout, EOF or truncated close
from an allowlisted read. The response is released exactly once, idle connections
are discarded, and the attempt context is canceled before another request. Only
the complete transient error tree may retry. A local file close, explicit
cancellation, unknown release defect or any joined integrity cause remains hard.
Writes and unknown RPC methods still do not enter this read transport.

## Deterministic evidence to collect

Use physical `GOCACHE=/mnt/data/sn-testnet/gocache`, a capture-specific physical
`TMPDIR`, `GOWORK=off` and `GOMAXPROCS=2`. Enumerate roots before normal/race bodies.

- `go test ./crv4 -run '^TestSubstrateReadHttp' -count=1`
- `go test ./validator -run '^TestProductionSteering(NativeHttp|ReadCauseClassification|ReadBudget|ReadRejects|Loop)' -count=1`
- The same selectors with `-race`; `go vet ./crv4 ./validator`.

The actual configured native constructor reads synthetic metadata, genesis and
runtime over local HTTP. A later response reaches an explicit body barrier before
the existing phase clock advances its requested 60-second deadline. Its genuine
physical error returns through GSRPC; the 300-second owner exhausts while the
caller stays live. The resulting receipt wait preserves original epoch/hash,
remains visible and does not spend the loop's hard-error budget. Mixed integrity
and a complete malformed response remain hard. Clock hooks change no production
budget or public API, and no injected callback grants an RPC success verdict.

The close fixture decorates only the physical RoundTripper beneath the real
configured adapter. It checks one close per response, idle discard before reuse,
the original read budget and recovery from EOF, unexpected EOF and typed network
timeout. File, canceled and mixed close causes are negative controls. A maximum
event hex field goes through the actual configured HTTP client; its decoder's
event semantics are intentionally outside this transport test.

Required causal controls restore the previous 32 MiB limit, previous hard-for-all
HTTP-close verdict, and rejection of native origin at the phase boundary. Each
must reach its named behavior assertion, not fail compilation or setup. Preserve
normal/race logs and source fences separately from prerequisite qualification.

## Limits and naming-only changes

This qualifies physical native deadline composition; the original component
separately covers full status/EOF budget exhaustion using its own deterministic
clock. It does not claim a 300-second wall-clock HTTP500 outage in the validator.
Full real `RunRelease` activation/config/dual-upload composition, bounded durable
receipt prefixes, the historical-only renewal foreign-nonce path and miner
partial-range checkpoints remain open. No new signing or epoch authority is
created.

The prior continuation test's eleven positional case literals now name the
same `name`, `err` and `want` fields. Values, ordering and assertions are unchanged.
The receipt-method receiver rename was already isolated in `18ff77cb`; neither
naming-only edit is a new behavioral qualification claim.
