# Version 2: handoff to the deploying agent

Written 2026-10-13 at about 23:30 UTC, when deployment passed to another
agent. Updated 2026-10-14 at about 01:15 UTC: sections 1 and 2 follow the
merges since, and items 2 and 3 of section 3 follow the runbook.
[VERSION2-LAUNCH.md](VERSION2-LAUNCH.md) is the runbook and
[VERSION2-FINAL.md](VERSION2-FINAL.md) the plan of record; both win over this
note. This note says what is on `main`, what is still landing, and the facts
found during the last day that the deployer must act on.
[VERSION2-STATE.md](VERSION2-STATE.md) is the earlier orchestration log.

## 1. What is on main

Heads on 2026-10-14 at about 01:00 UTC:

| repo | main |
| --- | --- |
| sn | the commit that updates this note, on `00e383fe` |
| server | `72699c819` |
| sdk | `1ee41b40` |
| connect | `8d22832d` |
| proxy | `ada3aac` |
| localizations | `3a1e7ca` |
| apple | `70f3ce7a` |
| android | `a46813ea5` |
| windows | `e549203` |
| linux | `3332164` |
| mmm (ur.io) | `ec4760983` |
| web | `9526a97b` |
| warp | `99e01ec` |
| xops | `770fffc` |

Settlement v2 itself:

- the contracts (`evm/src/STSettlementVault2.sol`, `STSettlement2.sol`,
  `STClaimBatcher.sol`, `LedgerTree.sol`);
- the server's driver and readers, with migrations 821 and 822, and the
  activation record, with migration 832;
- the validator's audit;
- the claim tools;
- the SDK, and every app.

Network payout consent is in the SDK, all five apps, ur.io and the
localization store.

Deploy tooling:

- `apply --action vault2-create` prints the escrow envelope;
- `sn-mainnet register-operator-pool review|readback` builds, preflights and
  reads back the registration;
- the examples and the fill recipe are in `mainnet/`
  ([SETTLEMENT-V2-FILL.md](mainnet/SETTLEMENT-V2-FILL.md),
  [SETTLEMENT-V2-EXAMPLES.md](mainnet/SETTLEMENT-V2-EXAMPLES.md),
  [SETTLEMENT-V2-ADOPTION.md](mainnet/SETTLEMENT-V2-ADOPTION.md)).

Runtime:

- reviewed sources for runtimes 477 and 478;
- audits in `docs/spec/runtime-477-audit.md` and `runtime-478-audit.md`;
- the rehearsal pinned at 478. R-1 to R-7 pass on 475, 477 and 478 with the
  catalogue's bytecode.

Gas floors for runtime 478. The server caps its two close floors at the
operator gas policy's `maximum_gas`, so a floor never causes a refusal by
itself. No policy stands above the claim floors.

| Sender | Floor | Where |
| --- | --- | --- |
| v2 close | 3,200,000 | server |
| v1 close | 3,200,000 | server |
| v2 claim | 2,600,000 | SDK |
| v1 claim | 2,700,000 | SDK |
| claim tools | 2,600,000 | sn |
| claim gas passed to the wallet, v1 and v2 | (the SDK's) | ur.io |

ur.io passes the SDK's gas with v1 claims too since mmm `37859e302`, and
refuses a claim without it.

The server activation record (SV-9, migration 832): once the server observes
the pool's registration, the st subsystem refuses to run without the
recorded v2 address pair. `st v2-activation` shows the record, and
`st v2-activation-clear` clears one that names the wrong pair.

The server's database head is 835. Upstream added migrations 833 to 835
after the activation record's 832, and `bringyourctl db migrate` applies
them all (runbook step 6).

The root-key stranding fix (server `8d63033c9`, merged as `4e5759875`):
under a gas policy a new nonce is reserved only together with its first
attempt's gas reservation. So an attempt the policy refuses takes no nonce,
and the key's later sends go out.

- The policy still refuses that operation's further tries. Three reverted
  closes of one epoch spend the close's three signatures.
- A key that a build before the fix stranded stays stranded. To free it:
  1. stop the engine's sends for that key: the taskworker, or payout
     admission;
  2. send a 0-value self-transfer of the key at the stranded nonce, from
     outside the engine;
  3. wait until that transaction is finalized, then let the engine send
     again.

  Never delete rows. The log lines are in the runbook's section 5, and the
  steps in its section 6.5.

The dual-stack validator hops, in all four repositories: server `72699c819`,
connect `8d22832d`, sdk `1ee41b40` and sn `00e383fe`. The launch's builds
carry them.

- Deploy order: the server's API and taskworker, then its connect service,
  then the validators.
- `hop_latency_from_attributed_attempt`, a server-only key of verify.yml's
  `settings`, stays off until every validator, third-party ones included,
  runs the relaxed receipt check.
- Roll back in the reverse order, that setting off first.
- sn `VALIDATOR.md`:812-828 ("Rolling deploy") and connect
  `IPV6.md`:332-341 describe it; the runbook's section 6.3 places it in the
  launch.

Release gate repairs:

- preflight module classification and tidiness;
- the 19 stale census pins (the native-stake startup pin follows the current
  code, by the owner's decision);
- the Solidity coverage and static jobs;
- the test vault guard under `GOTMPDIR`;
- the selection of the v1 close floor's tests and of `sn25golive`'s (sn
  `6caf9667`).

Release builder: an explicit `dependency_mode`.

- `local` uses the working checkouts. This launch uses it.
- `pinned` is the build version, made with
  `scripts/mainnet-release-build/pin.sh`.

## 2. Still landing on main

This session keeps merging these and pushes each as it verifies:

1. **Gas floors for the deposit key's stake moves** (server), with the same
   cap. Review holds them until they respect the operation's remaining
   liability.
2. **The apps' v1 gas top-up hint**, rising to 0.02 TAO. The SDK signs a v1
   claim at no less than 2,700,000 gas, and the gas key must hold that limit
   times the price: about 0.0149 TAO at 5 gwei with legacy pricing. The
   apps' `main` still suggest 0.005 or 0.01 TAO.
3. **The server's written recovery of a stranded root key** (section 1). No
   document on server `main` holds it yet.

## 3. Facts the deployer must act on

1. **Mainnet runs runtime 478** since block 9,276,925 (2026-10-13 21:08:36
   UTC); `:code` BLAKE2b-256 is `0x7e722b3aa5c9192c98665a5d65fa91364bf9055e21fc90efce6d59bf4ef414e1`.
   - Step 8's phase config names source `850f2755bb05a3851f948489f0ab27b0645cebce`
     at version 478. `sn-mainnet` refuses any source named for another
     version.
   - Repeat step 2 immediately before steps 8 and 15.
2. **Re-sign the operator gas policy (step 5) before the server build of
   step 6 runs, and so before switching the server to v2 (step 11).**
   - `maximum_gas` must be at least 3,200,000; 3.5M is planned.
   - `maximum_intent_liability_wei` must be at least 3 × `maximum_gas` × the
     fee cap: 3.15·10^17 wei at 3.5M gas and 30 gwei.
   - Under today's 1M policy the floors sign closes at 1M, and nothing new is
     refused.
3. **Re-sign the native-fee settlement policy for runtime 478.**
   - The policy binds the runtime code hash and a native block range
     (`model/st_native_fee_settlement_model.go`, `policy.RuntimeCodeSha256`).
   - Statements of the blocks that 478 executed, from block 9,276,926, are
     refused as "outside its approved denomination" until the policy names
     478.
4. **Deploy ur.io from mmm main** before any app release that carries consent
   signing. It carries the wallet bridge security fix (`64173fd13`), the
   consent bridge page and the claim gas.
5. **Build the apps.** No repository has CI.
   - Apple and Windows have never compiled the v2 and consent code.
   - Android's unit tests ran against a stub SDK; build its AAR from sdk main.
   - On Windows, confirm the long consent link opens in the https browser that
     holds Talisman, and that wallet sign-in still works (sign-in sessions are
     now final once cancelled).
6. **Step 1 and the release build** use Go 1.27.2 (`go.mod`) with
   `dependency_mode: local`.
   - The census includes connect, sdk and gvisor.
   - The source freeze requires gvisor cloned from
     `github.com/urnetwork/gvisor`, with that remote, on its `go` branch at
     `c0783dba`.
7. **The release gate (step 4).**
   - The workspace needs the vault and xops checkouts, GitHub, the Go proxy and
     the LAN node. Every release repository must be clean, on a branch
     tracking `origin/main`, and at exactly `origin/main`.
   - Refresh the release lock as the last commit after the freeze
     (`sim-testnet/README.md`, "Atomic release-lock refresh"). The owner has
     authorized it.
   - Still failing on pristine main, for an unfound reason:
     `TestProduceFinalSemanticOutputs{WritesOneSealedPair,ScansBeforeCreatingFiles,ScannerCannotMutateVerifiedBytes}`
     ("measurement-derived pool decision no_id 1 differs"), in sn-all-normal
     and sn-simulator-race.
8. **The schedule.**
   - E_s is epoch 2, which starts at block 9,297,628, about 2026-10-16 18:10
     UTC.
   - The plan's registration window is blocks 9,283,228 to 9,290,428.
   - The six departures in the runbook's section 9.3 need the owner's explicit
     acceptance. If the window is missed, E_s moves to epoch 3.
9. **The safeguard's first read.** The activation record arms on the first
   successful registration read. Check it with `sn-transition-status` or
   `st-check --v2-activation` after step 15, and never remove the v2 addresses
   after it.
