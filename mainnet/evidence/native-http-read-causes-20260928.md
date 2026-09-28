# Native configured HTTP read cause preservation, 2026-09-28

Author workspace: `/mnt/data/sn-testnet/worktrees/native-http-read-causes-20260928/sn`,
based on qualified root `b00d1e54be93a5f47d047810dd76ed15744d66fa`.
Evidence and Terra runner:
`/mnt/data/sn-testnet/evidence/native-http-read-causes-20260928/`.
The physical sibling replacements come from the frozen
`server-model-final-20260927` graph; consumed modules/files and clean Git heads
are retained separately in the evidence manifest. No frozen installer candidate
or previous test capture is changed.

## Root cause and bounded change

The pinned `github.com/centrifuge/go-substrate-rpc-client/v4` version
`v4.2.2-0.20240919131012-e3b938563803` in `gethrpc/http.go` returns non-2xx status
through `errors.New(resp.Status)` and later text formatting. Its success decoder
returns EOF without retaining whether the physical read or complete JSON failed.
Inspection of the local pinned module and actual configured-client tests ground
the change; no replacement module or fork is used.

`DialChainContext` uses the library's existing `DialHTTPWithClient` hook for HTTP.
A private context marker is set only by the existing allowlisted native read
owner. Its instance-owned transport retains numeric HTTP statuses and physical
round-trip/body errors before GSRPC can flatten them. It reads and closes the
complete physical body before admitting JSON decoding, so a valid JSON prefix
cannot conceal interrupted framing. A 32 MiB finite response bound exceeds the
existing 17 MiB runtime-code and 21 MiB block wire bounds; oversize replies stop
hard and are not silently truncated or retried. Standard HTTP decompression is
bounded at its decoded body reader. A separate body-close failure stays hard.

No new retry loop is added. The existing 300-second owner, 60-second attempts,
argument freezing, caller cancellation and shared capacity handling remain in
charge. Only 408/425/429 and 5xx statuses are transient. An empty 2xx body is an
absent physical response; nonempty fully received malformed JSON, permanent
JSON-RPC errors and redirects remain hard. Writes and unknown methods bypass
the decorator and cannot acquire the read marker or a new attempt budget.

The public `RetryableSubstrateReadTransportError` requires actual configured
read origin and an entirely transient error tree. Joined owner deadlines retain
the origin; joined integrity, local file, close or cancellation failures refuse
retry. Bare EOF, timeout or status text grants no authority. Existing websocket
reconnect-marker compatibility remains private and is not used by this HTTP
classifier. Runtime, canonical header/body and receipt checks are unchanged.

## Deterministic qualification scope

Eleven new `TestSubstrateReadHttp` roots cover:

- Actual configured 500/502 recovery and unchanged pinned parameters.
- Persistent 502 through one virtual five-minute owner and structured cause.
- Empty body, incomplete physical JSON and complete JSON with broken framing.
- Complete malformed JSON, result type errors and permanent JSON-RPC failures.
- Permanent statuses/redirects and caller cancellation during a physical body.
- One-call write/unknown-method failure with no read retry owner.
- High-level `DialChainContext` metadata recovery before genesis/runtime reads.
- Body-close/size failures, mixed integrity joins and diagnostic lookalikes.

Time control replaces only the existing retry clock/pacing hooks; HTTP requests,
GSRPC decoding and production constructor routing are real local fixtures.
Explicit response/cancellation barriers establish ordering. The selected body
also includes prior `TestSubstrateRead`, native capacity and `TestDialChainContext`
regressions affected by the shared classifier/constructor. The collector retains
all independent normal/race failures and only counts causal controls when they
reach their exact intended assertions.

Author checks are `go test -mod=readonly -c ./crv4`, `go vet -mod=readonly ./crv4`
and formatting/diff checks. **No test bodies ran in the Astra author lane.**
Normal/race and causal qualification are pending Terra. No live route, live key,
signing, broadcast or node mutation is used. Production continuation integration
with the exported classifier belongs to its separate owner and remains pending.
