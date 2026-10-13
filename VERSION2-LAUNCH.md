# Version 2: launch plan

Prepared 2026-10-13. This is the runbook for deploying and activating
settlement v2, phase A of [VERSION2-FINAL.md](VERSION2-FINAL.md), on SN25
mainnet. The owner runs it. The plan of record wins wherever the two differ.

Pins. The first round of merges is done and pushed (STATE.md:276-281,297).
These `main` heads are the runbook's premises, and its file and line
citations hold at them:

| Repository | `main` head |
| --- | --- |
| sn, this repository | the revision that holds this runbook |
| server (`server/`) | `548a90e0` |
| sdk (`sdk/`) | `7b024e09` |
| connect | `f7318bca` |
| localizations (`localizations/`) | `1dc9045` |
| apple | `2e9cc9f8` |
| android | `8566ba53c` |
| windows | `c1f10c5` |
| linux | `faba790` |
| mmm | `64173fd13` |
| web | `9526a97b` |

The second round is under way (STATE.md:298). sn `v2/launch-tools`
(`c296b255`) is merged. Still to merge are sn `v2/launch-examples`, server
`v2/launch-latch` and the consent branches, mmm's at `9e2e8c2b5`. Step 1
freezes whatever `main` holds after it.

The orchestration log is [VERSION2-STATE.md](VERSION2-STATE.md), cited as
STATE.md. It is a snapshot of 2026-10-13 20:15 UTC; its lines 1 to 286 keep
the log's numbers. Nothing here has been run against mainnet. Unprefixed paths
are in this repository.

Conventions:

- **Owner** is the owner, with the Ledger-held keys
  ([mainnet/LAUNCH.md](mainnet/LAUNCH.md):161-166). **Operator** is whoever
  runs the UR server, the validator host and the tooling.
- A bare § cites VERSION2-FINAL.md. This runbook's own parts are called
  sections and steps.
- `O1` to `O26` are owner inputs (section 3). A step does not start until its
  inputs are fixed.
- Each step gives **Who**, **Do**, **Expect** and **Stop if**. To stop is to
  change nothing further, keep every journal and output, and decide with
  section 6. The steps the server and the validator run by themselves give
  **Act if** instead.
- Blocks are counted as the contracts count them. A mainnet epoch is 50,400
  blocks, the commit window 1,200 and vault2's challenge window 14,400
  (VERSION2-FINAL.md §5). At 12 seconds a block these are 7 days, 4 hours and
  48 hours (LAUNCH.md:29-32; WHITEPAPER.md §8.3).
- A gap is something the repositories do not settle. Each is marked **Gap**
  and listed again in section 8.

## 1. Purpose and scope

### 1.1 What ships

- **Contracts**, beside the unchanged v1 set (VERSION2-FINAL.md §7):
  - `STSettlementVault2` ("vault2", `evm/src/STSettlementVault2.sol`),
    immutable, which holds the funds;
  - `STSettlement2` (`evm/src/STSettlement2.sol`) behind an `ERC1967Proxy`,
    UUPS, owned by the governance Safe;
  - optionally `STClaimBatcher` (`evm/src/STClaimBatcher.sol`), immutable,
    ownerless and stateless (§10.2).
- **Deployment tooling**
  ([mainnet/BOOTSTRAP-CONTRACTS.md](mainnet/BOOTSTRAP-CONTRACTS.md)):
  - the settlement v2 phase of `sn-mainnet bootstrap-contracts` ("Settlement
    v2 contract phase");
  - `sn-mainnet register-operator-pool`, which reviews and reads back the
    activation call ("Settlement v2 activation").
- **Operator server** (VERSION2-FINAL.md §8):
  - the v2 epoch driver and the ledger builder;
  - `GET /sn/ledger/object` and `GET /sn/ledger/account`, public
    (`server/api/api.go:337-338`);
  - the v2 readers: the earnings email, `/account/epochs` and the event
    mirror (§8.4);
  - the commands `v2-unrecoverable`, `v2-rebuild` and `v2-rebuild-withdraw`
    (§8.6);
  - migrations 821 and 822, new tables only
    (`server/db_migrations.go:10013-10049`);
  - five alerts (`server/controller/st_ledger_v2_driver.go:50-56`).
- **Validator** (§9, §13): the optional `settlement_v2` config block, the
  ledger audit, approvals, challenges and demands, the shared weight resolver
  and the "settlement v2 adoption" successor class.
- **Provider tooling** (§10): `provider claim-daemon-v2` and `snclaim v2`.
- **SDK and apps** (§11): v2 balances beside v1, and v2 claims.

### 1.2 What "on" means

Settlement v2 passes through three states, in this order.

1. **Shipped, v2 off.** The new binaries run with today's configuration.
   - The server keeps v2 off while `settlement_v2_address` and
     `vault_v2_address` are absent (`server/st_ledger_v2_config.go:11-17`).
   - The validator reads nothing for v2 without its `settlement_v2` block
     (`validator/settlement_v2.go:44-51`). Today's `validator.yml` and history
     bundle hash identically under the new binary (VERSION2-FINAL.md §13).
2. **Configured, not active.** The server and the validator name the deployed
   contracts, but no pool is registered.
   - The server's v2 driver runs only the prune check, and the v1 driver keeps
     every epoch (`server/controller/st_ledger_v2_driver.go:255-280,308-325`).
   - The validator weights every epoch of an unregistered pool through
     `operatorAt` (§9.1).
   - The apps show the v2 balances the ledger returns, of which there are
     none yet, and offer no v2 claim until `Vault2Address` is pinned
     (`sdk/sn_chain.go:83-88`).
3. **Active.** Phase A activates when the owner Safe sends
   `registerOperatorPool` (§14). That call is the commitment point (§12.1
   step 5). From its `firstEpoch`, called E_s:
   - `settlementAt` returns vault2 and the v2 pool hotkey (§4.3 S4;
     `evm/src/STSettlement2.sol:99-108`);
   - the validator weights the v2 pool UID (§9.1);
   - the server's v2 driver closes, commits, finalizes and answers demands,
     and the v1 driver never closes or commits an epoch at or after E_s
     (§8.1; `server/controller/st_ledger_v2_driver.go:255-280`);
   - epoch E_s − 1 completes on v1 in its normal windows (§12.1 step 7).

In this runbook "on" means state 3. The first v2 funds move at the close of
epoch E_s, within 1,200 blocks of its end. The first claimable balances appear
when that root finalizes, at least 14,400 blocks after its commit (§5).

### 1.3 Not part of this launch

- **Later phases** (VERSION2-FINAL.md §16). Policy v1 stays in force (§2.2).
  The v1 contracts, deployment id, evidence journal, `verify.yml`,
  `provider_work.yml`, the policy file and the launch readiness digest are
  unchanged (§13).
- **Network payout consent** ships on its own. The owner chose network payout
  consent signing in the SDK and every app (owner decision, 2026-10-13). It is
  VERSION2-FINAL.md §11.1. One sr25519 signature by the coldkey, requested by
  the network's owner, covers every provider client of the network.
  - It is independent of activation and is not an activation gate (§11.1).
  - The operator accepts only a prospective consent: its `from_epoch` must
    exceed the current epoch at the finalized block. A consent therefore takes
    effect from the epoch after its signing, and epochs before a network's
    first consent stay unpaid (§11.1, "Earnings range").
  - v1 also pays consent-selected wallets (§2.1, decision 4). A consent signed
    before E_s therefore counts for the v1 epochs from the one after its
    signing, and then for v2.
  - v2 stays consent-only (§3; owner decision, 2026-10-13). Until its network
    signs, an app-only provider stays unpaid, in v2 as in v1.
  - Recommended: step 7's app release carries consent signing, so network
    owners can sign at least one epoch before E_s and be paid from E_s.
  - Every native app signs the consent through ur.io's bridge page, so ur.io
    is deployed before, or with, any app release that carries consent signing
    (STATE.md:268). It needs the consent bridge page (mmm `v2/consent-signing`,
    `9e2e8c2b5`) and the bridge fix (mmm `main`, `64173fd13`; STATE.md:297)
    (step 7).
- **The legacy USDC reconcile** is a separate, adjacent track. Its run order
  is:
  1. `bringyourctl payouts correct-legacy-attempts`, dry run, then `--apply`;
  2. `bringyourctl payouts reconcile-legacy-attempts`, dry run, then
     `--apply`;
  3. wait until the sender completes every attempt those two scheduled;
  4. `bringyourctl payouts sn-reconcile`, dry run, then
     `--send --expect=<digest>`.

  The source is the runbook on server branch `v2/sn-reconcile` (`513d600d`,
  `SN-RECONCILE.md`:40-59) and STATE.md:211-212. The track meets this launch
  in one place. sn.yml is parsed with known fields only
  (`server/provider_payout_transition.go:57-58`), and that branch's parser
  has no v2 fields. So run the reconcile before step 11 adds the v2 keys to
  sn.yml, or with a binary built from a revision that has both. This is an
  inference from the code, not a documented rule.
- **The dual-stack validator change** (`v2/dual-stack-hops`) is still in
  progress. The merge into `main` excluded it, and it stays out of scope
  until it is finished (STATE.md:244). It has its own rollout order: the
  server's API and taskworker first, then its connect service, then the
  validators, with providers and clients in any order (connect `IPV6.md`
  lines 312-317 on that branch). Its egress-index "v2" and "v3" are unrelated
  to settlement v2.

## 2. Preconditions and gates

Every item of sections 2.1 to 2.7 holds before step 5, the first step that
changes production. Gates 1 to 4 are VERSION2-FINAL.md §14's. Gate 5 (software
released and contracts deployed) and gate 6 (approvals) complete inside the
runbook, at steps 9 and 12.

### 2.1 Code merged to main and deployed

- The owner's instruction of 2026-10-13 was "Commit changes. Merge branches
  into main. Pull and push." (STATE.md:244). The first round is merged and
  pushed, without force, after a fresh fetch and a fast-forward check
  (STATE.md:278-281). The `main` heads are the Pins table above.
- The second round is under way (STATE.md:298). sn `v2/launch-tools` is
  merged. sn `v2/launch-examples`, server `v2/launch-latch` and the consent
  branches are still to merge. Step 1 freezes whatever `main` holds after
  it, and the runbook is then read against those heads.
- `v2/dual-stack-hops` is not in `main` (STATE.md:244).
- The localization store's `main` (`1dc9045`) holds the eight `sn_ledger_*`
  keys (VERSION2-FINAL.md §11). Every release build regenerates the app
  catalogs from the store (`localizations/README.md`:23-25), so each app is
  built against that store or a later one.
- The server is rebuilt against the sn revision that carries the new
  successor class (VERSION2-FINAL.md §13, Server row).
- Every launch binary is built from the frozen `main` (step 1).

### 2.2 CI green

- No repository has CI. The workflows were removed on 2026-10-05 and
  2026-10-06: "We build and test on our own hardware and do not use GitHub
  Actions" (sdk commit `7120b2cf`, with the matching removals in the app
  repositories and the localization store). So "CI green" means the owner's
  build pipeline and the hand-run suites below.
- The release gate of section 2.5 passes on the merged `main` (step 4).
- The apps' v2 code was not compiled where it was written (STATE.md:174,
  219, 228). Each app is built and its v2 suites pass:
  - apple: the build, `SnLedgerTests` (`app/networkTests/SnLedgerTests.swift`)
    and `AppShortcutsCatalogTests`;
  - windows: the build, and `go test ./tests`, which holds `TestSnPayoutLine`,
    `TestSnPayoutAgainstSdkHeader` and `TestSettlementV2Wiring`
    (`tests/sn_payout_test.go`);
  - linux: meson, msgfmt and the glibc floor, and meson's `unit` test, which
    builds `tests/SnLedgerPresentationTest.cpp` (`app/meson.build:612,849`);
  - android: the `androidTest` compile, and the unit tests
    `app/app/src/test/java/com/bringyour/network/ui/wallet/SnLedger{Tile,Claim,Wiring,Strings}Test.kt`;
  - mmm: `npm run test:sn-ledger-v2` in `ur.io/react` (`ur.io/react/package.json:79`).

### 2.3 The runtime against the rehearsal

- vault2 depends for good on the ABIs of the staking precompile's `getStake`,
  `getMinerCollateral`, `moveStake` and `transferStake`. A runtime that changes
  or drops one freezes every capture and claim, and vault2 cannot adapt
  (VERSION2-FINAL.md §17, third row).
- Gate 2 ran on a node built from runtime 475's source. "Any runtime change on
  mainnet repeats this gate" (§14 gate 2).
- Subtensor replaces its runtime every few days
  ([mainnet/VALIDATOR-PRODUCTION-RUNTIME.md](mainnet/VALIDATOR-PRODUCTION-RUNTIME.md):39).
  Step 2 checks it. Steps 8 and 15 repeat the check.

### 2.4 R-1 to R-7 on the then-current runtime

- The R battery runs the exact release bytecode on the rehearsal node
  (VERSION2-FINAL.md §15.8): R-1 the v2 contract phase with readback, R-2
  capture, R-3 claims, R-4 registration, R-5 approval and demands, R-6 gas and
  R-7 batched claims.
- It is rerun at launch on the runtime step 2 finds (step 3).

### 2.5 The release gates

- Gate 1: the Foundry suite at its coverage bars, `forge fmt --check`,
  `forge build --deny warnings --sizes` and `scripts/test-solidity-static.sh`,
  sizes recorded; and an independent review of vault2, `LedgerTree.sol` and
  `STSettlement2`, resolved (§14).
- Gate 3: the GT, GA, GL, SV, VA and CL suites, including GL-10 and SV-6
  inside their budgets.
- Gate 4: the E suite, E-1 to E-7.
- One command runs all of them (step 4).

### 2.6 Funding and gas

| Payer | Pays for | Size, from the rehearsal | Source |
| --- | --- | --- | --- |
| Deployer (O4) | the six v2 actions, and the escrow registration's value | limits 4,000,000, 2,500,000, 3,000,000, 1,000,000, 150,000 and 400,000; used 2,821,690, 1,463,833, 1,935,622, 310,486, 31,329 and 285,209; value `maximumBurnRao × 10^9` wei | BOOTSTRAP-CONTRACTS.md:660,697-712 |
| Governance Safe (O8) | `registerOperatorPool`'s value; the surplus over the burn comes back to the Safe | R-4 sent a 100 TAO ceiling, burned 3.80 TAO and used 1,594,088 gas | `scripts/runtime-rehearsal/README.md` R-4; `rehearsal/stack_test.go:31-32,190-194` |
| Operator root key | every v2 close, commit, finalize and answer | the §8.1 table; a capturing close is signed at no less than 3.2M; on 478 it needs 236,399 with vault2's staking-hotkey list short and about 1.37M with it full | VERSION2-FINAL.md §8.1; `docs/spec/runtime-478-audit.md`; `server/controller/st_controller.go:1079-1087` |
| Validator auditor key (O10) | approvals, challenges and demands | approve: 132,524 used, limit 135,736; demand: 98,268 used, limit 104,424, plus the fee | §8.1 table |
| Watcher key (O21) | demands and challenges | each demand pays `demandFeeWei`, 750,000 rao, plus gas; a sample starts at 256 demands and doubles | §7.1, §9.3, §10.2 |
| Claimants: the daemon relayer, app gas keys, anyone | claims | on 478 an account's first claim uses 187,332 gas, needs 192,318 with vault2's list short and about 1.31M with it full, and is signed at no less than 2,600,000; in a batch of 32, 148,378 gas a claim | `docs/spec/runtime-478-audit.md`; README, "Runtime 478" |
| App gas key | v2 claims from the apps | about 0.02 TAO on the device gas key | §11 |

- The deployer's `maximum_total_wei` covers every limit times its fee cap,
  plus the escrow funding (BOOTSTRAP-CONTRACTS.md:711-712). Each send's
  admission also needs the deployer's pending balance to cover the selected
  action and every later reservation of the same sender (:239-244).
- The operator gas policy caps every operator transaction. Today's
  `maximum_gas` of 1,000,000 and per-intent liability of 5·10^16 wei are below
  a capturing close signed at 3.2M, so the policy is re-signed before E_s
  (step 5; VERSION2-FINAL.md §8.1, §13).

### 2.7 Key custody

| Key | Role in this launch | Custody | Source |
| --- | --- | --- | --- |
| Deployer EVM key (O4) | sends the six v2 actions at consecutive nonces | an external EVM signer; the v1 deployer is on the owner's Ledger | BOOTSTRAP-CONTRACTS.md:652-655; LAUNCH.md:161-163 |
| Ed25519 approval key | signs the v2 phase approval | `root/subtensor/approval/`, operator-held | LAUNCH.md:185,369-376 |
| Governance Safe owner and Safe relayer (O8) | own `STSettlement2`; send `registerOperatorPool`; upgrade; pause | the owner's Ledger; the Safe is 1-of-1 | LAUNCH.md:47-48,164-166,187-188; VERSION2-FINAL.md §4.1 |
| Guardian | pauses v1 settlement, and with it v2 | the owner's Ledger | LAUNCH.md:161-163; VERSION2-FINAL.md §4.1 |
| Operator gas approver key | signs the operator gas policy | off the operator host; the server refuses any other approver for the scope | `server/st_operator_gas_policy.go:24-25`; `server/model/st_transaction_gas_model.go:386-388` |
| Treasury approval key | re-signs the treasury production approval | the Ed25519 key the validator config pins as `treasury_approval.signer` | `mainnet/treasury_approval.go:192-251`; `validator/treasury_approval_draft.go:64-66` |
| Operator root key | signs v2 closes, commits, finalizes and answers | `root_key` in `st.yml`, in the deployment vault | `server/controller/st_controller.go:251`; `server/controller/st_ledger_v2_client.go:356-362`; VERSION2-FINAL.md §7.2 |
| Validator auditor key (O10) | approvals, challenges and demands | a hex secp256k1 key file on the validator host, `--ledger_auditor_key_file` | `validator/run.go:142-143` |
| Watcher key (O21) | `snclaim v2 check --all --challenge --demand` | a host other than the validator's | VERSION2-FINAL.md §10.2 |
| Claim daemon relayer key (O22) | claims | the daemon config's `key_file` | `miner/claim_v2_daemon.go:61` |
| SN25 owner, `ur-owner` | holds SN25's stake-transfer switch, which can freeze every claim | 2-of-3 native multisig | LAUNCH.md:184,197-203,222-235; VERSION2-FINAL.md §4.1, §17 |

The deployer, guardian, Safe relayer and Safe owner keys are all on the
owner's single Ledger. The owner's availability is therefore a dependency of
steps 5, 8, 10 and 15 (VERSION2-FINAL.md §13, §17).

## 3. Owner inputs

| ID | Input | Constraint or default | Used in | Source |
| --- | --- | --- | --- | --- |
| O1 | The launch revisions: the `main` head of each repository that step 1 freezes | the Pins table or later, after the second merge round; without `v2/dual-stack-hops` | 1 | section 2.1 |
| O2 | The mainnet route: native and EVM RPC URLs, the HTTPS leaf SPKI pin if HTTPS, and the approved native chain name | the launch used Snow's Finney archive, `ws://172.28.208.185:9944` and `http://` for EVM JSON-RPC; the route is part of each signed plan | 2, 8, 13, 20 | LAUNCH.md:124-141; BOOTSTRAP-CONTRACTS.md:208-221 |
| O3 | The runtime identity the v2 phase config binds: version, code and metadata hashes and the reviewed source commit | what step 2 observes, after review | 8 | BOOTSTRAP-CONTRACTS.md:208-210 |
| O4 | The v2 deployer: its EVM address, next nonce and signer | neither the Safe nor an auditor; funded per section 2.6 | 8 | BOOTSTRAP-CONTRACTS.md:652-655,742-743 |
| O5 | The escrow hotkey | 32 bytes, nonzero, with no UID on SN25 at the send; not the pool hotkey | 8 | BOOTSTRAP-CONTRACTS.md:720-727, and "Back-to-back escrow registration (A7)" |
| O6 | The escrow registration cap `maximumBurnRao` | nonzero; the value is the cap × 10^9 wei | 8 | BOOTSTRAP-CONTRACTS.md:660,733-735 |
| O7 | The phase envelope: each action's gas limit and fees, `maximum_total_wei`, `maximum_attempts`, the native start checkpoint and send window, the source-lock, cutover and custody-fence evidence hashes, the custody identifier and the run directory | gas starts from BOOTSTRAP-CONTRACTS.md:705-708; `maximum_attempts` at most 8 across all actions, the back-to-back escrow send included | 8 | BOOTSTRAP-CONTRACTS.md:208-213,673-677; `mainnet/evm_phase.go:180` |
| O8 | The `STSettlement2` owner and its signers | the governance Safe `0x56F4Dad575576CC0B679FEf52899630f9F605418`, 1-of-1, owner `0x16C372dbBb24cd8473345ab13971E40814C8658F` at the v1 launch; confirm | 8, 15 | LAUNCH.md:47-48,164-166; VERSION2-FINAL.md §7.2 |
| O9 | The v1 coordinator for `initialize` | the proxy `0xC18925925E2B7bb9059b7d696b8c92762AE86406`; confirm | 8 | LAUNCH.md:42 |
| O10 | The auditor key and its EVM address | one address, quorum 1 at launch; the same key runs with `--ledger_auditor_key_file` | 8, 10 | BOOTSTRAP-CONTRACTS.md:740-744; VERSION2-FINAL.md §2.1 decision 2 |
| O11 | Whether to create the claim batcher | optional sixth action | 8, 13 | VERSION2-FINAL.md §8.5 |
| O12 | The demand fee | decided: 750,000 rao, 750,000,000,000,000 wei (owner decision, 2026-10-12); the command has no default | 8 | VERSION2-FINAL.md §7.1; BOOTSTRAP-CONTRACTS.md:724-729 |
| O13 | The re-signed operator gas policy: `revision`, `previous_policy_sha256`, `maximum_gas`, `maximum_intent_liability_wei`, `maximum_lifetime_liability_wei`, the validity window and the attempt caps | `maximum_gas` about 3.5M; intent liability at least `maximum_gas` × the 30 gwei fee cap; `maximum_intent_attempts` at most 3; same bindings | 5 | VERSION2-FINAL.md §8.1, §13; `server/local/sn25golive/gas.go:26-28` |
| O14 | The treasury re-sign inputs: the input file and the unsigned policy plan the live approval was drafted from, with their SHA-256, and the approval key's signature | unchanged; only `config_hash` may change | 10 | `mainnet/treasury_approval_test.go:466-512`; `validator/production_authority_history.go:368-388` |
| O15 | The adopted validator config: its absolute path, the predecessor's retained authority bundle, the progress file, the durable-volume declaration, and how the service command line gains `--ledger_auditor_key_file` | all paths absolute; the recommended default for the flag is a systemd drop-in for the validator unit (step 10) | 10 | `validator/treasury_approval_draft.go:71-77`; `validator/run.go:112-117` |
| O16 | The v2 pool hotkey (`poolHotkeyV2`) | new; nonzero; not the escrow hotkey; not used by another pool | 15 | `evm/src/STSettlementVault2.sol:166-171`; VERSION2-FINAL.md §12.1 |
| O17 | The pool registration cap `maximumBurnRao` and the value sent | the rehearsal sent the cap × 10^9 wei at the runtime's default MaxBurn of 100 TAO | 15 | `rehearsal/stack_test.go:31-32,190-194` |
| O18 | E_s and the registration date | decided: E_s is epoch 2, which starts at block 9,297,628 (owner decision, 2026-10-13: "we are launching v2 now to be active the next ur epoch"). Section 9 gives its schedule and the departures the owner must still accept | 14, 15 | section 9; VERSION2-FINAL.md §12.1 step 5, §14 |
| O19 | `v2_prune_alert_positions` | default 3 | 11 | `server/controller/st_ledger_v2_prune.go:33-35` |
| O20 | `--ledger_max_bytes` | default 2147483648 (2 GiB); a runtime flag, raised without a re-sign | 10 | `validator/run.go:144-145`; VERSION2-FINAL.md §6.6 |
| O21 | The watcher: host, key, RPC and fee budget, and how often it runs | a host other than the validator's | 13 | VERSION2-FINAL.md §10.2 |
| O22 | The claim daemon: coldkeys, relayer key, state directory, threshold and batcher | threshold default 1,900,000,000 rao (1.9 α), never below 15,200,000 rao | 13, 20 | `miner/run.go:195-197`; `miner/claim_v2_daemon.go:39-112` |
| O23 | The SDK release that pins `Vault2Address`, and the app release dates | lands before step 15 | 9 | VERSION2-FINAL.md §11; `sdk/sn_chain.go:83-89` |
| O24 | Who runs the first claim, and for which account | anyone may claim for anyone; payment goes to the leaf's coldkey | 20 | VERSION2-FINAL.md §1, §17 |
| O25 | Whether and how to hold the SN25 owner key `ur-owner` under the Safe's governance | recommended; no coldkey swap; no tooling | section 8 | VERSION2-FINAL.md §4.1, §17 |
| O26 | Gate 1's independent review: confirmation that it is resolved | the owner confirms | 4 | VERSION2-FINAL.md §14; STATE.md:91,99,103,176,189,198 |

## 4. Runbook

The order is VERSION2-FINAL.md §12.1's. Each sub-step sits where its inputs
first exist.

| Step | What | Who | Gate |
| --- | --- | --- | --- |
| 1 | Freeze the revisions and build | Owner, operator | |
| 2 | Check the mainnet runtime | Operator | |
| 3 | Rerun R-1 to R-7 | Operator | 2 |
| 4 | Run the release gates | Operator, owner | 1, 3, 4 |
| 5 | Re-sign the operator gas policy | Owner, operator | 6 |
| 6 | Ship the server v2 build, v2 off | Operator | 5 |
| 7 | Ship the validator, tools, SDK and apps, v2 off | Owner, operator | 5 |
| 8 | Deploy the v2 contracts with readback | Owner, operator | 5 |
| 9 | Pin `Vault2Address` and release the apps | Owner | 5 |
| 10 | Adopt the validator config, re-sign the treasury approval | Owner, operator | 6 |
| 11 | Switch the server config | Operator | 6 |
| 12 | Check what runs | Operator | |
| 13 | Start the watcher and set up the claim tools | Operator | |
| 14 | Schedule E_s | Owner | |
| 15 | Register the pool (activation) | Owner | |
| 16 | The switch at the start of E_s | Operator | |
| 17 | Close and commit epoch E_s | Server; operator watches | |
| 18 | Audit and approval | Validator, watcher | |
| 19 | Challenge window and finalization | Server | |
| 20 | First claims | Operator | |

### Phase A. Qualify; nothing in production changes

**Step 1. Freeze the launch revisions and build.**

- **Who.** Owner and operator.
- **Do.**
  1. Fix O1. Confirm section 2.1, including the localization store's `main`.
  2. Build `sn-mainnet` with Go 1.27.2. The pin is the `go` line of `go.mod`
     (`go.mod:3`, `go 1.27.2`; the server's `go.mod:3` is the same). Under
     `GOTOOLCHAIN=local` an older local toolchain is refused: "go: go.mod
     requires go >= 1.27.2 (running go …; GOTOOLCHAIN=local)". The command is
     [mainnet/MACOS.md](mainnet/MACOS.md):24-29's:

     ```bash
     GOTOOLCHAIN=local CGO_ENABLED=0 go build -mod=readonly -trimpath -buildvcs=true \
       -o /absolute/path/sn-mainnet ./mainnet
     ```

  3. Build the production composition, which holds `cli/validator`,
     `cli/snclaim`, `cli/miner` and the server services
     ([mainnet/RELEASE-BUILD.md](mainnet/RELEASE-BUILD.md):3-8,37-42):

     ```bash
     go run -mod=readonly ./scripts/mainnet-release-build --config /absolute/config.json
     ```

  4. Retain each binary's SHA-256 and `go version -m` output (LAUNCH.md:334-335).
- **Expect.** Every binary's build information names its O1 commit.
- **Stop if** a repository's `main` lacks its pin, `v2/dual-stack-hops` is in
  it, or the store's `main` lacks the `sn_ledger_*` keys.

**Step 2. Check the mainnet runtime against the rehearsal.**

- **Who.** Operator.
- **Do.** Capture the finalized runtime (`mainnet/runtime_snapshot.go:183-224`;
  genesis from LAUNCH.md:114-119) and read its version and hashes (fields at
  `mainnet/runtime_snapshot.go:33-41`):

  ```bash
  sn-mainnet runtime-snapshot --rpc "$MAINNET_RPC" \
    --expected-chain "$APPROVED_NATIVE_CHAIN" \
    --expected-genesis 0x2f0555cc76fc2840a25a6ea3b9637146806f1f44b090c175ffde2a7e5ab36c03 \
    --expected-evm-chain-id 964 > /secure/ur-mainnet/runtime-snapshot.json
  jq '.snapshot.runtime_version, .snapshot.runtime_code_hash, .snapshot.runtime_metadata_hash' \
    /secure/ur-mainnet/runtime-snapshot.json
  ```

- **Expect.** `specVersion` 478, code hash
  `0x7e722b3aa5c9192c98665a5d65fa91364bf9055e21fc90efce6d59bf4ef414e1` and
  metadata hash
  `0x4f96a9cb954ebe3ae0e364fe7e34ec8a0e902239832f68dfe90aaa1e4511c747`
  (`docs/spec/runtime-478-audit.md`, "Exact provenance"). The output is an
  unapproved observation, never an approval
  ([mainnet/MAINNET.md](mainnet/MAINNET.md):1653).
- **Observed already.** Mainnet ran 475, then 477 from block 9,267,632, and
  478 from block 9,276,925 (2026-10-13 21:08:36 UTC), as the public RPC
  `https://lite.chain.opentensor.ai` and archive
  `https://archive.chain.opentensor.ai` report. That is an observation, not
  the check of record. The owner's `runtime-snapshot` above still decides.
- **Stop if** any of the three differs. Gate 2 then repeats on the new runtime
  before step 8. For 478 it ran on sn branch `v2/runtime-478` (section 9.3,
  item 2).
  - **Pins.** `scripts/runtime-rehearsal/build.sh` builds only tag `v478`,
    commit `850f2755…`, against `docs/spec/runtime-v478-source.sha256`
    (build.sh:13-14,21,45). Another runtime first needs a reviewed change of
    those pins and a source review like `docs/spec/runtime-478-audit.md`.

**Step 3. Rerun the runtime rehearsal, R-1 to R-7.**

- **Who.** Operator.
- **Do.** From the repository root, with `forge` on `PATH`
  (`scripts/runtime-rehearsal/README.md`:12-15,104,220-223;
  BOOTSTRAP-CONTRACTS.md, "Runtime rehearsal (R-1)"):

  ```bash
  PROTOC=/path/to/protoc CARGO_TARGET_DIR=/path/to/target \
    scripts/runtime-rehearsal/build.sh /path/to/subtensor-checkout
  RUNTIME_REHEARSAL_NODE=/path/to/target/release/node-subtensor \
    go test -tags runtime_rehearsal ./rehearsal/ -v -timeout 60m
  RUNTIME_REHEARSAL_NODE=/path/to/target/release/node-subtensor \
    go test -tags runtime_rehearsal ./mainnet/ -run TestR1SettlementV2Phase -v
  ```

- **Expect.**
  - The build passes its source and genesis-code checks and writes
    `release/runtime-rehearsal-build.txt` in the target directory
    (README:17-44).
  - Every test passes. R-2 to R-6 took 8 minutes 22 seconds, R-7 about 6 and
    R-1 80 to 100 seconds (README:225-227; BOOTSTRAP-CONTRACTS.md, "Runtime rehearsal (R-1)").
  - R-1 uses the approved 750,000 rao fee and completes every action under
    the step 8 limits.
  - R-6's operator transactions fit the policy that step 5 re-signs (§15.8
    R-6).
- **Stop if** any test fails, or the logged gas exceeds the limits of
  section 2.6.
- `build.sh` builds the 478 node, and the battery that counts is the one on
  it (section 9.3, item 2). On 478 the least gas limit of a stake-moving
  transaction is far below the 475 figures of README and section 2.6, and
  grows with vault2's own staking-hotkey list; R-6's limits are logged as
  least limits beside `eth_estimateGas`'s (README, "Runtime 478").

**Step 4. Run the release gates.**

- **Who.** Operator runs; owner confirms O26.
- **Do.** In a workspace with the sibling checkouts the script names
  (`scripts/test-release-1.0-local.sh`:7-9), with `SN_TEST_STORAGE_ROOT` set
  or `/mnt/data` mounted (`scripts/test-storage.sh`:6-17):

  ```bash
  RUN_SERVER_DB_TESTS=1 ./scripts/test-release-1.0-local.sh
  ```

  This is the documented invocation (FINALIZE.md:5211-5214). Run the app
  suites of section 2.2 beside it.
- **Expect.** The last line is `[release-1.0] local release gate passed`
  (:653). The settlement v2 jobs:
  - `sn-v2`, gate 3's own verdict (:171-185);
  - `sn-scale` and `server-scale`, GL-10 and SV-6 (:187-205,519-538);
  - `sn-e2e`, gate 4 (:540-562);
  - `solidity`: format, release build with sizes, tests, the `v2_invariant`
    campaign, and the catalog and binding checks (:214-237);
  - `solidity-coverage`, gate 1's bar (:239-254), and `solidity-static`
    (:211);
  - `sdk`, CL-2 and CL-4 (:374-381), and the server's v2 suites with
    migrations 821 and 822 (:469-481).
  - The wiring tests `TestReleaseGateSettlementV2*` of
    `sim-testnet/release_gate_settlement_v2_test.go` run in the aggregate job.
- **Stop if** any job fails, or the script prints `[release-1.0] DB suites
  deferred` (:582): the database suites, the scale shard's database half and
  the E suite then did not run.

### Phase B. Release, approve and deploy

**Step 5. Re-sign the operator gas policy.**

The policy binds the v1 coordinator, the policy hash and the two accounts,
not call targets (`server/controller/st_transaction_gas_policy.go:97-104`). So
it does not depend on the deployment. It must be in force before the first v2
close at the end of E_s.

- **Who.** Owner signs with the approver key; operator installs.
- **Do.**
  1. Copy the launch request `server/local/sn25golive/sn25-mainnet-gas-params.json`
     to a new params file. Change only:
     - `revision`: the live revision plus 1;
     - `previous_policy_sha256`: the live policy's digest. The launch request's
       digest is `e2de5acf1ef30de0b5c00f0e9cb5291db1b56b5fe6d0a17a524bb65806b5d64d`
       (`server/local/sn25golive/main_test.go:26-28`), and LAUNCH.md:73-75
       names the installed one `e2de5acf…`. Confirm it against `st.yml`;
     - `maximum_gas`: about 3,500,000, which covers a capturing close signed
       at about 3.2M with room for a few deferred epochs (§13);
     - `maximum_intent_liability_wei`: at least `maximum_gas` ×
       `maximum_fee_per_gas_wei`, 1.05·10^17 wei at 3.5M and 30 gwei;
     - `maximum_lifetime_liability_wei`, the validity window and the attempt
       caps: checked against v2 traffic (O13; §13).

     Keep `chain_id`, `genesis_hash`, `no_id`, `coordinator`, `policy_hash` and
     `accounts` ("re-signed with the same bindings", §13).
  2. Review and sign with the launch approver key (usage at
     `server/local/sn25golive/main.go:32-38`):

     ```bash
     sn25golive gas-policy --params PARAMS.json --dry-run
     sn25golive gas-policy --params PARAMS.json --approver-key KEY.json \
       --out-policy POLICY.yml --out-authority AUTHORITY.yml
     ```

  3. Merge POLICY.yml's top-level `operator_gas_policy` into `vault/main/st.yml`
     (`server/local/sn25golive/gas.go:277`). Install AUTHORITY.yml as
     `config/main/operator-gas-authority.yml` (gas.go flag text;
     `server/controller/st_transaction_gas_policy.go:75`).
  4. Check offline, then restart api, alt and taskworker, which load st.yml
     once per process (`server/controller/st_controller.go:298-302`):

     ```bash
     sn25golive st-check --st st.yml --profile mainnet --authority AUTHORITY.yml --sn sn.yml
     ```

- **Expect.**
  - `policy_sha256`, an `approver_public_key` equal to the launch approver's,
    and `limits: gas 3500000, …` (gas.go:216-233).
  - The server admits the policy at its next signing
    (`server/model/st_transaction_gas_model.go:369-391`), and v1 sends
    continue.
- **Stop if** the server refuses with one of:
  - "operator gas policy rollback or disconnected revision";
  - "operator gas independent approval key cannot replace its original trust
    root";
  - "operator gas account owner or original history pin changed";
  - "operator gas policy differs from the selected deployment and vault role
    accounts".

  These are at `server/model/st_transaction_gas_model.go:386-391,420` and
  `server/controller/st_transaction_gas_policy.go:102-104`.
- **Gap.** `sn25golive` is the server's `local/sn25golive` command. No build or
  run instructions exist beyond its usage text.

**Step 6. Ship the server v2 build with v2 off.**

- **Who.** Operator.
- **Do.**
  1. Apply the migrations first. Migration 821 says "Apply before deploying a
     server with v2 addresses" (`server/db_migrations.go:10013-10025`):

     ```bash
     bringyourctl db migrate
     bringyourctl db version
     ```

  2. Deploy the step 1 build of every server service. The ones that read
     st.yml or sn.yml for v2 are api, alt (which mounts the same router),
     taskworker and `bringyourctl` (`server/api/run.go:274-293`;
     `server/alt/run.go:178`; `server/taskworker/workload_profile.go:93-94`).
  3. Leave st.yml and sn.yml without v2 keys. The binary goes before the
     config, because sn.yml is parsed with known fields only (§13;
     `server/provider_payout_transition.go:57-58`).
- **Expect.**
  - The database head is 822, and readiness passes. A binary stays not-ready
    while the head is below its own migration count:
    "database migration head %d is below binary-required head %d"
    (`server/router/warp_handlers.go:255-260`).
  - v1 settlement runs unchanged (section 1.2, state 1).
- **Stop if** readiness fails, or any v1 settlement step stops.
- No v2 code runs in the server's connect service or in the connect library.
  connect's v2 change is its API description, `api/bringyour.yml` at
  `029fb40d` (STATE.md:215). So connect has no place in this deploy order.

**Step 7. Ship the validator, the tools, the SDK and the apps, v2 off.**

This completes the "gate 5 software is released" part of §12.1 step 1.

- **Who.** Operator for the validator and the tools; owner for the releases.
- **Do.**
  1. Run the new validator binary on its unchanged config.
  2. Publish `snclaim` and the `provider` binary (`cli/snclaim`, `cli/miner`;
     RELEASE-BUILD.md:39).
  3. Release the SDK and the apps with the v2 display and claims, and without
     `Vault2Address`. Build the apps against SDK `b9b2bf78` or later, which
     carries the claim API in `sdk/sn_ledger_claim.go`.
  4. Recommended: let this release carry network payout consent signing
     (section 1.3). A consent takes effect from the epoch after its signing,
     so a network that signs at least one epoch before E_s is paid from E_s.
     Two things then come first:
     1. Deploy ur.io with the consent bridge page (mmm `v2/consent-signing`,
        `9e2e8c2b5`) and the bridge fix (mmm `main`, `64173fd13`). Every
        native app signs the consent through that page (STATE.md:268,297).
     2. Put the eleven `sn_consent_*` keys in the localization store (§11.1).
- **Expect.**
  - The validator's config and history hash as before (§13; VA-6,
    `validator/settlement_v2_test.go:43-47`).
  - The apps show v1 as before and no v2 tile: with no `Vault2Address` a tile
    shows only accounts the ledger returned, and there are none. Each app
    encodes this:
    - android `app/app/src/main/java/com/bringyour/network/ui/wallet/SnLedger.kt:83-93`;
    - windows `app/src/App/SnPayoutPresentation.cpp:73`;
    - linux `app/src/SnLedgerPresentation.hpp:112`;
    - apple `app/network/Main/Account/Earnings/SnLedgerPresentation.swift:55-64`;
    - mmm `ur.io/react/src/app/load/readViews.js:44-45`.
- **Stop if**:
  - the validator's config hash changes under the new binary;
  - an app build fails. Against a store without the keys, the Android build
    fails and ur.io's prebuild refuses with "store key missing" (mmm
    `ur.io/react/scripts/build-locales.mjs:82`);
  - an app release carries consent signing while ur.io is not deployed with
    both the consent bridge page and the bridge fix. Hold that release.

**Step 8. Deploy the v2 contracts with readback.**

- **Who.** Owner signs the approval and every transaction; operator runs the
  commands.
- **Inputs.** O2 to O12. Repeat step 2 first.

*8.1 Build and verify the catalog.* "Catalog" means `sim-testnet/contracts_gen.go`,
whose `SettlementV2ReleaseArtifacts` the release file is exported from
(BOOTSTRAP-CONTRACTS.md:677-695; `evm/README.md`:53-61). Run `stabi` under
umask 022: its generated files must not be group-writable, and its check
fails on mode 664 (STATE.md:9-10).

```bash
(cd evm && forge build --deny warnings --sizes)
go run ./sim-testnet/gencontracts --check evm/out sim-testnet/contracts_gen.go
stabi/generate.sh --check
go run ./sim-testnet/gencontracts --release-v2-json evm/out sim-testnet/contracts_gen.go \
  /secure/ur-mainnet/settlement-v2-contracts.json
```

- **Expect.** Both checks pass. The release file has schema
  `urnetwork-settlement-v2-contract-release-artifacts-v1` and holds exactly
  `SettlementVault2`, `Settlement2`, `ERC1967Proxy` and `ClaimBatcher`
  (BOOTSTRAP-CONTRACTS.md:677-681). The catalog's runtimes are 12,815 bytes
  for vault2, 8,588 for `STSettlement2` and 1,073 for the batcher
  (`sim-testnet/contracts_gen.go`).
- **Stop if** either check fails.

*8.2 Prepare custody.* Prepare one fresh `mainnet-evm-action` snapshot for each
approved journal, with daemon scope (BOOTSTRAP-CONTRACTS.md:765-768 and "Runtime rehearsal (R-1)";
[mainnet/OWNER-CUSTODY-PREPARATION.md](mainnet/OWNER-CUSTODY-PREPARATION.md):40-56,105-109).

- The journals are `vault2-create.json`, `escrow2-register.json`,
  `settlement2-create.json`, `proxy2-create.json`, `vault2-link.json` and, if
  O11, `batcher-create.json` (BOOTSTRAP-CONTRACTS.md:657-664).
- Each holds at most 524,288 bytes (`mainnet/evm_action_store.go:361`).
- Plan, review and apply the preparation (usage at
  `mainnet/storage_prepare_unix.go:119`):

  ```bash
  sn-mainnet storage-prepare plan --request "$V2_CUSTODY_REQUEST" \
    --request-sha256 "sha256:$V2_CUSTODY_REQUEST_DIGEST" > /secure/ur-mainnet/run-v2-preparation-plan.json
  sn-mainnet storage-prepare apply --plan /secure/ur-mainnet/run-v2-preparation-plan.json \
    --plan-sha256 "sha256:$V2_CUSTODY_PLAN_DIGEST" > /secure/ur-mainnet/run-v2-preparation-result.json
  ```

- Pass the result's `declaration.path` and `declaration.sha256` on every
  `apply` and `resume` (OWNER-CUSTODY-PREPARATION.md:100-102):
  `HOST=(--durable-volumes "$HOST_DECLARATION" --durable-volumes-sha256 "$HOST_DECLARATION_SHA256")`
  (LAUNCH.md:349-352).
- **Gap, being closed.** The daemon-scope request for these journals is not
  written out; the documented example is the owner-local one
  (OWNER-CUSTODY-PREPARATION.md:40-56). An example is being written, and its
  path will be cited here.

*8.3 Draft, preview, approve and plan.*

- Draft `/secure/ur-mainnet/settlement-v2.unsigned.json`, schema
  `urnetwork-mainnet-settlement-v2-contract-phase-config-v1`
  (BOOTSTRAP-CONTRACTS.md:666-669). Every input is in this file, never in a
  flag:
  - the deployer is each action's `sender`;
  - the escrow hotkey and the fee are action 0's constructor words;
  - the burn cap is action 1's calldata, with `value_wei` equal to the cap ×
    10^9;
  - the owner, v1 coordinator, auditors and quorum are action 3's initializer
    (`mainnet/evm_v2_contracts.go:81-86,219-295`; `mainnet/evm_phase.go:37-47,58,77-82,180`).
- **Gap, being closed.** No example config exists in the repositories. An
  example v2 phase config is being written, and its path will be cited here.
- Preview, extract the approval bytes and sign them (BOOTSTRAP-CONTRACTS.md:194-198;
  LAUNCH.md:369-376):

  ```bash
  umask 077
  sn-mainnet bootstrap-contracts preview --action vault2-create \
    --config /secure/ur-mainnet/settlement-v2.unsigned.json > /secure/ur-mainnet/settlement-v2.preview.json
  jq -r '.approval_signing_message_hex' /secure/ur-mainnet/settlement-v2.preview.json |
    xxd -r -p > /secure/ur-mainnet/settlement-v2.approval.bin
  APPROVAL=/absolute/path/root/subtensor/approval
  go run "$APPROVAL/sign.go" -key "$APPROVAL/sn25-mainnet-approval.key" \
    /secure/ur-mainnet/settlement-v2.approval.bin
  ```

- Review the preview before signing (BOOTSTRAP-CONTRACTS.md:752-759):
  - `vault2_constructor`: the escrow hotkey O5 and the fee
    750,000,000,000,000 wei;
  - the predicted addresses and runtime hashes of vault2, the implementation,
    the proxy and, if O11, the batcher;
  - the initializer: owner O8, v1 coordinator O9, the auditor O10 and quorum
    1;
  - the gas, fees and nonces of O7.
- Put the 128-character lowercase signature into `approval_signature_ed25519`
  (:139-143), save the config as `/secure/ur-mainnet/settlement-v2.json`, and
  run `plan`. It verifies the approval and prints `plan_hash`, which becomes
  `SETTLEMENT_V2_HASH`:

  ```bash
  sn-mainnet bootstrap-contracts plan --action vault2-create --config /secure/ur-mainnet/settlement-v2.json
  ```

- **Stop if** any preview value differs from the owner inputs, or `plan`
  refuses.

*8.4 vault2 and its escrow registration, back to back (A7).*

- Vault2's creation reveals the escrow hotkey. Anyone who registers it first
  makes `registerEscrow` fail for good, so the registration is the deployer's
  very next nonce (BOOTSTRAP-CONTRACTS.md, "Back-to-back escrow
  registration (A7)").
- The commands are the block after that paragraph. Append `"${HOST[@]}"` to
  each `apply` and `resume`. The statuses are the ones R-1 asserts
  (`mainnet/evm_v2_rehearsal_test.go:545-556`).
  1. Open vault2's journal. Expect `signature-awaiting-import`. The result
     prints vault2's `signing_digest` and `unsigned_transaction` and, beside
     them, the escrow registration's `escrow_signing_digest` and
     `escrow_unsigned_transaction`, the approved action 1
     (BOOTSTRAP-CONTRACTS.md, "Back-to-back escrow registration (A7)";
     `mainnet/evm_action.go:90-91`):

     ```bash
     sn-mainnet bootstrap-contracts apply --action vault2-create --config /secure/ur-mainnet/settlement-v2.json --run-dir /secure/ur-mainnet/run-v2 --accept-plan-hash "$SETTLEMENT_V2_HASH"
     ```

  2. The deployer signs both before any send: `unsigned_transaction` and
     `escrow_unsigned_transaction`. Each signature is canonical binary
     EIP-1559 bytes in its own file, pinned by its SHA-256
     (LAUNCH.md:422-423,443). `escrow2-register`'s later `apply` prints the
     identical transaction and digest as its own.
  3. Import both, send both back to back, and complete vault2. Expect
     `signed-custody-complete`, then
     `submitted-with-escrow-registration-awaiting-canonical-receipts`, then
     `vault2-created`. The last comes after the readback of vault2's exact
     runtime and getters: `CHALLENGE_BLOCKS` 14,400, `coordinator` zero, the
     counters zero and `conservationHolds` true (BOOTSTRAP-CONTRACTS.md:774-784).

     ```bash
     sn-mainnet bootstrap-contracts resume --action vault2-create --config /secure/ur-mainnet/settlement-v2.json --run-dir /secure/ur-mainnet/run-v2 --accept-plan-hash "$SETTLEMENT_V2_HASH" --signed-transaction /secure/ur-mainnet/vault2.signed.bin --signed-transaction-hash "$VAULT2_SIGNED_FILE_HASH" --escrow-signed-transaction /secure/ur-mainnet/escrow2.signed.bin --escrow-signed-transaction-hash "$ESCROW2_SIGNED_FILE_HASH"
     sn-mainnet bootstrap-contracts resume --action vault2-create --config /secure/ur-mainnet/settlement-v2.json --run-dir /secure/ur-mainnet/run-v2 --accept-plan-hash "$SETTLEMENT_V2_HASH" --online --submit
     sn-mainnet bootstrap-contracts resume --action vault2-create --config /secure/ur-mainnet/settlement-v2.json --run-dir /secure/ur-mainnet/run-v2 --accept-plan-hash "$SETTLEMENT_V2_HASH" --online
     ```

  4. Open the escrow journal, import the same escrow bytes and complete it.
     Expect `signature-awaiting-import`, `signed-custody-complete`, then
     `escrow2-registered`. The last comes after the readback of the
     `EscrowRegistered` event, the UID mapping and `escrowRegistered` true,
     and `escrow_collateral` must read `(false, 0, 0, 0, 0)` (:785-796).

     ```bash
     sn-mainnet bootstrap-contracts apply --action escrow2-register --config /secure/ur-mainnet/settlement-v2.json --run-dir /secure/ur-mainnet/run-v2 --accept-plan-hash "$SETTLEMENT_V2_HASH"
     sn-mainnet bootstrap-contracts resume --action escrow2-register --config /secure/ur-mainnet/settlement-v2.json --run-dir /secure/ur-mainnet/run-v2 --accept-plan-hash "$SETTLEMENT_V2_HASH" --signed-transaction /secure/ur-mainnet/escrow2.signed.bin --signed-transaction-hash "$ESCROW2_SIGNED_FILE_HASH"
     sn-mainnet bootstrap-contracts resume --action escrow2-register --config /secure/ur-mainnet/settlement-v2.json --run-dir /secure/ur-mainnet/run-v2 --accept-plan-hash "$SETTLEMENT_V2_HASH" --online
     ```
- **Stop if**:
  - `escrow-registration-reverted-nonce-consumed`. Someone registered the
    escrow hotkey first. No funds are at risk; recover by redeploying under a
    new approval with a fresh escrow hotkey and fresh nonces ("Two residuals
    remain", after the A7 paragraph; section 6).
  - `escrow-registration-allowance-exhausted` (the A7 paragraph).
  - Any readback refusal, or a nonzero escrow collateral.

*8.5 The implementation, the proxy, the binding and the batcher.*

- For `settlement2-create`, `proxy2-create`, `vault2-link` and, if O11,
  `batcher-create`, in that order, each after the previous one's completed
  status (BOOTSTRAP-CONTRACTS.md, after the command block):
  1. `apply`;
  2. sign its `unsigned_transaction`;
  3. `resume --signed-transaction … --signed-transaction-hash …`;
  4. `resume --online --submit`;
  5. `resume --online`.
- **Expect.**
  - The statuses `signature-awaiting-import`, `signed-custody-complete` and
    `submitted-awaiting-canonical-receipt`, then `settlement2-created`,
    `proxy2-created-initialized`, `vault2-coordinator-bound` and
    `batcher-created` (`mainnet/evm_v2_phase.go:37-49`), each after the
    readback of BOOTSTRAP-CONTRACTS.md:797-824.
  - At `vault2-link`: `installation_complete` true and `activation_ready`
    false (:826-829). The result names `vault2_address`, `settlement2_address`
    (the proxy), `settlement2_implementation_address` and, after the sixth
    action, `claim_batcher_address` (`mainnet/evm_action.go:81-84`).
- **Stop if** any readback mismatches. Never repeat a completed CREATE, and
  never change the accepted graph or nonces to escape an ambiguous outcome
  (LAUNCH.md:443-447).

*8.6 Keep the record.* The six journals' receipts are the v2 deployment record
(BOOTSTRAP-CONTRACTS.md:829-833). Retain the run directory, the config, the
release file and every signed file. The launch readiness digest is unchanged
(§8.5).

**Step 9. Pin `Vault2Address` and release the apps.**

Activation needs v2 claims in the apps (§2.1 decision 8; §11 "At activation").
No app reads `Vault2Address` from a server or a config: "/sn/epoch never sets
it: the vault a claim is sent to is pinned by the release"
(`sdk/sn_chain.go:83-88`). Pinning therefore takes an SDK change and a second
release of every app.

- **Who.** Owner, with the app releases.
- **Do.**
  1. In `DefaultSnChainSettings()` (`sdk/sn_chain.go:89`), set `Vault2Address`
     to step 8's `vault2_address`, lowercase `0x` (the field: :42).
  2. Change the two tests that require an empty default:
     `TestSnChainSettingsVault2Address` (`sdk/sn_ledger_claim_test.go:1665-1667`)
     and `TestSnChainSettingsWasmVault2Address` (`sdk/js/sn_wasm_test.go:107-109`).
  3. Run the SDK's v2 suites, the `sdk` job selector of the release gate
     (`scripts/test-release-1.0-local.sh`:374-381).
  4. Rebuild and release every app against that SDK: the Android AAR, the
     Apple framework, the Windows DLL, the Linux library and the ur.io wasm.
- **Expect.** Each app shows the v2 tile and offers Claim when an account has
  anything claimable. Before the first finalization nothing is claimable.
- **Stop if** the pinned address differs from the server's `vault_v2_address`
  of step 11. The SDK claims an account only in the pinned vault2
  (`sdk/sn_ledger_claim.go:161-162`), and the server names the vault from its
  config (`server/controller/sn_ledger_v2_handler.go:193`).
- The releases land before step 15 (O23). The SDK can also read
  `Vault2Address` from the network space's `sn_chain` values
  (`sdk/network_space.go:92`), but no app writes them.

**Step 10. Adopt the validator config and re-sign the treasury approval.**

- **Who.** Owner signs; operator runs.
- **Gap.** No adoption command lines are documented. The procedure below is
  the one the VA-6 test runs: "settlement v2 adoption needs no new tooling"
  (`mainnet/treasury_approval_test.go:466-512`).
- **Do.**
  1. Write the adopted config: the running config plus exactly these changes
     (`validator/production_authority_history.go:350-388`):
     - `settlement_v2` with `settlement` set to step 8's `settlement2_address`
       (the proxy) and `vault` to its `vault2_address`, both lowercase `0x`
       (`validator/settlement_v2.go:14-30`);
     - one appended `production_authority_history` entry: the predecessor's
       authority bundle, which `validator run` retains as
       `<state_dir>/owner-recycle-production-authority-<sha256>.json`
       (`validator/production_authority_history.go:80-82`;
       [mainnet/VALIDATOR-UPLOAD-RUNTIME.md](mainnet/VALIDATOR-UPLOAD-RUNTIME.md):19-26);
     - `treasury_approval` naming the new envelope's absolute path, with the
       same `signer`.

     Every path is absolute (`validator/treasury_approval_draft.go:71-77`).
  2. Draft the approval again, from the same input and policy plan as the
     live one (O14; `mainnet/treasury_approval.go:192-251`):

     ```bash
     sn-mainnet treasury approval-plan --input "$TREASURY_INPUT" \
       --config "$ADOPTED_CONFIG" --config-sha256 "$ADOPTED_CONFIG_SHA256" \
       --policy-plan "$TREASURY_POLICY_PLAN" --policy-plan-sha256 "$TREASURY_POLICY_PLAN_SHA256" \
       --approval-key "$TREASURY_APPROVAL_KEY" --out "$ADOPTED_APPROVAL"
     ```

  3. Sign the printed `signing_digest`, the 32 digest bytes, with the approval
     key (Ed25519). The printed derivation gives the check command
     (`mainnet/treasury_approval.go:24-31`).
  4. Assemble the envelope, then put the printed `treasury_approval` into the
     adopted config (:252-277):

     ```bash
     sn-mainnet treasury approval-envelope --approval "$ADOPTED_APPROVAL" \
       --signature "$SIGNATURE" --approval-key "$TREASURY_APPROVAL_KEY" --out "$ADOPTED_ENVELOPE"
     ```

  5. Restart the validator on the adopted config, with the auditor key
     (`validator/run.go:112-117,142-145`):

     ```bash
     validator run --config="$ADOPTED_CONFIG" --progress-file="$PROGRESS_FILE" \
       --ledger_auditor_key_file="$AUDITOR_KEY_FILE" \
       --durable-volumes="$VALIDATOR_DECLARATION" --durable-volumes-sha256="$VALIDATOR_DECLARATION_SHA256"
     ```

- **Expect.**
  - The approval equals the old one except `config_hash` (the VA-6 test).
  - The loader admits the adopted config.
  - At its first finalized snapshot the validator confirms
    `STSettlement2.vault()`, `STSettlement2.v1Coordinator()` and
    `vault2.coordinator()` (`validator/settlement_v2.go:44-71`;
    `validator/release_runtime_v2.go:379-387`).
- **Stop if** any of these appears:
  - "settlement v2 adoption must append exactly one authority history entry",
    "settlement v2 adoption changes signed configuration beyond the added
    settlement_v2 block" or "settlement v2 adoption changes the approval
    beyond its config hash"
    (`validator/production_authority_history.go:374,382,385`);
  - "settlement_v2 … at block … returned …" (`validator/settlement_v2.go:67`).
- **Gap, being closed.** The repository's validator unit runs
  `run --config=… --progress-file=…` and adds no auditor flag
  (`mainnet/repair_validator_authority.go:181-183`), and its installer never
  overwrites a unit ([mainnet/VALIDATOR-ACTIVATION.md](mainnet/VALIDATOR-ACTIVATION.md):198-204).
  Without the flag "none is sent" (`validator/run.go:142-143`), and no root is
  ever approved. The recommended default (O15) is a systemd drop-in for the
  validator unit that adds `--ledger_auditor_key_file`. It is being written,
  with a test, and its path will be cited here.
- Adoption is one-way: "once adopted it can be neither re-pointed nor removed"
  (`validator/production_authority_history.go:256-259`).
- Step 11 follows directly. The server's reserved upload lane pins the
  validator config file by its path, bytes and SHA-256
  (VALIDATOR-UPLOAD-RUNTIME.md:3-9).

**Step 11. Switch the server config.**

The event mirror reads v2 logs only while v2 is enabled, from one shared
checkpoint, with no backfill (`server/controller/st_controller.go:2598-2604,4255-4258`).
The switch therefore comes before step 15.

- **Who.** Operator.
- **Do.**
  1. In one change, add the same pair to both files:
     - `vault/main/st.yml`, mainnet profile: `settlement_v2_address` and
       `vault_v2_address`, and optionally `v2_prune_alert_positions` (O19)
       (`server/controller/st_controller.go:243-244,262`);
     - `config/main/sn.yml`, under `mainnet:`: `settlement_v2_address` and
       `vault_v2_address` (`server/provider_payout_transition.go:45-47`).

     Both are lowercase `0x`, nonzero and distinct, and differ from the v1
     contracts (`server/st_ledger_v2_config.go:14-27`;
     `server/controller/st_ledger_v2_driver.go:94-98`).
  2. Point `reserved_attempt_upload.admission.production_runtime_config` at
     the adopted validator config, with the new approval and authority files
     beside it (§13; VALIDATOR-UPLOAD-RUNTIME.md:3-33;
     `server/controller/st_reserved_attempt_upload.go:31-35`).
  3. Check offline:

     ```bash
     sn25golive st-check --st st.yml --profile mainnet --authority AUTHORITY.yml --sn sn.yml
     ```

     It runs the mainnet admission with the v2 fields
     (`server/local/sn25golive/stcheck.go:274-296`), but its printout omits
     the v2 addresses (:298-322). Compare those against step 8 by eye.
  4. Restart api, alt and taskworker. st.yml is loaded once per process, and
     the builder is installed at start (`server/controller/st_controller.go:298-302`;
     `server/api/run.go:283-284`).
- **Expect.**
  - No `[st]st.yml unavailable (st subsystem disabled)` in the logs
    (`server/controller/st_controller.go:305,311`).
  - The gauge `urnetwork_api_reserved_upload_startup_failed` reads 0
    (`server/api/run.go:255-272`).
  - `GET /sn/ledger/account` answers, with no accounts yet.
  - v1 closes and commits continue; the pool is not registered.
- **Stop if**, and restore both files together:
  - "sn: selected deployment differs from reviewed earnings schedule"
    (`server/provider_payout_transition.go:161-163`). Every mainnet send, v1
    included, is refused while the two files disagree.
  - "[st]v1 cutoff for epoch … unknown" (`server/taskworker/work/st_ledger_v2_work.go:29-37`).
    A failed `vault2.pools` read leaves that turn's v1 close and commit
    unscheduled.
  - The reserved-upload gauge reads 1.

**Step 12. Check what runs.**

This is §12.1 step 4. Gate 6 completes here: the validator config is adopted,
the treasury approval and the gas policy are re-signed, and the server is
configured (§14 gate 6). Gate 5 completed at step 9.

- **Who.** Operator.
- **Do.**
  - Confirm the validator runs the adopted config, and that its v2 identity
    check passed.
  - Confirm the running validator unit carries `--ledger_auditor_key_file`,
    through the step 10 drop-in. Without it the audit sends no approval
    (`validator/run.go:142-143`). The check will be cited with the drop-in.
  - Confirm every server service runs the step 6 build with the step 5 policy
    and the step 11 config.
  - Confirm the step 9 app releases are out.
- **Stop if** any of these differs.

**Step 13. Start the watcher and set up the claim tools.**

- **Who.** Operator.
- **Do.**
  1. The watcher runs on a host other than the validator's (§10.2), against
     each pending root, from its commit until its window closes. Its usage is
     at `miner/onchain/run.go:32-42`:

     ```bash
     snclaim v2 check --all --vault=<vault2_address> --no_id=1 --rpc=<url> \
       --api_url=<operator api> --challenge --demand --key_file=<watcher key>
     ```

     `--challenge` and `--demand` need `--key_file`
     (`miner/onchain/v2_check.go:204-213`). It stops sending demands when the
     challenge window closes (:677). `--api_url` defaults to
     `https://api.bringyour.com` (`miner/onchain/run.go:64-67`).
  2. A claim daemon config has these keys (`miner/claim_v2_daemon.go:51-112`):
     - `schema_version: 2`, `api_url`, `rpc`, `chain_id: 964`;
     - `vault`: vault2, lowercase;
     - `batcher`: optional; the step 8 batcher, lowercase and not the vault;
     - `coldkeys` in SS58, `key_file`, `state_dir`, and `poll_seconds` from 5
       to 3600, default 30.

     Run it with the durable-volume flags, which it requires
     (`miner/run.go:136-138`; `miner/claim_v2_daemon.go:643`):

     ```bash
     provider claim-daemon-v2 --config=<path> [--v1-config=<path>] [--threshold-rao=<rao>] [--claim-now] \
       --durable-volumes=<path> --durable-volumes-sha256=<hash>
     ```
- **Expect.**
  - The daemon starts only if the batcher's code is exactly the release
    runtime (`miner/claim_v2_daemon.go:445-454`).
  - It claims an account when its claimable reaches the threshold, default
    1.9 α (`miner/run.go:195-197`).
- **Stop if** the daemon refuses its batcher, or refuses a threshold below the
  floor (`miner/claim_v2_daemon.go:640-642`).
- **Gap, being closed.** Nothing in the repositories schedules the watcher or
  installs it as a service, and no example daemon config exists (O21, O22). An
  example claim daemon config and a watcher service and timer are being
  written, and their paths will be cited here.

### Phase C. Activate

**Step 14. Schedule E_s.**

- **Who.** Owner (O18).
- **Do.**
  1. E_s is decided: epoch 2 (O18). Section 9 holds its schedule, and the
     departures from the plan of record that the owner must accept first.
     The plan's own rule takes E_s as the first epoch that starts at least
     two days after the last gate passes, and calls epoch 3 (block 9,348,028)
     not reachable (§14). That rule applies again if E_s moves.
  2. Compute its start. With the weekly policy effective from epoch 1 at
     block 9,247,228 (LAUNCH.md:29-32), `epochStartBlock(e)` is
     9,247,228 + 50,400 × (e − 1) (`evm/src/STCoordinator.sol:302-310`). This
     holds while the v1 policy schedule is unchanged. Epoch 2 starts at block
     9,297,628.
  3. Pick the registration date: one to two days before the start of E_s,
     inside the 21,600-block registration immunity (§12.1 step 5). At 12
     seconds a block that is 7,200 to 14,400 blocks before the start: blocks
     9,283,228 to 9,290,428 for epoch 2.
  4. Check the v1 policy for E_s: `epochBlocks >= rootCommitWindowBlocks +
     14,400` (`evm/src/STSettlement2.sol:86-89`). Mainnet's 50,400 and 1,200
     satisfy it (§5).
- **Note.** The treasury production approval expires at native block
  9,739,882, about 2026-12-17 (§13). That falls inside epoch 10, blocks
  9,700,828 to 9,751,227. Its renewal is a separate successor, after adoption.
  So plan steps 10 to 15 to leave time for that renewal.

**Step 15. Register the pool. This is the activation.**

The Safe sends `registerOperatorPool(1, poolHotkeyV2, maximumBurnRao, E_s)`
with value `maximumBurnRao × 10^9` wei, as a direct call, the way v1's
`registerOperator` went (§12.1 step 5; LAUNCH.md:47-56).
`sn-mainnet register-operator-pool` builds and preflights that call and reads
its result back; "it never signs or sends" (BOOTSTRAP-CONTRACTS.md,
"Settlement v2 activation"). Every flag is required.

- **Who.** Owner, through the Safe; operator runs the tool.
- **Do.**
  1. Repeat step 2.
  2. Review the call. `--pool-hotkey` is `0x` and the hotkey's 32 bytes in
     hex, and `--value-wei` is `maximumBurnRao × 10^9` (O16, O17):

     ```bash
     sn-mainnet register-operator-pool review --rpc "$EVM_RPC" --settlement2 "$SETTLEMENT2_PROXY" \
       --safe "$GOVERNANCE_SAFE" --no-id 1 --pool-hotkey "$POOL_HOTKEY_V2" \
       --maximum-burn-rao "$MAXIMUM_BURN_RAO" --value-wei "$VALUE_WEI" --first-epoch "$E_S"
     ```

  3. The Safe sends exactly the printed `to`, `value_wei` and `data`.
  4. Read the registration back, with the Safe transaction's hash:

     ```bash
     sn-mainnet register-operator-pool readback --rpc "$EVM_RPC" --settlement2 "$SETTLEMENT2_PROXY" \
       --no-id 1 --pool-hotkey "$POOL_HOTKEY_V2" --first-epoch "$E_S" --transaction-hash "$SAFE_TRANSACTION_HASH"
     ```

- **Expect.**
  - `review` exits 0. It prints the call, `to`, `value_wei`, `data`,
    `function` and `args`, with `first_epoch_start_block`, `blocks_remaining`
    and `preflight_uid`, the UID the preflight registered at the head.
  - `readback` exits 0. It has confirmed vault2's `PoolRegistered` event,
    `pools(noId)` and `settlementAt(noId, E_s)` with version 2, and it prints
    the event's `uid`, `transaction_hash`, `block_number` and `head_block`.
  - The surplus over the burn returns to the Safe
    (`evm/src/STSettlement2.sol:90-96`).
  - Within a minute the server's driver sees the pool, and the v1 driver
    keeps only epochs below E_s
    (`server/controller/st_ledger_v2_driver.go:197-214,255-280`).
- **Stop if**:
  - `review` exits 3: the call must not be sent. It names the refusal:
    - before any read, a zero pool hotkey, or a value other than
      `maximumBurnRao × 10^9` wei;
    - a route that is not EVM chain 964, vault2's own escrow hotkey, or a
      first epoch whose start block the head has reached;
    - the preflight's revert: `InactiveOperator`, `InvalidEpoch`,
      `CadenceTooShort`, `NativeRefundFailed`, vault2's
      `InvalidConfiguration`, `OwnableUnauthorizedAccount` (the Safe does not
      own `STSettlement2`) or the v1 coordinator's `UnknownOperator`.
  - `readback` exits 3: the chain does not hold the reviewed registration. It
    names each difference: no receipt; a reverted transaction; no
    `PoolRegistered` event of `noId` from vault2, or one with another hotkey
    or first epoch; `pools(noId)` unregistered or different; `settlementAt`
    with another vault, hotkey or version.
  - Either mode exits 1: a read failed, "which decides nothing". Run it again.
    Exit 2 is a usage error.
  - A reverted registration: "If it fails, nothing has changed, and E_s
    moves" (§12.1 step 5).
- R-4 registers its pool through both modes on the rehearsal node, and PR-1
  to PR-5 run them on the in-repo chain (`rehearsal/r4_registration_test.go`
  and `mainnet/evm_v2_pool_test.go`).

**Step 16. The switch at the start of E_s.**

- **Who.** Operator watches.
- **Expect.**
  - The validator weights the v2 pool UID, and no longer consults the v1 pool
    hotkey (§9.1).
  - The v1 driver closes and commits nothing at or after E_s.
  - Epoch E_s − 1 completes on v1 in its normal windows. If a v1 step fails,
    v1 rules apply, and the phase C sweep recovers the value (§12.1 step 7;
    §12.2).
  - v1 claims stay open until each v1 expiry, and v1 credit stays withdrawable
    with `snclaim withdraw --vault=v1 --coldkey=<key> --contract=<v1 vault>
    --rpc=<url> --key_file=<path>` (§12.1 step 8; `miner/onchain/run.go:32-42`;
    `miner/onchain/withdraw.go:28-30`).
- **Stop if** the validator refuses "settlementAt differs from vault2's
  immutable pool" (`validator/settlement_v2_resolver.go:75-103`).

### Phase D. The first cycle

The windows, in blocks after end(E_s), from VERSION2-FINAL.md §5:

| Blocks after end(E_s) | What | Who |
| --- | --- | --- |
| 0 to 1,200 | `closeOperatorEpoch(E_s)` captures the pool hotkey's movable stake | anyone; the server |
| 0 to 1,200, after the close | `commitDistribution(E_s)` | root signer |
| from the commit to commit + 14,400 | `demandPath`, `checkPath` | anyone; the server answers |
| from the commit to end(E_s + 1) | `approveDistribution` | the validator |
| at or after commit + 14,400 | `finalizeDistribution`, once approved, with no open demand | anyone; the server |

**Step 17. Close and commit epoch E_s.**

- **Who.** The server; the operator watches.
- **Expect.**
  - `v2close:E_s:1`, then `EmissionCaptured`.
  - The ledger built and published, chunks before the header.
  - `v2commit:E_s:1`, then `DistributionCommitted`
    (`server/controller/st_ledger_v2_client.go:33-48`;
    `server/controller/st_ledger_v2_driver.go:8-14`).
  - Every send passes its `eth_call` preflight and is signed at the estimate
    plus 20%, within the policy (`server/controller/st_ledger_v2_client.go:356-374`).
- **Act if** `v2_missed_close` or `v2_missed_commit_window` fires (section 5).
  Anyone may close inside the window (§7.2). A missed close defers the epoch with
  zero funding, and the next close captures its emission (§6.4).

**Step 18. Audit and approval.**

- **Who.** The validator; the watcher; the operator reads both.
- **Expect.**
  - The validator audits and sends `approveDistribution` before end(E_s + 1)
    (§9.3).
  - Its diagnostic is `ledger_audit_approve` (`validator/release_diagnostics.go:223`).
    It records the finding and decision under `<state_dir>/settlement-v2-ledger`
    (`validator/settlement_v2_custody.go:3-8`).
  - The watcher reports no fault for every account of the protected root.
- **Act if**:
  - the validator logs `ledger_audit_refuse`, `ledger_audit_challenge` or
    `ledger_audit_demand`;
  - the watcher reports a provable fault or "unverifiable";
  - a demand is open: the server answers it (`v2answer`). Watch for
    `v2_open_demand_over_one_day`.

**Step 19. The challenge window and finalization.**

- **Who.** The server.
- **Expect.**
  - At or after commit + 14,400, with the one approval and no open demand,
    `v2finalize` and `DistributionFinalized`; vault2 then holds one root.
  - The earnings email and `/account/epochs` show the epoch's v2 amounts
    (§8.4).
- **Act if** `v2_root_not_finalized` fires: the root is not finalized by
  end(E_s) + 14,400 + 1,200. A root still unfinalized at end(E_s + 1) is
  replaced by the next commit, and its epochs' funding stays (§4.3 S3).

**Step 20. First claims.**

- **Who.** Operator (O24).
- **Do.**
  1. Read an account (`miner/onchain/run.go:32-42`):

     ```bash
     snclaim v2 status --coldkey=<ss58> --vault=<vault2_address> --rpc=<url> --api_url=<operator api>
     ```

  2. Preflight, then claim it:

     ```bash
     snclaim v2 claim --coldkey=<ss58> --vault=<vault2_address> --rpc=<url> --key_file=<key> --dry-run
     snclaim v2 claim --coldkey=<ss58> --vault=<vault2_address> --rpc=<url> --key_file=<key>
     ```

     With the batcher, `snclaim v2 claim --all --coldkey=<ss58> --batch=<batcher> …`
     claims two or more accounts in one transaction, at most 32.
  3. Then let the daemons and the apps claim.
- **Expect.** `Claimed(noId, rootIndex, index, coldkey, cumulative, amount, caller)`
  pays the leaf's coldkey (§7.1), and `status` then shows the amount as
  claimed. "After the first v2 finalization, claim at least one account
  before relying on it" (§17, first row).
- **Stop if** a claim reverts with anything but a documented refusal:
  `AmountTooLow` below the floor, `TooManyStakingHotkeys`, or `NothingToClaim`
  after a third party's claim (§5, §7.3). Then vault2 may not pay, and a
  vault2 defect is permanent for what it holds (section 6; §17).

## 5. Monitoring during the first epochs

The server's v2 alerts are log lines only, `[st]v2 ALERT <name> <subject>: …`.
Each fires once per process and subject, so a restart can fire it again
(`server/controller/st_ledger_v2_driver.go:176-187`). No metric or dashboard
exists for them.

| Watch | Where | Threshold | Response | Source |
| --- | --- | --- | --- | --- |
| `v2_missed_close` | taskworker log | no close by end(e) + 1,200 | the epoch defers with zero funding; the next close captures it | `server/controller/st_ledger_v2_driver.go:349-352,523-526` |
| `v2_missed_commit_window` | taskworker log | no root committed by end(e) + 1,200 | the next root covers the epoch from its own usage | :353-359,655-658; VERSION2-FINAL.md §6.4 |
| `v2_open_demand_over_one_day` | taskworker log | a demand open for one day, 7,200 blocks at 12 seconds | check the server's answer of that demand | :366-371 |
| `v2_root_not_finalized` | taskworker log | not finalized by end(e) + 14,400 + 1,200 | check approval, demands and pause | :378-388 |
| `v2_sn25_prune_risk` | taskworker log | SN25 among the lowest 3 non-immune subnets by moving price, or no longer added; read every ten minutes | encourage claims (§17); dissolution strands custody | `server/controller/st_ledger_v2_prune.go:33-38,138-165` |
| `[st]v2 prune check failed` | taskworker log | any | the census read failed | `server/controller/st_ledger_v2_driver.go:317-321` |
| `[st]v1 cutoff for epoch … unknown` | taskworker log | any | a `vault2.pools` read failed; that turn's v1 steps wait | `server/taskworker/work/st_ledger_v2_work.go:29-37` |
| "sn: selected deployment differs from reviewed earnings schedule" | server log | any | st.yml and sn.yml disagree; every mainnet send is refused | `server/provider_payout_transition.go:161-163` |
| "unsigned gas/fee/tip/intent envelope exceeds approval" | server log | any | the gas policy is too low for a send | `server/model/st_transaction_gas_model.go:109-110` |
| `urnetwork_api_reserved_upload_startup_failed` | API metric | 1 | the reserved upload lane is off | `server/api/run.go:33-38,255-272` |
| `ledger_audit_approve`, `_challenge`, `_demand`, `_refuse`, `_retrying` | validator diagnostics | anything but an approval of an honest root | read the custody record | `validator/release_diagnostics.go:223`; `validator/settlement_v2_custody.go:3-8` |
| the byte budget | validator audit | a root's new objects over `--ledger_max_bytes` | raise the flag; it needs no re-sign | VERSION2-FINAL.md §6.6; `validator/settlement_v2_runtime.go:3-5` |
| the watcher's report | `snclaim v2 check` output | any provable fault or "unverifiable" | challenge or demand inside the window | VERSION2-FINAL.md §10.2 |
| claim gas | claim receipts | about 1.27M gas a claim instead of about 0.2M | a third party filled vault2's staking-hotkey list; raise daemon thresholds to about 10 α | §3, §10.1, §17 |
| validator weights | every tempo | a tempo without weights | the escrow hotkey earns by stake in such a tempo; that is surplus | §7.3, §17 |
| the runtime | `sn-mainnet runtime-snapshot` | a new `specVersion` | rehearse vault2 on it (gate 2) | §14, §17 |
| the treasury approval | the native head | block 9,739,882 | renew before it, as a separate successor | §13 |

## 6. Abort and rollback

### 6.1 What each step leaves behind

| Step | Can it be undone? | How |
| --- | --- | --- |
| 1 to 4 | Yes | Nothing in production changed. |
| 5, gas policy | Only forward | A changed policy must be the next revision naming the current digest; the server refuses "operator gas policy rollback or disconnected revision" (`server/model/st_transaction_gas_model.go:389-391`). To lower the caps, sign a further revision with the same approver key. |
| 6, server build | Yes, while v2 is off | Migrations 821 and 822 add new tables only, which v1 settlement does not read (`server/db_migrations.go:10013-10025`). An older binary requires a lower migration head, so it passes readiness (`server/router/warp_handlers.go:255-260`). |
| 7, validator build and app releases | Yes | Without the block, the new validator hashes today's config identically (§13). The released app code hides the v2 claim without `Vault2Address`. |
| 8, contracts | No, but they can be abandoned | vault2 is immutable. Until a pool registers it holds nothing and nothing depends on it (BOOTSTRAP-CONTRACTS.md, "Two residuals remain"). A redeploy needs a new approval, a fresh escrow hotkey and fresh nonces. After step 10 a redeploy also meets the validator's one-way adoption (row 10). |
| 9, `Vault2Address` | Yes, by a release | The compiled default is never stored on the device (`sdk/sn_chain.go:352-355`; `sdk/sn_local_state.go:72-80`), so a release that clears or changes it takes effect on update. This is an inference from the code. |
| 10, adoption | No | The block "can be neither re-pointed nor removed" (`validator/production_authority_history.go:256-259`). Until step 15 the validator still weights through `operatorAt` (§9.1). No repository path re-points it; a redeploy after adoption needs new validator code. |
| 11, server switch | Before step 15 only | Remove the pair from st.yml and sn.yml together, then restart. v1 is untouched. After step 15, do not remove it. Without the pair the v1 driver again closes and commits every epoch, E_s and later included (`server/controller/st_ledger_v2_driver.go:263-266`), against the v1 cutoff (§8.1). And the mirror misses every v2 event while the pair is absent, with no backfill (`server/controller/st_controller.go:2598-2604,4255-4258`). Coming: a server latch, so that once the server has observed the pool's registration it fails closed without the pair. Its exact refusal text will be cited here. |
| 15, pool registration | No | It is the commitment point. `register-operator-pool review` refuses, before any send, what the chain would refuse. A revert changes nothing, and E_s moves (§12.1 step 5). |

### 6.2 After activation

- **Pause.** The guardian or the Safe pauses v1 settlement, and with it v2
  closes, commits and finalizations. Claims, approvals, challenges and
  demands continue (§4.1; §4.3 S5).
- **Upgrade.** The Safe can upgrade `STSettlement2`, which is UUPS (§7.2).
  vault2's guarantees G1 to G6 hold whatever `STSettlement2` does (§4.2). An
  upgrade is new code, reviewed as such, outside phase A.
- **vault2** cannot change. A vault2 defect is permanent for the funds it
  holds (§17, first row).
- **The transfer switch.** Only the SN25 owner `ur-owner`, or the chain's
  root, can freeze claims (`sudo_set_toggle_transfer`, §4.1).

### 6.3 Server and connect order

- **Deploy:** `bringyourctl db migrate`; then the server binaries, api, alt,
  taskworker and `bringyourctl` among them (step 6); then, later, st.yml and
  sn.yml together; then a restart (step 11). Old binaries refuse an sn.yml
  with the v2 keys (`server/provider_payout_transition.go:57-58`).
- **Roll back:** the config first, both files together, then a restart, then
  the binaries. Never roll back the config after step 15.
- **connect** takes no part. No v2 code runs in the connect library or in the
  server's connect service. The v2 routes live in api and alt, and the driver
  in the taskworker (step 6).

### 6.4 What happens to accrued v2 epochs

- **Funding** snapshots are written once and never change (G5).
- **Finalized roots** are all kept. Their accounts stay claimable by anyone,
  for good (G4). Contracts never stop claims (S5).
- **A missed close** defers its epoch with zero funding. The emission stays on
  the pool hotkey, and the next close captures it (§6.4).
- **An unfinalized root** is replaced at the next commit after end(e + 1), and
  its epochs' funding stays (S3). The next root covers every epoch since the
  last finalized root, each allocated from its own usage, at most 96 (§6.4).
- **A long outage**: at 1M providers the default `--ledger_max_bytes` covers
  about 36 epochs of rows not yet fetched; raise it for a longer one (§6.6).
- **While the proof service is down**, the apps show "unavailable", never zero
  (§11). `snclaim` needs the published objects, which the validator retains
  (§6.5).
- **v1** epochs before E_s are unaffected. The phase C sweep recovers v1
  carry and reveal-lag emission (§12.2, §16).

## 7. Risks carried at launch

From VERSION2-FINAL.md §17:

| Risk | Mitigation at launch |
| --- | --- |
| A vault2 defect is permanent for the funds it holds | Gates 1 and 2 (steps 3, 4); claim at least one account after the first finalization (step 20) |
| The rehearsal node is a fast-runtime dev chain, not mainnet | It keeps mainnet's call set, weight formulas and gas schedule; gate 2 cannot be waived; any runtime change repeats it (steps 2, 3) |
| vault2 depends for good on the staking precompile's `getStake`, `getMinerCollateral`, `moveStake` and `transferStake` ABIs | Rehearse each runtime release with vault2's exact bytecode when it is published; encourage claims, which keeps custody small |
| SN25 is dissolved, which strands vault2's custody (accepted by the owner, 2026-10-11) | The `v2_sn25_prune_risk` alert; encourage claims through the daemon and the apps |
| `ur-owner` or root disables stake transfers on SN25 | Nothing is taken or redirected; claims revert and stay claimable; captures continue. Recommended: hold `ur-owner` under the Safe's governance (O25) |
| Someone registers the escrow hotkey between vault2's creation and `registerEscrow` | Back-to-back sends at consecutive nonces (step 8.4); otherwise a redeploy, with no funds at risk |
| Admission bypassed, with the tree withheld | Demands block finalization and may name any finalized root; watchers sample by value (step 13) |
| A finalized root that is not tight, or that lowered an account of an older root, halts honest successors | The validator refuses to build on a root that is not tight; anyone can delete such a successor; recovery needs a reviewed validator release |
| Demand griefing | Each demand costs its gas and the 750,000 rao fee; answers are automatic |
| Stake moves need a gas limit far above their use, and a refused precompile call burns all its forwarded gas | Preflight with `eth_call`, size with `eth_estimateGas`; the re-signed gas policy (step 5); anyone can send a long deferring close |
| Third parties fill vault2's staking-hotkey list | Claims and `absorbStake` keep working at about 1.27M gas; raise daemon thresholds to about 10 α |
| A missed close moves an epoch's share to the next epoch's earners | The 1,200-block window and the alerts (section 5) |
| App users without a paying consent stay unpaid | Network payout consent in the SDK and every app (§11.1), independent of phase A and not a gate. A consent takes effect from the epoch after its signing, and epochs before a network's first consent stay unpaid. Ship it with step 7's release, after ur.io has the consent bridge page, so networks can sign before E_s |
| The owner's availability for Ledger-signed steps | Plan steps 5, 8, 10 and 15 with the owner |
| The treasury approval expires about 2026-12-17 | A separate successor after adoption (step 14 note) |
| Small balances are uneconomic to claim | Accrual never expires; the published threshold; an optional UR subsidy |
| The escrow hotkey earns emission in a tempo without weights | The validator sets weights every tempo; escrow earnings are surplus; claims accept a destination one rao short |
| A later v1 policy breaks `epochBlocks >= rootCommitWindowBlocks + 14,400` | A governance constraint on the v1 policy schedule (§13) |

Found while writing this runbook:

| Risk | Mitigation |
| --- | --- |
| The validator unit carries no `--ledger_auditor_key_file`, so no root is approved | The systemd drop-in of step 10, being written; check the flag in step 12 |
| st.yml and sn.yml disagree, which refuses every mainnet send, v1 included | One change for both files, `sn25golive st-check` and the step 11 stop conditions |
| A failed `vault2.pools` read delays v1 closes and commits | The log line of section 5; RPC health |
| The mirror has no v2 backfill, and without the pair the v1 driver resumes epochs at or after E_s | Switch the server before step 15, and never remove the pair after it. Coming: a server latch that fails closed without the pair once it has observed the pool's registration; its refusal text will be cited here |
| An app is built against a store without its strings | The store's `main` holds the `sn_ledger_*` keys (section 2.1); build each app against it or a later one |
| No CI anywhere | The owner's pipeline and the hand-run suites of section 2.2 |
| The rehearsal tooling builds only runtime 475 | Step 2's check, and a reviewed pin change before any rerun on a new runtime |

## 8. Follow-ups after launch

Open owner decisions and work that the repositories leave open:

1. **The SN25 owner key.** Decide whether and how `ur-owner` comes under the
   Safe's governance (O25; VERSION2-FINAL.md §17; contract review M2,
   STATE.md:91,99).
2. **The treasury approval renewal**, before native block 9,739,882, as a
   separate successor after adoption (§13).
3. **Custody retention.** The validator keeps every ledger object it fetched,
   with no pruning (`validator/settlement_v2_custody.go:3-8`). At 1M providers
   that grows by about 46 MiB of accounts per root and about 54 MiB of rows
   per epoch (§6.6). Decide its retention and disk. Retain the step 8 run
   directory as the v2 deployment record.
4. **The gate 3 CI job.** SV-8's `TestEvmV2*` runs only in the aggregate
   normal job (STATE.md:129; `scripts/test-release-1.0-local.sh`:116-119), and
   the `mainnet` package exceeded the 90-minute budget on the build machine
   (STATE.md:133). No CI runs any of this (section 2.2).
5. **The phase C sweep**, with `E_sweep >= E_s + 8` (§16).
6. **Network payout consent** (§11.1), the largest single increase in
   providers paid. It ships on its own, independent of activation, after
   ur.io has the consent bridge page (step 7). If it misses step 7's release,
   release it as soon as it is ready: a network's app-only providers stay
   unpaid until the epoch after it signs.
7. **The Windows v2 Claim colour**, blue or lime (STATE.md:200).
8. **Tooling gaps** marked in section 4:
   - being closed: the validator unit's auditor flag, through a systemd
     drop-in (step 10);
   - being closed: examples of the v2 phase config (step 8.3), the
     daemon-scope custody request (step 8.2), the claim daemon config and a
     watcher service and timer (step 13);
   - coming: the server latch of section 6.1;
   - build and run instructions for `sn25golive` (step 5);
   - rehearsal pins for a runtime after 475 (step 2);
   - written adoption commands (step 10).

   Closed since the first version of this runbook: the escrow registration's
   envelope (step 8.4) and the `registerOperatorPool` review and readback
   (step 15).

## 9. Schedule for E_s = 2

Owner decision, 2026-10-13: "we are launching v2 now to be active the next ur
epoch". The current epoch is 1, so E_s is epoch 2 (O18).

One chain reading anchors the times below: the public RPC's `eth_blockNumber`
was 9,276,620 at 2026-10-13 20:07 UTC. It is an observation, not a repository
fact. The blocks are exact (step 14). The times assume 12 seconds a block
(`server/controller/st_controller.go:89`), so they are approximate.

### 9.1 Blocks and times

| Block | About, UTC | What | Source |
| --- | --- | --- | --- |
| 9,276,620 | 2026-10-13 20:07 | the reading, in epoch 1 | |
| 9,283,228 | 2026-10-14 18:10 | the plan's registration window opens, 14,400 blocks before E_s starts; about 22 hours after the reading | §12.1 step 5 |
| 9,290,428 | 2026-10-15 18:10 | the plan's registration window closes, 7,200 blocks before E_s starts; about 46 hours after the reading | §12.1 step 5 |
| 9,297,628 | 2026-10-16 18:10 | epoch 2 starts, about 70 hours after the reading. This is the hard on-chain limit: `registerOperatorPool` must be mined before this block, while the current epoch is 1 | `evm/src/STSettlement2.sol:80-82` |
| 9,348,028 | 2026-10-23 18:10 | end(E_s): the close and commit window opens | §5 |
| 9,349,228 | 2026-10-23 22:10 | close and commit by this block | §5 |
| 9,362,428 to 9,363,628 | 2026-10-25 18:10 to 22:10 | the earliest finalization, 14,400 blocks after the commit | §5 |
| after the finalization | | the first claims (step 20) | §17 |

### 9.2 A compressed order of steps 1 to 15

Steps 1 to 12 must be complete before step 15 is sent. Their dependencies set
the order:

| Order | Steps | Runs in parallel | Owner signs |
| --- | --- | --- | --- |
| 1 | 1: freeze `main` after the second merge round, and build | with step 2 | |
| 2 | 2, 3, 4: the runtime check; gate 2 on runtime 478, with its source audit, rehearsal-node build and R-1 to R-7, on sn `v2/runtime-478`; the release gates | with each other | O26, the gate 1 confirmation |
| 3 | 5, 6, 7: the gas policy, the server build with migrations, the validator binary and the tools | with each other | step 5, with the gas approver key |
| 4 | 8: deploy the contracts, after steps 2 to 4 pass, with the phase config naming runtime 478 | its six actions run in sequence | the phase approval, with the approval key; then six transactions, with the deployer key |
| 5 | 10: adopt the validator config, after step 8 and step 7's validator binary | with step 13 | the treasury approval, with its approval key |
| 6 | 11, then 12: the server switch, after steps 6, 8 and 10 | with step 13 | |
| 7 | 15: register the pool, inside blocks 9,283,228 to 9,290,428 | | the Safe |

- Step 8 needs gate 2's verdict on runtime 478 (section 9.3, item 2) and a
  phase config whose runtime is source `850f2755…` with version 478 and the
  hashes of step 2. Steps 6 and 7 must ship the senders' gas floor of section
  9.3, item 2, before the first close and the first claims.
- Step 9, the pinned app releases, moves after step 15 (section 9.3, item 4).
- Step 13 is off the critical path. The watcher must run by the first commit,
  at block 9,348,028 or later.
- Step 14 is decided.
- Known durations:
  - the rehearsal node builds in about 37 minutes from scratch, and the R
    battery runs in about 15 (`scripts/runtime-rehearsal/README.md`:46-50,225-227);
  - the release gate's `mainnet` package alone exceeded 90 minutes on the
    build machine (STATE.md:133);
  - v1's eight contract actions landed within 84 blocks, about 17 minutes
    (LAUNCH.md:36-45).

### 9.3 Departures from the plan of record

Each item needs the owner's explicit acceptance, and this runbook accepts
none of them. Item 2 is not open to acceptance: it is a gate, and step 8
waits for it.

1. **The two-day rule.** "E_s is the first epoch that starts at least two days
   after the last gate passes" (§14). For epoch 2 the last gate, gate 6 at
   step 12, would have to pass by block 9,283,228, about 22 hours after the
   reading. §14 also calls epoch 3 "not reachable", and epoch 2 is earlier.
2. **Gate 2 on runtime 478.** Mainnet installed 477 and then, at block
   9,276,925 on 2026-10-13, 478 (step 2). "Any runtime change on mainnet
   repeats this gate" (§14); "gate 2 cannot be waived" (§17). It was repeated
   on 477 (`docs/spec/runtime-477-audit.md`) and on 478
   (`docs/spec/runtime-478-audit.md`), on sn branches `v2/runtime-477` and
   `v2/runtime-478`: R-1 to R-7 pass on both with the same vault2 bytecode,
   and gate 2 holds for 478. 478 admits `moveStake` and `transferStake`
   against a walk of the caller's own staking-hotkey list, so the limit a
   capture or a claim needs grows with vault2's list, which third parties can
   lengthen after an estimate. Every sender of a stake-moving vault2
   transaction therefore signs it at no less than its pre-478 size: a claim
   at the larger of its estimate with margin and 2,600,000, a capturing close
   at the larger of its estimate with 20% and 3,200,000. snclaim and the claim
   daemon do so on `v2/runtime-478`; the server's close and the SDK's claim
   must before the first close and the first claims.
3. **O26.** Gate 1 needs the independent review of vault2, `LedgerTree.sol`
   and `STSettlement2` resolved (§14). The owner's confirmation is not given
   yet, and step 8 waits for it.
4. **The apps.** Activation needs v2 claims in the apps (§2.1 decision 8; §11;
   §14 gate 5). The releases with the pinned `Vault2Address` (step 9) cannot
   precede step 15 here. Claims are impossible before the first finalization,
   at block 9,362,428 or later, so the apps have until then.
5. **Consent.** A consent takes effect from the epoch after its signing
   (§11.1), so only one signed before block 9,297,628 covers epoch 2. Consent
   signing is in the second merge round, in no released app (Pins), and ur.io
   is not yet deployed with the bridge page the apps sign it through
   (STATE.md:268,297). So no
   app-signed network consent can be in effect for epoch 2, and app-only
   providers stay unpaid in it, as in v1.
6. **Other validators.** A validator weights the v2 pool UID only through its
   `settlement_v2` block; without it, it resolves every epoch through
   `operatorAt` (§9.1). Any third-party validator must adopt before E_s.

### 9.4 Decision point

If any stop condition fires, or the registration window of section 9.1 is
missed, E_s moves to epoch 3, which starts at block 9,348,028. Its
registration window is blocks 9,333,628 to 9,340,828. The chain itself takes
a registration for epoch 2 until block 9,297,627; the plan's window is the
earlier one.

Nothing is lost before step 15: "If it fails, nothing has changed, and E_s
moves" (§12.1 step 5). What steps 5 to 12 put in place stays as section 6.1
describes it.
