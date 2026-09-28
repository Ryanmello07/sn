# Contract graph candidate: retained failure and review block

The separate candidate `75ae5c14a9fb94644fc20c44f6e92235ddcd2db0` is frozen at
`/mnt/data/sn-testnet/worktrees/sn-mainnet-contract-graph-20260928/sn`.
It is not integrated into the selected production branch. The exact shared
cadence source is already integrated separately as `a58878ea`.

The public nine-action command passed normally at `b2fb2cdd` in 33.71 seconds
and under race at `75ae5c14` in 163.36 seconds. Those are scoped local results,
with genuine Safe execution and a declared model of the native registration
precompile. They do not prove live native rollback or mainnet deployment.
Raw evidence is under
`/mnt/data/sn-testnet/evidence/mainnet-contract-graph-20260928`.

The frozen affected selection remains incomplete. Its normal shard 9 retained
three passes and one failed root,
`TestEvmGraphOuterSuccessCannotHideSafeInnerFailure`. The neighboring
`TestEvmGraphLostRepliesRetainEveryCompletedAction` passed, including recovery
of the original final transaction after a receipt-reader failure.

The failed fixture expected a lower signed `safeTxGas` to force inner
out-of-gas. The vendored Safe v1.4.1 source instead forwards nearly all remaining
gas when reimbursement `gasPrice` is zero, as this profile requires. The actual
call therefore succeeded. Its signed gas field is a reserve/minimum guard in
this mode; the approved outer transaction gas limit supplies the overall gas
ceiling. The test does not yet establish correct handling of a genuine inner
failure. Preserve its original result rather than relabeling it as a pass.

Automatic review stopped Astra's attempted correction of this failure fixture,
citing possible cybersecurity risk. No correction was implemented or tested.
The new correction worktree was created at unchanged `75ae5c14`; the original
candidate, isolated control source and existing captures remain intact. Do not
retry or route that rejected correction through another agent or tool.

Finish collecting only the already admitted frozen qualification, retaining
its failures and successful scopes. The correction, affected qualification,
production integration and live deployment gates remain open. Independent
receipt recovery and diagnostic-export implementation can continue.
