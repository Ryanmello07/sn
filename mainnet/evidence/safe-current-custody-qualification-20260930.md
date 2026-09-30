# Safe current-policy custody qualification

**Qualification pending.** Focused streams are passing; the complete adjacent
and causal matrix and final independent manifest are still required. This note
is an isolated documentation draft and authorizes neither integration nor policy
activation. Public submission remains closed.

Frozen implementation is `3f88a9484c639e999081cf8a2b8a8f2cf14ed536`, tree
`c4cf3de3ad382b6894e4bb9081f4120e13a06458`, based on qualified shared
`af501f6ea21113af81393f3174f0babda2526058`. The implementation worktree is
`/home/by/urnetwork/sn-successor-safe-current-custody-owner-20260930`.
Documentation composition will preserve every non-Markdown byte of the frozen
source, all module files, the SDK oracle and earlier qualification receipts.

## Property and limits

The [proposal](../BOOTSTRAP-SUCCESSOR-SAFE-CURRENT-CUSTODY.md) adds a separate
domain-signed acceptance around the current-only signed proposal. The original
independent approver signs both. Exact predecessor, distinct evidence and
monotonic retained runtime-prefix requirements preserve the immutable original
execution and history statements. Old omitted event fields retain their original
bytes and seals. Current proof never claims complete history was proved.

Counted attempts name the exact completed policy revision. Terminal outcomes
keep the policy of their counted attempt even if a later revision arrives.
Original Safe/relayer signatures, nonce claims, attempt counts and maximum
liabilities cannot be reset by imports. Hash-bound partial stages fix one
acceptance, including an empty stage; publication failure closes the owner.
Interrupted counted attempts remain consumed, while partial outcomes block new
runtime/policy imports until canonical recovery. Reopen and live checkpoints
reauthenticate the complete journal and reject lost, forked or changed authority.
This remains local file custody, without an external monotonic rollback log.

This slice has no public import flag or production current-policy capability.
Both generic-owner and direct-adapter writes refuse such custody. Historical
reconciliation remains available. A separately qualified native capability and
explicit approval/route installation remain P0 gates; the complete-history
interface and signed original statement are not weakened. Automatic compatible
runtime admission is also still open: signed additive revisions are incremental
authority, not automatic runtime compatibility.

## Author handoff

Astra max authored implementation, debugging and compile-only/vet checks. Sol
medium owns behavioral qualification. Formatting, compile-only and vet pass;
all eight isolated mutations also compile and pass vet. No author behavioral
test, live RPC, real signature or transaction was performed.

The handoff is `/tmp/safe-current-custody-handoff-20260930`. Its immutable
14-file `PAYLOAD.SHA256SUMS` hashes to
`5a778e42e1e45dd1111bfd6004de5d4396bb1777df2a42aa94af99fa2b705788`.
The supplementary 32-file `SHA256SUMS` hashes to
`26aafa1ed1d3e8c080dfb79faa9604a1ad6a07f72be27f0d711fa217aa292767`.
`CONTROLS.json` hashes to
`31a91fa8e56d5b7e84675ad2a92ba3fec15eeaf01a55eb2248a087a7a9a933c7`;
`TEST-PLAN.json` hashes to
`feab9db105d26e3ba7672fde1376078cd4a8af0563ec2e77eb46f9aa3dadb547`;
`SOURCE-FENCE.json` hashes to
`61b0818629d18b1e6a603879169d2d5c36644b4a78d0084d14a1a18921309b6a`.

## Independent matrix awaiting final seal

Raw evidence is under
`/home/by/urnetwork/temp/safe-current-custody-validation-3f88a948`.
The final independent manifest and author read-only audit are pending.
The required matrix is eight new roots, thirty adjacent light roots and one
full original-graph canonical adapter root, each normal and race: 78 positive
root executions when complete. Environment is `GOMAXPROCS=2 GOPROXY=off`,
`-p 1 -count=1 -v`; each group keeps its separately bounded harness timeout.
Production retry windows are unchanged.

| Control | Barrier | Normal | Race |
| --- | --- | --- | --- |
| `acceptance_signature` | Separate independent acceptance signature. | Pending | Pending |
| `acceptance_predecessor` | Exact immutable signed predecessor. | Pending | Not selected |
| `counted_policy_reference` | Every counted policy authority remains retained. | Pending | Pending |
| `partial_policy_attempt` | Partial policy blocks a counted attempt. | Pending | Pending |
| `pending_policy_outcome` | New authority cannot change partial terminal recovery. | Pending | Not selected |
| `exact_outcome_policy` | Outcome retains the exact counted policy. | Pending | Pending |
| `policy_history_snapshot` | A live owner detects removed authority suffixes. | Pending | Not selected |
| `unavailable_policy_capability` | Signed custody cannot supply a send capability. | Pending | Pending |

Exactly eight normal and five selected race controls are required. Each must
reach its named assertion, selected root/package FAIL and process exit one,
without a build, panic, timeout or race confounder. No result is claimed for the
three unselected race mutations. The final note must replace pending claims with
exact completed evidence before integration; earlier receipts remain immutable.
