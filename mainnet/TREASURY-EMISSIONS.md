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

Use a dedicated native treasury coldkey owning at least two ordinary registered
SN25 hotkeys. It must differ from `SubnetOwner`; neither recipient may belong to
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

Provision private treasury coldkey material or its signing configuration/reference
in `vault/main/sn.yml`; this path was absent during this review. Do not guess an
address, reuse the subnet-owner/root custody, or create keys as part of this
documentation change. Validator and public deployment inputs receive only the
approved public coldkey/hotkey identities, registration generations and policy
hashes. Keep spending authority outside validator services and establish recovery
and rotation before activation.

Direct native treasury custody requires no new Solidity contract.
[`STSettlementVault`](../evm/src/STSettlementVault.sol) continues to secure the
provider pool's claims. [`STReserveSink`](../evm/src/STReserveSink.sol) remains an
immutable one-way deposit sink with no spending operation. Neither its principal
nor unclaimed vault balances become treasury funds. An EVM treasury alternative
would require separately qualified ownership, precompile and withdrawal behavior.

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
