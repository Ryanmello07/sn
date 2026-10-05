# Treasury emission proposal — October 5, 2026

The selected launch policy is **10% of the native miner allocation for providers
and 90% retained for future network improvements**. This supersedes the September
27 owner-recycle choice. Retain the full miner tranche for distribution instead
of deliberately recycling its remainder. This document defines the successor;
implementation, qualification, key provisioning and activation remain pending.

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

The proposed custody reader uses the existing environment resolver convention:
`Vault.SimpleResource("sn.yml")` in environment `main` resolves the selected vault
resource. Its physical identity and exact bytes must then be retained for the
operation; an environment fallback cannot select another network or custody.
No such treasury consumer exists in the reviewed baseline. The independent
`Config.SimpleResource("sn.yml")` reader in Server already parses the public
earnings schedule strictly. Do not insert the following fields into
`config/main/sn.yml`, replace that schedule, or reuse `st.yml`'s legacy
`treasury_hotkey` deposit-staging field.

This non-secret template defines the custody parser being implemented. Empty
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
recipient_hotkeys:
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

The separate signed economic policy binds these public fields; the custody file
cannot authorize emissions or spending by itself:

| Public policy field | Required meaning |
| --- | --- |
| Network/runtime and predecessor policy | Exact approved network, runtime/source, advancing policy/activation boundary and unchanged historical authority |
| Multisig account, threshold, sorted signatories | Derived native account with unique signatories and threshold `2..N`, `N <= 100`; no device paths |
| Treasury recipient roster | At least two exact hotkey/UID/registration generations owned by that multisig; no provider-role overlap |
| Allocation denominator | Full actual native miner allocation before distribution, excluding other emission tranches and principal |
| Distribution fractions | `1/1` distributed: providers `1/10`, treasury `9/10`; no deliberate owner recycle or deferred provider liability |
| Per-recipient cap and assurance | Existing `32768/65535` cap and observed-native-target with exact runtime tolerance; theta applies only to providers |

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
or spending readback. Hardware signing, wrapper submission and recovery need
their own production implementation and qualification; a descriptor parser does
not supply them.

## Allocation and evidence

Let `M` be cumulative actual native miner allocation before distribution, in
alpha atomic units, over exact activated intervals. Preserve the cumulative
provider reference `P = floor(M / 10)` and define treasury reference `T = M - P`.
Theta divides only `P` between provider head and tail; paid/free completed traffic
keeps equal weight. Treasury retention creates no deferred provider entitlement.
Exclude owner cuts, validator/root dividends, deposits and existing principal
from `M`. Separate earned collateral capture, truncation dust and any actual
unplanned recipient or owner-withholding outcome.

The submitted 90/10 row is a target. Yuma masks, other validators, clipping,
normalization and integer rounding determine actual incentive. Preserve the
observed-native-target assurance and runtime-derived tolerance; no hard payout
guarantee follows from owner control. Removing deliberate owner withholding
removes that component of `MinerBurned`; subsequent price-share renormalization
and emission gating still determine the subnet's allocation.
[Yuma source](https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/epoch/run_epoch.rs#L750),
[emission shares](https://github.com/RaoFoundation/subtensor/blob/923fd1fa7d6eadad3ec16f3941826b86c9c3aa1d/pallets/subtensor/src/coinbase/subnet_emissions.rs#L350).

## Implementation and launch consequence

1. Add a distinct signed economic successor and treasury recipient role to the
   [proposal/admission path](../validator/recycle_planner.go). Current validation
   accepts only recognized-owner recycling. Preserve old signed bytes, pending
   work and archive interpretation; they cannot acquire treasury authority.
   The validator lane owns this policy and producer integration; the bootstrap
   lane owns the public custody parser, pure native multisig derivation and
   owner-local hardware transaction lifecycle.
2. Join the new policy through production measurement, weight preparation,
   approvals, bootstrap and reset. Bind exact coldkey ownership, UID/hotkey
   generations, owner-set exclusion, masks and caps at the applicable native
   execution boundary. Ordinary treasury registrations lack owner immunity:
   monitor pruning, re-registration, ownership changes and permit effects.
   Missing recipients must not silently renormalize 90% onto providers or the
   remaining treasury UID. Reconcile original pending work before replacement.
3. Extend [native references](economic_reference.go),
   [execution accounting](economic_native_accounting.go),
   [recipient attribution](economic_native_attribution.go), conservation and
   monitoring with authenticated treasury earnings, locked/liquid stake and
   later outflows. Current code fixes reserve credit at zero, expects 90% recycle
   and classifies nonprovider miner credits as residual. Do not rename those
   historical amounts into treasury credit. Prove provider and treasury outcomes
   independently, with exact native receipts and custody reconciliation.
   The native-accounting lane owns these projections and their explicit schema
   successor, with absent optional fields preserving legacy hashes and replay.
4. Qualify the changed path, ownership/generation failures, pruning/recovery,
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
