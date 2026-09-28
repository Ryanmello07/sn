# Contract offline recovery and approval preparation follow-up

This follow-up is based on `576dea586ba30e08fce6f3c7923b9717cc320b54` in
`/mnt/data/sn-testnet/worktrees/mg08-bootstrap-contracts-preview-20260928/sn`.
The preceding candidate and all its Terra captures remain frozen. The evidence
directory is `/mnt/data/sn-testnet/evidence/mainnet-bootstrap-contracts-preview-20260928`.

## Retained terminal status

Previously an offline command reopened a completed journal but always reported
`signed-custody-complete`. It now retains `reserve-created` or
`create-reverted-nonce-consumed`, including the unchanged original receipt and
attempt count. `receipt_observation` distinguishes `retained` from
`revalidated-online`; neither claims complete installation or activation.

`TestEvmCreateOfflineResumeRetainsCreatedReceipt` and
`TestEvmCreateOfflineResumeRetainsRevertedReceipt` exercise actual command,
private disk, local HTTP and reviewed geth EVM execution before reopening
offline. The latter uses a bounded low-gas original transaction to produce a
real failed CREATE with no installed code and its original nonce consumed.
Both compare receipt facts, original transaction, attempts, journal bytes and
all RPC counters after the offline reopen. The deterministic causal control
removes only the terminal-status projection; the intended assertion is
`offline reopen downgraded or replaced terminal custody`.

Author checks are compile-only and vet. Normal/race bodies and causal controls
belong to the separate Terra qualification lane and are pending. No live key,
signing, network write or deployment is included in this evidence.
