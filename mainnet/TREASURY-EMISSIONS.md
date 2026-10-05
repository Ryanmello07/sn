# Native treasury emissions — October 5, 2026

The selected launch policy is **10% of the native miner allocation for providers
and 90% retained for future network improvements**. This supersedes the September
27 owner-recycle choice. Retain the full miner tranche for distribution instead
of deliberately recycling its remainder. This document defines the explicit
successor and its custody command interface. The source changes are being
integrated; execution qualification, actual identity provisioning and activation
remain pending.

## Native routing and custody

The reviewed runtime is v470, source
`923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d`. Its owner-directed incentive branch
recycles or burns; it never credits a treasury. Recycling reduces outstanding
alpha and issuance. Changing `RecycleOrBurn` cannot select a wallet.
[Distribution source](https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/coinbase/run_coinbase.rs#L659),
[alpha accounting](https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/coinbase/alpha.rs#L38).

Use a dedicated native multisig treasury coldkey owning at least two ordinary
registered SN25 hotkeys. Its signatories and any reserve hotkey signing keys stay
on their owners' Ledger devices; no private keys or seeds leave the hardware.
The multisig account must differ from `SubnetOwner`; neither recipient may belong to
that owner's registered `OwnedHotkeys` or equal `SubnetOwnerHotkey`. The existing
signed `32768/65535` per-recipient cap requires at least two usable treasury UIDs
for a 90% row, initially 45% each. These are explicit treasury recipients, never
invented provider contributions. Registration uses ordinary coldkey authority,
subject to current eligibility, capacity, fees and collateral; no chain-root
allocation override is assumed. An owner key alone cannot force the result.
[Registration source](https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/subnets/registration.rs#L70).

Ordinary miner rewards become native alpha stake controlled by the recipient's
coldkey. Record collateral capture separately from withdrawable stake. This is
not an automatic free-TAO, ERC20 or EVM-wallet payment. Future spending requires
the treasury's authorized native transfer or conversion, with its locks, fees
and receipts. Optional `AutoStakeDestination` changes the staking hotkey while
keeping coldkey ownership. Select and qualify that policy explicitly: growing
stake on treasury miner hotkeys can affect top-k validator permits. Preserve
independent validator participation and account for any additional dividends.
[Autostake authority](https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/macros/dispatches.rs#L1949),
[permit selection](https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/epoch/run_epoch.rs#L673).

The user is preparing this separate hardware multisig. `vault/main/sn.yml`
contains its **public descriptor only**: account, threshold, public signatories,
recipient hotkeys and references to owner-local Ledger configuration. No coldkey
or hotkey seed, private key, mnemonic or online signer belongs there. Native
incentive reception needs no always-on treasury signer. This review does not read
or create the actual vault file or choose identities. Validator inputs receive
only the public policy projection below. Establish hardware recovery and rotation
before activation; changing signatories or threshold changes the native account.

Direct native treasury custody requires no new Solidity contract.
[`STSettlementVault`](../evm/src/STSettlementVault.sol) continues to secure the
provider pool's claims. [`STReserveSink`](../evm/src/STReserveSink.sol) remains an
immutable one-way deposit sink with no spending operation. Neither its principal
nor unclaimed vault balances become treasury funds. An EVM treasury alternative
would require separately qualified ownership, precompile and withdrawal behavior.

## Public configuration contract

The [custody reader](treasury_config.go) opens an explicitly selected absolute
file path under an independently accepted SHA-256 pin. Use the mainnet tool's
`treasury describe --custody FILE --custody-sha256 HASH` command, with `FILE` set
to the actual absolute path of the anticipated `vault/main/sn.yml`. The CLI has
no `Vault.SimpleResource("sn.yml")` lookup or environment fallback. Describe
validates only the public descriptor and does not open its device references.

The independent `Config.SimpleResource("sn.yml")` reader in Server parses the
public earnings schedule strictly. Do not insert the following fields into
`config/main/sn.yml`, replace that schedule, or reuse `st.yml`'s legacy
`treasury_hotkey` deposit-staging field.

This non-secret template matches the custody parser. Empty
identities, zero threshold and empty file references are deliberately invalid;
they are not runnable defaults or an assertion about the user's threshold.

```yaml
schema: urnetwork-native-treasury-custody-v1
profile: mainnet
netuid: 25
genesis_hash: ""                       # independently approved native hash
multisig:
  account_id: ""                       # derived native AccountId32
  threshold: 0                         # actual approved threshold, 2..N
  signatories:                         # all N accounts, sorted by raw bytes
    - account_id: ""
      signature_scheme: ed25519        # selected native Ledger workflow
      device_config: {path: "", bytes: 0, sha256: ""}
    - account_id: ""
      signature_scheme: ed25519
      device_config: {path: "", bytes: 0, sha256: ""}
recipient_hotkeys:                     # sorted by raw AccountId32
  - account_id: ""
    device_config: {path: "", bytes: 0, sha256: ""}
  - account_id: ""
    device_config: {path: "", bytes: 0, sha256: ""}
```

Account IDs and native hashes use canonical nonzero `0x`-prefixed lowercase
32-byte hex, matching native admission. An SS58 display address must decode to
the same account; a 20-byte EVM address is not a substitute. File references use
the existing `path`/`bytes`/`sha256` shape: absolute owner-local path, positive
exact byte count and canonical `0x`-prefixed SHA-256. They reference public
Ledger configuration and are never executable commands, seeds or validator
signing files. No validator export includes these local device paths.

The descriptor admits two through 100 recipient hotkeys, distinct from the
multisig account and every signatory. The signed validator policy has its own
bounded roster validation; parsing a custody descriptor is not equivalent to
admitting that policy. The separate signed economic policy binds these public
fields; the custody file cannot authorize emissions or spending by itself:

| Public policy field | Required meaning |
| --- | --- |
| Network/runtime and predecessor policy | Exact approved network, runtime/source, advancing policy/activation boundary and unchanged historical authority |
| Multisig account, threshold, sorted signatories | Derived native account with unique signatories and threshold `2..N`, `N <= 100`; no device paths |
| Treasury recipient roster | At least two exact hotkey/UID/registration generations owned by that multisig; no provider-role overlap |
| Allocation denominator | Full actual native miner allocation before distribution, excluding other emission tranches and principal |
| Distribution fractions | `1/1` distributed: providers `1/10`, treasury `9/10`; no deliberate owner recycle or deferred provider liability |
| Per-recipient cap and assurance | Existing `32768/65535` cap and observed-native-target with exact runtime tolerance; theta applies only to providers |
| Auto-stake destination | Omitted means authenticated absence; a supplied public AccountId32 must match original native storage exactly |

Owner-set exclusion, multisig derivation, identity uniqueness and cap checks are
mandatory validation, never caller-disableable flags. Reject unknown fields,
duplicate keys/identities, multiple YAML documents, excessive input and incomplete
references. Require authenticated native ownership and registration readback;
public key possession or successful parsing alone supplies neither.

## Hardware multisig execution

Pinned v470 installs native `Multisig` at pallet index 13 using SDK
`cacb4310f20c7cac83eb3ccd8ed5a5ad4212608a` (`pallet-multisig` 41.0.0).
Its deterministic account derives from the sorted complete signatory list and
threshold. Threshold execution dispatches the inner call with the multisig
account as signed origin, so ordinary registration can make that account the
hotkey owner. This is independent of EVM Safe.
[Runtime configuration](https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/runtime/src/lib.rs#L555),
[native multisig source](https://github.com/RaoFoundation/polkadot-sdk/blob/cacb4310f20c7cac83eb3ccd8ed5a5ad4212608a/substrate/frame/multisig/src/lib.rs#L618).

Prepare bounded registration calls and explicit multisig approvals offline. Each
owner signs their exact outer extrinsic on their local Ledger; a coordinator may
retain and submit only returned signed bytes. Bind the original call hash,
signatory, threshold, other-signatory order, nonce, mortal era, runtime metadata,
maximum weight, deposit/fee allowances and initial approval timepoint. Preserve
pending approvals and unknown results across restart; cancellation belongs to
the original depositor. An outer successful extrinsic is insufficient: require
the matching `MultisigExecuted` inner result and actual registration/ownership
or spending readback.

The [treasury commands](treasury_command.go) implement this separation:

| Stage | Commands and retained authority |
| --- | --- |
| Read and review | `describe` validates the pinned descriptor. `observe`, `plan` and `policy-plan` consume an explicit `--input` JSON, optionally binding that same descriptor with `--custody` and `--custody-sha256`. Observation pins native finality; plans remain unsigned. `policy-plan` returns `approved: false`. |
| Permanent host custody | `reserve`, `export`, `import-reply`, `status`, `reconcile` and `submit` require the original signed `--config`, independent `--approval-key`, `--accept-action-hash` and `--production-authority-hash`. Export retains original metadata; imported replies require an exact file pin. Unknown outcomes retain the original request and bounded submission allowance. |
| Owner-local Ledger | `inspect-request`, `ledger-plan` and `sign` require the original `--request`, independently accepted request hash, signatory account, genesis and approval key. `sign` additionally requires the descriptor's exact `--device-config` and permanent `--owner-state`; no host private key is accepted. |

The device reference resolves to a public
`urnetwork-native-treasury-ledger-device-v1` JSON containing the expected account,
derivation path, absolute Python/helper/backend paths, helper/backend SHA-256
pins and app version. The supported signing path is Ed25519 at
`m/44'/354'/account'/0'/index'`. The local journal retains a returned signature;
an unresolved device issuance cannot trigger a fresh signing call. A verified
signature alone does not prove Ledger provenance; qualify the actual device
workflow before use.

Supported inner calls are bounded SN25 `register_limit`, `remove_stake_limit`
with partial execution disabled, and `transfer_keep_alive`, wrapped in native
`approve_as_multi`, `as_multi` or `cancel_as_multi`. Liquidation checks both the
selected stake position and coldkey-wide availability, including collateral.
Cancellation retains the original depositor and timepoint. There is no
auto-stake setter in this command path: begin with an absent destination or
separately authorize its native management, then bind its exact observed value
in the economic policy. Source implementation does not establish successful
hardware signing, native execution or launch authority.

## Allocation and evidence

Let `M` be cumulative actual native miner allocation before distribution, in
alpha atomic units, over exact activated intervals. Preserve the cumulative
provider reference `P = floor(M / 10)` and define treasury reference `T = M - P`.
Theta divides only `P` between provider head and tail; paid/free completed traffic
keeps equal weight. Treasury retention creates no deferred provider entitlement.
Exclude owner cuts, validator/root dividends, deposits and existing principal
from `M`. Separate earned collateral capture, truncation dust and any actual
unplanned recipient or owner-withholding outcome.

The [original native availability query](historical_principal.go) retains the
runtime API's raw SCALE response and coldkey-wide total, locked and available
alpha at both boundaries. `StakeInfo.locked` alone is insufficient on v470.
Captured rewards are already staked and must not be added to principal twice;
available alpha also accounts for collateral. Deduplicate a shared coldkey's
availability across recipient queries. Complete custody reconciliation requires
the retained stake positions and original causes to cover that coldkey's total;
untracked positions leave the custody conclusion unqualified. Later liquidation
and spending are separate native outflows, never negative provider earnings.

The submitted 90/10 row is a target. Yuma masks, other validators, clipping,
normalization and integer rounding determine actual incentive. Preserve the
observed-native-target assurance and runtime-derived tolerance; no hard payout
guarantee follows from owner control. Removing deliberate owner withholding
removes that component of `MinerBurned`; subsequent price-share renormalization
and emission gating still determine the subnet's allocation.
[Yuma source](https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/epoch/run_epoch.rs#L750),
[emission shares](https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/coinbase/subnet_emissions.rs#L350).

## Implementation and launch consequence

The frozen [validator successor](../validator/TREASURY-PRODUCTION.md) uses an
explicit `treasury_approval` selector, distinct signed domains and the complete
ordinary recipient roster through measured preparation, submission and recovery.
It preserves original owner-recycle bytes and read-only historical authority.
The frozen multisig implementation supplies the public parser and the custody
commands above. Native accounting and monitoring add explicit treasury schemas;
legacy recycling and residual amounts must retain their historical meaning.
These source changes and the original availability capture/replay fixture still
require integration and execution qualification together.

1. Bind exact coldkey ownership, UID/hotkey generations, owner-set exclusion,
   masks and caps at the applicable native execution boundary. Ordinary treasury
   registrations lack owner immunity: monitor pruning, re-registration,
   ownership changes and permit effects. Missing recipients must fail the
   complete row. Retain prior stake positions through recipient replacement or
   destination rotation; reconcile pending work before replacing its authority.
2. Prove provider and treasury outcomes independently using
   [execution accounting](economic_native_accounting.go), original native
   receipts, collateral and custody reconciliation. A synthetic original
   capture/replay fixture qualifies mechanics only; it does not admit the live
   runtime or demonstrate a mainnet 10/90 outcome.
3. Qualify the changed path, ownership/generation failures, pruning/recovery,
   cap/mask failures, consensus divergence and treasury conservation. The owner
   recycle-mode transition is no longer a prerequisite for the ordinary treasury
   remainder; any actual withholding remains measured. Reassess deployment and
   readiness using the completed successor, then observe the required native
   intervals and settlement/claim cycle after authorized activation.

Keep the existing 38-requirement ledger and retained qualification evidence.
This decision changes the economic outcome those requirements must demonstrate;
it supplies no additional test pass or launch authority. Canonical
`config/main/sn.yml` remains `activation: blocked`. The inclusive October 6
new-earnings boundary and all pre-cutoff USDC obligations remain unchanged.
