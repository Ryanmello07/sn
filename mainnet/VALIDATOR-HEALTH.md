# Bounded validator proof and worker observation

`sn-mainnet activate-validators admit-health` uses the original independently
signed activation envelope and its permanent operation allowance. It accepts
the same `--approval`, `--accept-approval-hash` and `--independent-public-key`
arguments as the other admission modes. Both static units must already be
installed. The command only reads chain, operator and host evidence and writes
its existing activation custody journal. It cannot start a service or sign.

The observation composes the qualified native checkpoint, current executable
and contract views, both roles' registered operator client keys and conservative
stake-capacity census at the same current native/EVM point. It then replays each
role's two **original pinned activation histories** through the real signed
record, trail and EMA verifier. Actual historical native eligibility, activation
publication, initial policy windows and every legacy terminal boundary are read
through the existing canonical native/EVM readers. The current client-key
response census remains a separate committed domain; it does not supply old
proof keys or replace the original activation.

This bounded increment proves the approved historical prefix. It does **not**
inventory the service UID's current mutable measurements, settlement closures,
unfinished trails, steering intents or successor generations. The existing live
history reader requires ownership by that service UID; root admission does not
weaken that guard, copy mutable state into a new authority, or interpret missing
files as empty history. Only a canonical, explicitly empty, pinned history can
prove a pristine prefix. `current_mutable_prefix_proven` remains false.

The proof reader admits exactly two operators, at most 16 MiB of combined pinned
history and 256 legacy closures per member, in addition to the original signed
runtime limits. Public verify-key replies are capped at 256 KiB. Historical RPC
attempts have a 60 second budget and one 300 second retry owner; verify-key GETs
use the existing 60/300 second owner. A pure timeout may retry against completed
mathematical work. Mixed transport/integrity errors and cancellation stop.
The enclosing original route deadline and sample-age limit remain authoritative
and can shorten these budgets; the command never extends a signed allowance.

Each completed role is synced separately in `completed_proof_checkpoints`, with
its original observation clock, exact config bytes, policy, operator domain,
generation, root and EMA digest. A later role/read refusal preserves completed
checkpoints. They are retained history, not a current readiness cache. A later
command performs fresh current reads and cannot substitute a different original
prefix. Ordinary `admit`, `admit-evidence` and `admit-stake` retain their scopes.

Worker observations consume the actual standard `urnetwork-validator-progress-v1`
file through protected physical paths and the approved service UID. Its progress
source hash is checked separately from the exact config-file hash. Acknowledged
manager invocation, source, instance, original start time and semantic progress
clocks must agree. A stopped fresh role can have no progress. Missing/stale
progress, publication retry and receipt/RPC wait are operational warnings;
wrong ownership, aliases, changed source/generation, rewritten protocol ages,
future clocks and reported hard errors refuse the observation. Read failure
retains the previous successful record without refreshing any protocol age.

The standard record exposes process, native scheduler, settlement and steering
health. It has no independent per-operator live worker attestation. The result
therefore retains explicit gates for current mutable-prefix completeness,
per-operator worker health, global signer custody, applied weights influence and
signed launch authority. Public `start` still supplies no activation authority.
Passing this command is not launch authorization.

Qualification uses synthetic signed fixtures, actual local HTTP/RPC transports,
protected files and deterministic deadline/cancellation transitions. No physical
device, deployed mainnet contract, live operator or production service was used.
