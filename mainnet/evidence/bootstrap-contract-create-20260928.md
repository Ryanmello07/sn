# Reserve CREATE candidate, 2026-09-28

This candidate adds executable `bootstrap-contracts plan/apply/resume` for the
first reserve CREATE, exact release-artifact export, public signed-byte custody,
finite original-byte HTTP submission and canonical inclusion/runtime/getter
recovery. It shares only fixed-route HTTP mechanics with existing native root
submission; their action/signature/journal authority remains separate.

Authoring workspace:
`/mnt/data/sn-testnet/worktrees/mg08-bootstrap-contracts-20260928/sn`, based on
`bd95a8f46ae632680da17e5c8aa831843c85bf02`. Evidence and Terra runner:
`/mnt/data/sn-testnet/evidence/mainnet-bootstrap-create-20260928/`.

Author checks: readonly `go test -c` for `./mainnet` and
`./sim-testnet/gencontracts`, plus `go vet` for both packages. No test bodies were
executed by the Astra author. The actual graph adds eight already selected geth
transitive modules needed by the real EVM fixture; existing versions and
`go.sum` are unchanged. Solidity, generated bindings, reviewed bytecode and
contract build inputs are unchanged.

The original `c10a2cac` qualification **failed before EVM execution**: the fixture
encoded its approval signature with `0x`, while the canonical parser requires
128 unprefixed lowercase hexadecimal characters. The retained original positive
and causal captures remain in the original evidence directory; the first causal
failure did not reach its intended signed-nonce assertion and is not counted as
causal qualification. The correction lives in the separate worktree
`/mnt/data/sn-testnet/worktrees/mg08-bootstrap-contracts-fix-20260928/sn`, with
evidence under `mainnet-bootstrap-create-fix-20260928/`. It corrects the fixture,
documents the distinct key/signature encodings, adds explicit positive/negative
wire and downstream-admission tests, and checks fixture signing/serialization
errors. The release JSON fixture also preserves its testnet-only generated
variable names so the retained-bytecode test reaches preservation instead of
panicking during setup. No existing native root wire contract changes.

Corrected Terra qualification is **pending**. The affected rerun selectors are
`^TestEvmCreate` (20 roots, including the two new wire roots) and
`^TestReleaseJsonUsesRetainedReviewedBytecode$`, plus vet and command build.
The unchanged native root submission and other generator bodies retain their
independent original normal/race evidence; failed package totals are not passes.
Six causal overlays restore omitted exact-signature checks,
omitted runtime readback, a send before durable attempt publication, an
unavailable-read-as-mismatch label, rejection of healthy head progress, and a
discarded completed scan checkpoint. The controls must fail their intended
assertions before qualification can be sealed.

Eighteen new EVM roots exercise the real command, exact maintained artifact,
genuine geth constructor/getters, local HTTP route, unequal native/EVM heights,
uncertain response, immutable finite replay, wrong authority and chain inputs,
runtime/getter/receipt/first-insertion corruption, expiry and historical upgrades,
initial-claim/attempt/output interruptions, canceled waiters, closed storage,
input pins, advancing heads and bounded historical recovery. Three export roots
cover the production census and preservation of reviewed payload bytes.

The retained source/component manifests distinguish this isolated SN candidate
from its physical frozen sibling dependency worktrees. This is component
qualification, not a new whole-release composition or a mainnet deployment.
No live key, signature, RPC mutation, chain deployment or service activation was
used. Full installation, Safe anchoring, bootstrap reset, production activation
and authenticated live trust inputs remain separate required work.
