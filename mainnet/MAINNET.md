# Mainnet launch and operations plan

Updated 2026-09-27. **Mainnet activation is blocked.** The read-only
[Snow/LAN RPC comparison](evidence/snow-rpc-route-20260927.json) showed that
`http://172.28.208.185:9944` served the same **testnet** chain as
`http://192.168.1.162:9944`: EVM chain ID **945** (`0x3b1`), rather than the
expected mainnet ID 964. During the operator's node-data move, the
[18:57 UTC read-only retry](evidence/snow-route-inspect-20260927-1857.json)
returned HTTP 502 for both chain ID and genesis reads. That is an unavailable
route, not a mainnet identity. Verify the restarted route and independently
approve the mainnet chain identity before admitting any signer.

Sim-testnet is closed with known exceptions at the user's direction. The
[original R48 report](../sim-testnet/FINAL-4.md) remains a failed provisional
attempt with **zero completed acceptance epochs**. Its later retained resume
recovered services with no setup actions dispatched and explicitly retained
`final_acceptance=false`; it did not produce final acceptance. The process-log
scanner overrun, missed native epoch, policy-scoped client-key rollover,
historical R46 handoff provenance bug and usage debt are mapped to concrete
production work in [the gate tracker](PRELAUNCH-FIXES.md#production-gates-in-execution-order).
Do not restart testnet or make a passing testnet result a fictional input to
the mainnet plan.

The original bootstrap design was based on SN
`a59294e98ea02d05125015ae02cf32f2c0059c8a`. Subsequent implementation and
qualification must use an explicit composed release, including compatible
SN/server/SDK/Connect/config revisions. Some shared and simulator fixes exist;
the complete mutating bootstrap, root-validator service and operational repair
system remain production work. [mainnet/main.go](main.go) implements signer-free
`inspect`, `runtime-snapshot`, `finalized-mapping`, `finalized-snapshot`,
`monitor`, `subnet-preview`, `owner-trim-plan`, `owner-trim-recheck`,
`owner-trim-reconcile`, `owner-trim-qualify`, `root-preview`, `root-monitor`,
`check-recycle-mode`, `economic-reference`, offline `source-lock`,
[local `release-inventory`](RELEASE-INVENTORY.md), and the
[signer-free blocked `plan`](PLAN.md); executable planning, `apply`, `resume`
and root-validator signing are not implemented. The
[retained Snow inspection](evidence/snow-route-inspect-20260927-1051.json)
observed chain ID 945, which fails the required mainnet ID 964 gate.
This plan and its read-only evidence perform no
mainnet transaction, deployment, UID removal or validator activation.

## Bootstrap prerequisites and current blockers

| Prerequisite | Current disposition and next result required |
| --- | --- |
| Owned mainnet RPC and independent identity authority | The [10:51 UTC read-only inspection](evidence/snow-route-inspect-20260927-1051.json) observed testnet ID 945 at Snow `:9944`; the [18:57 UTC retry](evidence/snow-route-inspect-20260927-1857.json) returned HTTP 502 during the node-data move. Reinspect after cutover and obtain separately approved genesis, expected EVM ID 964, native/EVM finalized mapping, code/metadata and node/source identity. An operator-approved mainnet genesis is still outstanding. |
| Immutable qualified release | Compose the actual SN/server/SDK/Connect/config and contract artifacts, including selected branch fixes and migration order; qualify their real interfaces and publish an approved manifest. Historical R48 builds do not qualify later per-user deposit or zero-price changes. |
| Exact mainnet census and authority | Read SN25 membership, roles, custody, immutable contracts and locks at one finalized snapshot; resolve reset feasibility and all protected identities before making an executable plan. |
| Economic and custody decisions | The user selected **owner-recycle for the remaining 90%**. Implement and qualify that path and the 10% native-miner target on the actual runtime; finalize mainnet policy, tolerance, keys/Safe, root-registration protection and spend/count/expiry ceilings. Recycled value is not reserve custody. No testnet allowance carries over. |
| Production safety and liveness | Close the linked recovery, runtime, policy/identity, settlement and monitoring gates; retain independent history and complete signature/nonce ownership. Testnet provisional exceptions grant no mainnet authority. |
| Operations and staged acceptance | Install monitor/alerts in the existing telemetry stack, name primary/backup on-call, rehearse bounded repair and rollout/rollback, then collect actual native and settlement evidence after an approved activation. |

Read-only inspection and offline implementation can proceed while required
inputs remain unresolved. The planner must expose those blockers and refuse
mutating phases until their exact dependencies and authorization are complete.
The current Snow xops `vars.yml` still selects `testfinney` with EVM ID 945 and
the testnet genesis. Its [prepared cutover guard](https://github.com/urnetwork/xops/commit/ec443da)
is not deployed: it requires the data mount in both full-host and isolated
lightnode rollouts, renders a network-specific bootnode including finney's
`/ws` transport, and rejects mixed testnet/mainnet identity inputs. Before
starting the mainnet node, select distinct reviewed node generations, the
approved finney genesis and runtime pins, EVM ID 964, bootnode host/port/peer,
and reference route as one configuration. Observe the started node's finalized
identity; configuration checks alone do not establish it.

## Requested outcome and decisions

The bootstrap must deliver all four requested outcomes:

1. Reset the existing miner registrations on our UR subnet (Bittensor SN25, netuid 25), with an exact census and an explicit meaning of reset.
2. Install and initialize the production contract set with the approved custody and governance identities.
3. Begin provider rewards at **10% of the native miner allocation**. This is not 10% of all subnet emission, not a validator take, and not the head/tail steering parameter.
4. Operate both an owned **root validator on netuid 0** and an owned **validator on the UR subnet**.

The target UR mainnet netuid is SN25 (netuid 25). The user selected `owner-recycle` for the other 90% on 2026-09-27; this recycles native allocation and does not fund our reserve. The verified mainnet RPC route, keys, spend ceilings, reset mechanism and exact runtime implementation remain inputs to the executable plan. None has a default mainnet address or financial allowance. Existing testnet spend approvals do not authorize mainnet spend.

Two constraints determine the implementation. There is no demonstrated subnet-owner call that arbitrarily clears every miner registration while retaining an arbitrary list of validators. Also, the current UR contracts and validator policy do not provide a standalone switch that changes the native miner allocation to 10%. The planner must expose these as capability decisions, not claim that lowering UID capacity or setting `theta: 0.1` fulfills them. The requested 10% target allows the exact runtime's explicitly established quantization tolerance; a stronger enforceable hard cap is a separate assurance choice, not an additional user requirement.

The read-only root observer and existing UR validator do not implement either
the root-validator signing service or the 10% native-miner mechanism. Both remain
explicit mainnet implementation gates. New zero-price/equal-demand support
changes operator demand/deposit semantics; it does not by itself cap the native
miner allocation or choose where the remaining 90% goes.

The miner fleet now has a [mainnet runtime authority gate](../miner/FLEET-MAINNET-RUNTIME.md)
for register, publish, bind, status and revoke. It requires separately approved
genesis, source/build review and exact code/metadata/version bytes before
signing, submission and receipt readback. That source change does not supply
those approvals or durable uncertain-send reconciliation.

The draft policy is `reset.mode: unresolved` and **`emissions.remainder: owner-recycle`**. A preview remains non-executable while reset capability, the runtime-specific 10%/90% mechanism or other required inputs are unresolved. Qualify the selected economic mechanism before installing an immutable vault or removing existing registrations. The remainder choice is settled; its implementation and exact signed production policy remain work.

## Source and runtime boundary

The September 14 design inspected Subtensor [commit `67dcf7f791dc495064c293f080a0702cb433e51e`][subtensor-commit], dated 2026-09-07, following the release-455 merge, in `RaoFoundation/subtensor`. The source-specific capability observations below describe that baseline, **not an attestation of the current mainnet Wasm**. Recheck them against the selected live runtime and approved artifacts before planning any action.

The production inspection gate must authenticate one finalized native block and its corresponding canonical EVM block using a separately verified, operator-owned mainnet node. Require an independently approved genesis hash, EVM chain ID 964, native chain identity, complete runtime version, `:code` hash, metadata hash, node build identity, and the reviewed runtime source/artifact mapping. Verify the finalized native header's complete SCALE bytes against its hash before using that hash for state reads; a header number and same-height lookup are insufficient. Decode one complete runtime identity, rejecting contradictory `stateVersion`/`systemVersion` aliases. The `runtime-snapshot` command captures exact finalized code and metadata bytes, verifies code against its storage hash, and repeats canonical/network checks after reading them. Its output remains an unapproved observation. Admission still needs signed-extension, call, storage and precompile review, an independently reviewed source-to-Wasm mapping, and native/EVM finalized mapping. A matching `specVersion` alone is insufficient; [the existing runtime authenticator](../crv4/runtime_identity.go) already binds more than that number.

For native/EVM mapping, the current Snow testnet header carries a Frontier
`fron` consensus digest whose payload names an EVM block hash. The signer-free
[`finalized-mapping` command](FINALIZED-MAPPING.md) authenticates the native
header, decodes the reviewed digest variant, fetches raw EVM RLP by that hash
with canonicality required, reproduces its Keccak hash, and checks canonical
lookup using the EVM header's decoded number. Equal block numbers alone are
not a mapping. Its [Snow evidence](evidence/finalized-mapping-snow-20260927.json)
remains unapproved until the selected mainnet identity, runtime and source
artifact are independently reviewed. The signer-free
[`finalized-snapshot` command](FINALIZED-SNAPSHOT.md) now captures runtime
bytes and this mapping under one authenticated finalized native hash, with
final canonical rechecks after both reads. Its [Snow evidence](evidence/finalized-snapshot-snow-20260927.json)
reproduces code, metadata, native header and EVM header hashes at that one
block. Two separate latest-head observations still cannot be joined into one
launch-plan authority merely because their chain IDs match.

The read-only observation at **2026-09-27 04:16:25 UTC** compared Snow VPN
`http://172.28.208.185:9944` with LAN testnet `http://192.168.1.162:9944`.
Both returned `system_chain=Bittensor`, `eth_chainId=0x3b1` (945), node version
`4.0.0-dev-e18ca67f1a0`, genesis
`0x8f9cf856bf558a14440e75569c9e58594757048d7b3a84b5d25f6bd978263105`,
and finalized native head
`0x3e9119c77dcb7b12557035023f9ad3dbadc60f24443d01c0f32a6c14d81f35d7`.
The [raw identity record](evidence/snow-rpc-route-20260927.json) has
`same_identity_and_head=true`. This is an observed **testnet genesis and route**,
not an approved mainnet genesis. A node's `Bittensor` display name is insufficient
network authority. Both routes must be rejected for mainnet in this state.
The later [07:41 UTC Snow readback](evidence/snow-route-inspect-20260927-0741.json)
still returned EVM ID 945 and the same testnet genesis; no mainnet route cutover
has been observed.

A later mainnet deployment at either address requires the correct owned route,
fresh readback and independent operator approval of the genesis/runtime domain.
Do not guess a different port or inherit a library/public fallback. Bind RPC URLs,
resolved upstreams, TLS identities where applicable and local proxy routes in the
plan. The Foundry configuration no longer defines public `mainnet` or `testnet`
RPC aliases; deployment and probe commands require an explicit owned URL. A
loopback proxy must have the approved owned mainnet node as its sole
upstream. Preserve zero artificial request pacing on that route; bound
concurrency, retries and cancellation. A separately approved read-only comparison
node is an independent observer, never a silent signing/submission fallback.
Protocol block windows and on-chain rate limits still apply.

At admission, record native and EVM clocks separately. Verify their mapping; do not assume equal height or treat an EVM receipt as native finality. Historical reads must remain at the receipt's authenticated block. Until RT-01 through RT-08 are qualified for production, an unknown runtime stops dependent new signing pending explicit adapter admission. The target operating model automatically admits a compatible consumed profile under the approved compatibility policy, retains exact historical identities and suspends only unsupported operations. A testnet provisional profile alone cannot authorize that production behavior.

Retain the authenticated runtime proof with each historical or signing view;
metadata-cache eviction must not revoke that view or force its immutable audit
to run again. The [RT-06 correction](evidence/runtime-proof-eviction-20260927.md)
implements this ownership boundary for provisional consumers while retaining
fresh block/chain identity checks, exact signing domains and strict mainnet
rejection. A new connection must establish its own authority; the correction
does not qualify automatic production runtime admission or durable proof reuse.

Current source changes matter to this design:

| Subject | Source-backed observation | Bootstrap consequence |
| --- | --- | --- |
| Subnet emission allocation | The inspected `get_shares` uses price EMA, a `1 - MinerBurned` adjustment, then an emission gate. A flow-based helper also exists but is not the selected `get_shares` path. [Source][subtensor-shares] | Do not assume an older Taoflow formula or a root-validator vote controls our subnet's allocation. Attest the actual runtime path. |
| Root weights | Root Reborn uses a validator's root weights for its own dividend basket. This differs from historical global subnet-allocation voting. [Official guide][root-reborn] | Implement root operation separately from UR miner scoring; do not send UR UID weights to netuid 0. |
| Miner collateral | Registration collateral can survive deregistration; later earnings can affect release and capture. [Official collateral guide, pinned source][collateral-guide] | A UID reset is not a balance, lock, or stake reset. Pool capture must distinguish emission, locked collateral, and principal. |
| Native versus signed limits | The local whitepaper records runtime-dependent weight-limit behavior and requires a signed policy cap. [Local specification](../WHITEPAPER.md#15-concrete-parameters) | Observe runtime getters and enforce the signed cap independently. Do not assume a successful setter changed native enforcement. |

When documentation and the exact runtime source disagree, record the discrepancy and resolve it against the authenticated runtime. For example, the inspected emission-enable implementation affects pool-side injection while retaining participant-side emission; it cannot serve as an owner-controlled miner payout pause. [Storage contract][subtensor-storage]

## Authority and capability matrix

The word “root” identifies three different things here: the Substrate `Root` origin, a netuid-0 validator, and a UR settlement Merkle root. None grants either of the other authorities.

| Action | Required authority | Real limit and admission check |
| --- | --- | --- |
| Inspect finalized chain state | Read access to the owned node | No signer; authenticate chain and block before interpreting storage. |
| Change supported subnet parameters or request trimming | Subnet-owner coldkey, or a genuinely authorized chain `Root` origin | Owner calls have individual permissions, rate limits and administrative windows. A call name beginning with `sudo_` does not itself mean the owner has chain Sudo. |
| Change maximum validator permits or the global subnet-owner cut | Chain `Root` in the inspected implementation | Root-validator registration and the UR owner wallet do not satisfy `ensure_root`. No automatic governance proposal or Sudo attempt. |
| Remove every selected miner | Depends on an actually supported mechanism | No general owner-authorized arbitrary bulk removal has been established. See the reset alternatives below. |
| Register a UR hotkey and acquire stake | Its coldkey or a proven permitted proxy/contract origin | Registration, burn, collateral, pool price, capacity and eligibility are independent checks. |
| Register the root-validator hotkey | Its coldkey through the root registration path | Does not confer administrative authority. The native call's burn-price limitation needs special handling below. |
| Submit UR consensus weights | The registered UR validator hotkey | Correct permit/eligibility, stake, activity, mechanism and CRv4 timing are required. |
| Manage a root dividend basket | The registered root-validator hotkey | Root-specific stake, enablement, diversity, concentration and timing constraints apply. |
| Deploy EVM contracts | Dedicated EVM deployment signer | Exact nonce, creation bytecode, constructor data, gas and value envelopes. |
| Govern the coordinator | Approved EVM 2-of-3 Safe | Safe authorization does not authorize native subnet-owner calls. |
| Pause permitted coordinator actions | Configured guardian under contract rules | Cannot claw back reserve principal, rewrite earned claims or pause valid vault claims. |
| Withdraw immutable reserve or upgrade the settlement vault | No such release-1.0 authority | Reject any proposed action requiring this capability. |

The inspected admin implementation permits owner-limited trimming and selected parameters; maximum validators and owner-cut setters require chain `Root`. The emission-enable setter also requires `Root` and is not a percentage setter. [Pinned admin implementation][subtensor-admin]

Every planned transaction records its actual origin: native account or proxy real account, EVM sender, Safe address and threshold, and the exact role it exercises. Prove ownership and proxy filters from chain state. Do not manufacture an authority assumption from possession of a similarly named key file.

## Exact UID census and reset

### What is being reset

Scope is the approved UR mainnet netuid, SN25 (netuid 25), only. Netuid 0 and other subnets are excluded. A UID is a mutable slot, not a permanent miner identity, and a neuron can perform more than one role. “All miners” must become a signed list of **hotkey identities and registration generations**, not a range such as `1..255` or “all UIDs without a validator permit.”

At finalized block `B`, write `census.json` containing every UID and both directions of its UID/hotkey mapping; coldkey ownership; registration block; owner identity; role classification; permits and activity; native and mechanism-specific emission/weights; immune status and expiry; collateral and other locks; stake positions relevant to custody; commitments and associated EVM identity. Include the block hash and runtime identity for every decoded field. Reconcile the complete cardinality against `SubnetworkN`; missing entries or ambiguous ownership block planning.

The signer-free [SN25 census and reset preview](SUBNET-CENSUS.md) now authenticates
one finalized runtime and complete forward/reverse SN25 and root identity maps,
then compares declared protected/removal generations with the inspected owner
trim selection. It does not yet collect collateral, stake, claims, commitments,
EVM associations or every mechanism-specific weight. Even an exact candidate
set keeps `reset_ready=false`: the trim call cannot bind hotkey generations at
execution, and the source-selected owner cooldown is not a metadata constant.
The separate signer-free `owner-trim-plan` command ranks bounded owner
capacities against that authenticated census, predicts removed generations and
survivor UID mapping, and names each old miner that would remain. Its
[algorithm and limits](SUBNET-CENSUS.md#best-effort-owner-trim) retain
`reset_ready=false`, `apply_authority=false` and `full_reset_completed=false`.
The [recheck and reconciliation commands](OWNER-TRIM-GUARD.md) now rebuild a
retained plan from its historical authenticated census, refuse drift before a
prospective call, and compare later exact generations, survivors and root
membership. A matching read is not a transaction receipt or execution token;
the reviewed owner call still accepts only netuid and capacity. Any execution
path must explicitly resolve the protected-identity risk between recheck and
inclusion, record the actual receipt and reconcile effects.

Construct disjoint `remove`, `preserve`, and `unresolved` sets. Preserve explicit owner and validator hotkeys, including a validator currently lacking a permit, and any reserve, pool or escrow identity whose existing custody or earned claims require continuity. Membership in both a requested removal scope and a protected custody/validator role is an explicit conflict requiring a reviewed resolution; it is not silently omitted from “all.” Third-party validator identities receive the same explicit classification. Snapshot netuid-0 membership independently to prove it was untouched.

The owner-key launch target is to remove as many approved old miner registration generations as the runtime safely permits, while preserving every identity in `preserve`. The report must show each requested miner as `removed`, `retained_by_runtime`, or `unresolved`, with a verified old-to-new UID mapping for every survivor. A partial trim is a partial native reset, never a claim that all old UIDs were removed. `unresolved` evidence blocks activation; a known retained old miner needs an explicit launch disposition and must be excluded from new UR scoring and payout admission. If a removed miner later re-registers, record a new generation; never let reuse of the numeric UID satisfy the old identity's postcondition. Registration-open policy and the cutover window must specify whether re-entry is allowed.

UR scoring exclusion cannot erase a retained hotkey's native registration or prevent an independent validator from weighting it. The 10% provider outcome must therefore be checked against the actual post-trim native incentive rows, including every retained old miner. A residual native payout is disclosed as an observed exception; it is not recast as UR provider earnings or a successful full reset.

The majority SN25 validator will run the standard `sn/validator` binary and
its ordinary evidence-based scoring policy. That can help the owner-key reset
only indirectly: old miner generations that are absent from eligible head and
pool evidence receive no positive weight from our validator, which may lower
their observed emissions over future native intervals and make them more likely
to be chosen by the [emission-ranked owner trim][subtensor-uids]. Do not assume
that ownership of the majority seat implies arbitrary zero weights or that an
old miner with valid current evidence will be excluded. The validator cannot deregister
anyone, bypass immunity or minimum capacity, force the other validators' votes,
or grant chain-Root authority. Reobserve finalized emissions and rerun the
complete protected-identity plan before each proposed trim; never assume a
submitted weight row has already changed the chain's trim ordering. The
netuid-0 root validator's weights serve its distinct root basket role, not
SN25 miner deregistration. [Subnet weight setter][subtensor-weights]

Deletion of a registration does not delete historical events, refund registration cost, erase coldkey assets, or extinguish collateral and claims. Historical UR bindings continue to use their original block-specific mapping. Invalidate or renew only future bindings that reference displaced UID generations; preserve proof and claim history.

### Supported paths and their limits

| Mode | What it accomplishes | Condition for selection |
| --- | --- | --- |
| `owner-trim` | Lowers capacity, removes runtime-selected low emitters and compresses surviving UIDs. | This is the preferred owner-key path. A pinned simulation and execution-time guard must prove no protected identity can be removed. Record the exact removals and surviving old miners; an incomplete removal set remains an explicit launch exception, not a full reset. |
| `bounded-replacement` | New registrations replace runtime-selected existing neurons as capacity fills. | A finite, budgeted sequence proves every intended replacement and no protected loss, including competing registrations and changed pruning inputs. If the runtime cannot enforce the approved selection at execution, do not automate the destructive sequence. |
| `new-subnet` | Starts a separate metagraph and contract deployment on a new netuid. | Explicitly selected alternative with its own subnet-registration allowance and migration plan. It leaves the old subnet and its registrations in existence; it is not a reset of that subnet. |
| `chain-root-migration` | Can implement the literal removal policy if the chain's authorized governance adopts a suitable migration. | Separately reviewed runtime/call and authentic governance execution. The bootstrap verifies its finalized result; it never pretends the UR owner can grant itself this authority. |
| `ur-generation-only` | Resets UR application admission/scoring/bindings prospectively. | Explicitly accepted narrower outcome. It makes no claim to remove native UIDs. |

The inspected trim implementation enforces minimum and maximum capacity, protects owner-immune and temporarily immune entries, and requires the immune percentage to remain strictly below its runtime threshold. It removes according to emission rank and migrates the survivors' slot-indexed state. The ordinary `set_max_allowed_uids` path cannot set capacity below the occupied count. [Trim implementation][subtensor-uids], [capacity documentation][max-uids]

At the reviewed runtime, `sudo_set_network_registration_allowed` and the
per-block registration limit require chain `Root`; the SN25 owner cannot assume
it can close registration for a trim window. Even an already closed
registration flag does not prevent a coldkey-authorized hotkey swap from
changing the registered generation. A bounded owner-key execution path must
therefore establish its protection invariant under actual registration and
swap/custody behavior through inclusion, or leave the trim as a read-only
proposal. [Registration setter](https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/admin-utils/src/lib.rs#L728), [hotkey swap](https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/swap/swap_hotkey.rs#L101).

Ordinary registration pruning is a different algorithm. In the inspected source it excludes owner-protected identities but can fall back to temporally immune candidates in some capacity conditions. Do not use the trim immunity model to justify a replacement sequence. `clear_neuron` is an internal implementation routine, not evidence of an owner-callable reset endpoint. [Registration implementation][subtensor-registration]

The reset planner must derive the live minimum, timing restrictions, immunity threshold and pruning order, including ties, rather than use the documentation's nominal 64-slot minimum or 30-day trim interval as constants. Select the most effective admissible owner trim by *safe removal of approved old miners*, not merely by the smallest UID count; keep all protected identities and native custody rights intact. An external actor can change the candidate set between preview and execution. A fresh preflight reduces that race but does not eliminate it; destructive automatic apply requires an execution-time guard or a demonstrated invariant under all allowed intervening changes. Otherwise export the unsupported action and report `RESET_CAPABILITY_BLOCKED`. Owner keys are the only available native administrative authority for this launch; do not plan chain-Root calls or imply that a netuid-0 validator seat supplies them.

A narrower owner-key path may be admissible without an atomic hotkey predicate:
prove that every generation the runtime could remove before the submitted call
expires is an explicitly approved old miner, while every protected identity
remains immune throughout that window. Then changes to emission ordering can
change *which approved old miners* are removed, but cannot remove a protected
identity. The proof must cover the entire mortal transaction window, including
earlier same-block actions: registration, hotkey swaps, temporary-immunity
expiry, owner/immune status, native epoch updates and capacity limits. An
already closed registration flag is insufficient if a subnet-owner takeover
can register a neuron directly during an epoch; rule that path out or choose a
window before the next epoch. First hotkey swaps have no cooldown, so a
cooldown proof needs an authenticated nonzero last-swap value for each relevant
coldkey or a concrete custody fence. Majority-validator weights alone do not
establish any of these conditions. Reconcile actual removals from the
finalized receipt rather than treating the preview's predicted list as the
result. [Epoch owner takeover](https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/coinbase/run_coinbase.rs#L389), [hotkey swap](https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/swap/swap_hotkey.rs#L101).

The signer-free [`owner-trim-qualify`](OWNER-TRIM-BOUNDED.md) now tests a
conservative version of this condition from exact-block RPC evidence: protected
immunity through a proposed 4–256-block mortal era, closed registration,
authenticated hotkey cooldowns and coldkey-swap delays/announcements, no native
epoch or admin-window closure, no lease, and the runtime capacity/immune-ratio
limits. Approved miners may lose immunity without expanding the approved set.
Every requested generation remains explicit, including unresolved originals
and conditional residuals. Passing predicates remain conditional on unproved
owner/proxy/pending-action, governance/runtime and public subnet-pruning/reuse
fences; source-to-Wasm,
custody, actual signed mortality, receipt and subset reconciliation still block
execution. Exit 0 is evidence only; apply/reset/full-reset remain false.

After a finalized reset, repeat the entire census and compare identities, not just counts. Reconcile commitments, balances and locks separately. Re-establish approved capacity and permitted registration settings before new pool/head registrations; no “temporary” parameter change may remain unreported. Existing settlement/claim service must stay available throughout any migration.

## Ten percent of native miner allocation

### Denominator and accounting

Use alpha's atomic units for reward accounting, distinct from TAO rao and EVM wei. The reference denominator is the UR subnet's **native miner tranche before the chosen 90% withholding mechanism**, over explicitly identified native emission intervals after activation. It excludes the owner cut, validator/root dividends, TAO pool injection, deposits, reserve principal, and collateral principal. A miner's newly earned reward captured into collateral still counts as that miner's economic reward; it is not a way to hide payments from the 10% calculation.

Let `M_i` be the authenticated miner allocation for native interval `i`, using the exact runtime's accumulation, mechanism split, drain and rounding rules. Let `M_total(k) = sum(M_i, i <= k)`. Define the integer reference for the cumulative provider target:

```text
P_reference(k) = floor(M_total(k) / 10)
P_interval(k) = P_reference(k) - P_reference(k - 1)
remainder_reference(k) = M_total(k) - P_reference(k)
```

This conserves reference rounding across intervals; independent floating-point `0.1 * amount` calculations are prohibited. Use checked integer/rational arithmetic. Record `Q(k)`, the absolute alpha-unit tolerance derived from the selected runtime's actual u16, fixed-point and per-recipient rounding over the observation window. The requested target is met when actual provider entitlement differs from `P_reference(k)` by no more than that explicitly reviewed tolerance. Do not invent a large percentage allowance for consensus disagreement or require zero dust when the chain cannot represent the fraction exactly. Track native truncation dust separately from the policy remainder. Allocation at a later UR settlement epoch must reference its underlying native intervals exactly once. Align activation to a proven drain boundary with no unexplained pre-activation `PendingServerEmission`; never relabel old rewards as a new 10% budget.

Under the nominal 18% owner / 41% miner / 41% validator split, this request corresponds to approximately **4.1% of participant-side subnet emission**. The actual owner-cut fraction and fixed-point rounding must be read and reproduced, not hardcoded as exactly 41%. The inspected implementation accumulates a miner half after the owner cut, and zero incentive can redirect the miner allocation to validators. [Participant distribution implementation][subtensor-coinbase]

The existing UR `theta` divides provider distribution between direct head miners and tail pools. Once the 10% mechanism is selected, construct reference allocations `H = floor(theta * P_reference)` and `T = P_reference - H`, then reconcile their actual native outcomes against the approved quantization tolerance. A separately selected hard-cap mode additionally requires actual cumulative entitlement not to exceed its signed ceiling. Changing theta alone leaves the native miner pot intact; row normalization also makes multiplying every validator weight by 0.1 ineffective. [UR steering specification](../WHITEPAPER.md), [existing steering tests](../validator/steer_test.go)

Count entitlement once: native rewards to provider-owned head coldkeys, and provider entitlement funded through tail pools, are the two payment channels. A tail capture followed by a claim is one reward, not two. Target measurement, and any separately selected cap, covers earned entitlement and locked miner reward, not just liquid claims completed so far. Existing valid claims retain their original terms.

### Selected owner-recycle policy for the other 90%

| Policy | Feasibility and consequences | Design disposition |
| --- | --- | --- |
| `owner-recycle` | If final Yuma incentive is directed to the runtime-recognized owner-hotkey set under recycle mode, that portion is recycled. It is not placed in our reserve or retained as a future provider claim. | **Selected by the user, 2026-09-27.** Implement an explicit mainnet policy successor and prove the actual 10% provider / 90% recycle outcome within the approved runtime tolerance. |
| `owner-burn` | The same owner-directed withholding path can burn instead. Burn/recycle supply effects differ. | Not selected; verify the actual on-chain mode is recycle. |
| `reserve-custody` | Routes the remainder into an explicitly defined reserve position, with separately enforced accounting and no provider claim on it. | Not selected. The current vault has no “send 90% to reserve” operation; owner-recycle provides no reserve credit. |
| `deferred-provider-liability` | Pays 10% now while preserving 90% as future provider claims. | Not selected; recycling does not create a deferred provider liability. |
| `native-runtime-cap` | A runtime-enforced miner sub-allocation could provide the strongest whole-subnet boundary. | Not the selected mechanism. No suitable owner-callable primitive was established in the inspected source. |

The existing implementation withholds owner-directed incentive in both burn and recycle modes and records the withheld ratio. Choosing recycle does not avoid that accounting. [Owner-directed distribution][subtensor-coinbase] In the pinned source, recycling decrements `SubnetAlphaOut` and calls the alpha-asset recycle operation; burning calls its distinct burn operation without that decrement. Neither operation credits the owner wallet or our vault. [Alpha accounting][subtensor-alpha-accounting] The withheld ratio reduces the subnet's demand share before the emission gate; a 90% withholding policy is economically consequential and does not imply an exact 90% reduction in final TAO allocation after renormalization and gating. [Allocation formula][subtensor-shares]

The inspected runtime defaults `RecycleOrBurn` to **Burn**. Its `AdminUtils.sudo_set_recycle_or_burn(netuid, Recycle)` call accepts the subnet owner or chain root, subject to the runtime's owner rate limit and admin window; the EVM alpha precompile exposes `setRecycleOrBurn(uint16,uint8)` with mode `1` for recycle. These are alternatives for one reviewed operation, not two changes to submit. The bootstrap must read the exact runtime metadata and current finalized storage, select the authorized route, set recycle if needed, and verify the finalized `RecycleOrBurn[25] == Recycle` value before activating any owner-directed 90% weight proposal. A pending call, a failed call, or the default Burn value is not recycle. Recheck the mode at every activation/recovery boundary and alert on drift. [Admin setter][subtensor-admin], [default storage][subtensor-storage], [EVM precompile][subtensor-alpha-precompile]

Release 1.0 explicitly rejected owner-directed burning as its head/tail steering strategy. The selected owner-recycle launch policy must therefore be encoded as an explicit economic-policy successor, with its activation and accounting independently verified. Preserve the independent-validator objective and signed weight caps: do not raise a cap, create arbitrary owner recipients, or displace validators merely to force a 90% weight destination. [Whitepaper, head/tail decision](../WHITEPAPER.md#138-headtail-split-θ-in-one-mechanism-chosen-not-two-mechanisms-not-owner-burn)

A weight proposal is not an enforceable payout fraction. Independent validator weights, Yuma clipping, bonds, activity, permits, normalization and u16 rounding affect final incentive. For either owner-withholding path, qualify the complete runtime outcome against the admitted validator set and review adjacent/adversarial weight states. The draft assurance mode is `observed-native-target`: demonstrate the actual 10% allocation within `Q(k)`, disclose sensitivity to other validators, and monitor subsequent deviation. It does not promise that other validators can never change the outcome. If a stronger `enforced-cap` mode is selected, prove that ceiling under all admitted conditions or report `EMISSION_CAP_UNENFORCEABLE`; an after-the-fact monitor is not enforcement. Halting our validator does not revoke other validators' weights or stop already queued native emission. [Consensus implementation][subtensor-epoch]

Claim “started at 10%” only after the observed native outcomes meet the target and its runtime-derived tolerance. Report deviations and dust explicitly; a low payout below a hard ceiling alone does not establish that the target was reached. If only a proposed weight target can be shown and the actual outcome cannot yet be established, report `ACTIVATED_AWAITING_EMISSION_OBSERVATION`. A material miss is not relabeled as quantization. Future changes to the target or assurance level require a new reviewed policy.

### Immutable vault implications

[STSettlementVault](../evm/src/STSettlementVault.sol) captures the eligible pool emission through the configured staking precompile and keeps immutable claim accounting. It has no upgrade or treasury sweep. Its conservation checks distinguish total captured, total paid, escrow accounting, pending funding and outstanding liabilities. Publishing payout roots at 10% while leaving 90% in this vault does not make the remainder an owner reserve or erase its accounting obligations.

The [claim-recovery correction](../evm/CLAIM-RECOVERY.md) keeps an accepted
Merkle leaf and provider credit if an exact runtime transfer or balance-delta
check fails. The attempted payment is isolated in a vault self-call so its
state can roll back without rolling back claim acceptance. This changes the
non-upgradeable vault's bytecode and requires a new deployment artifact and
an authenticated runtime rehearsal of nested EVM/native rollback. It does not
repair an already deployed vault or loosen exact payout accounting.

A reserve design must specify a different enforceable custody path before deployment: who owns each head and pool registration, which precompile call transfers each tranche, which principal and collateral are excluded, who can authorize the transfer, and how cumulative provider liabilities are bounded. Moving all head miners behind a new contract would change release 1.0's provider-owned direct-payment model. Such a design requires its own contract/policy qualification and migration plan; a coordinator upgrade cannot retrofit withdrawal authority into an old vault.

Implement the selected owner-recycle path against the exact runtime: authenticate the recognized owner-hotkey set and recycle mode, preserve the signed cap and validator independence, and prove the finalized 10% provider / 90% recycle split with exact interval accounting. Keep the recycle fraction separate from native rounding and supply-side effects. No reserve transfer or new reserve-custody contract is implied. Do not deploy an immutable contract set before the selected path and custody layout are qualified.

The [signer-free successor preview](../validator/OWNER-RECYCLE-PLANNER.md)
(`9a287d92`) is an implementation step, not activation. It binds a proposed
successor to the unchanged parent policy, builds the provider tenth using the
existing head/tail theta, gives the remainder equally to usable recognized
owner UIDs, and refuses rows that violate the signed cap before or after u16
quantization. With the current `32768/65535` per-recipient cap, a single
owner destination cannot carry 90%; at least two usable registered owner
hotkeys are necessary. The pinned source permits multiple hotkeys of the
subnet-owner coldkey and recognizes their incentive for recycling, but their
mainnet registration, masks and protection are unknown. The preview always
refuses submission and reports its row as a proposed weight fraction; an
authenticated owner census, signed policy transition and measured native
Yuma/emission outcome remain separate gates.

The [signed successor admission](../validator/OWNER-RECYCLE-ADMISSION.md)
now verifies an independently pinned approval, immutable local custody and an
exact finalized runtime/owner census under explicit Recycle mode. It does not
start steering: both submission paths remain fenced until measurement,
signatures, pending intents, archive replay, coordinator/client-key policy and
the native drain boundary use one successor authority. Original pending
receipts can still be reconciled. An admitted 10/90 weight row remains a
proposal until independent validators and finalized native allocation prove
the economic result.

Settlement admission also requires the compatible server custody reader and
migrations through 728. The [retained timestamp correction](PRELAUNCH-FIXES.md)
prevents historical terminal NULL close times from disappearing out of every
epoch: it blocks new payout construction before chain reads and retains the
same debt after archive cleanup. Record the exact historical NULL census before
coordinated reader/writer/reaper cutover. Unknown timestamps are unresolved
debt, never guessed epoch assignments or automatic zero-credit exclusions.
Previously retained artifacts stay immutable and are not certified complete by
this prospective fix. NetEscrow writer drain, revision/fence restore policy and
archive capacity remain separate production gates.

## Contract deployment, custody and initialization

Reuse the release-1.0 contracts and reviewed ABI/artifact generation, with mainnet-specific inputs. The existing [Deploy script](../evm/script/Deploy.s.sol) is the ordering reference, not a command the bootstrap blindly shells out to. The Go planner must build exact transaction payloads and independently read back their results.

| Identity | Custody/authority |
| --- | --- |
| Subnet owner coldkey | Native subnet administration; offline or explicitly qualified native multisig/proxy. |
| EVM deployer | Limited bootstrap gas/value; no ongoing governance custody. |
| Coordinator owner | Actual 2-of-3 Safe with three distinct approved owners. |
| Guardian | Separate limited operational authority. |
| Commitment oracle | Separate reviewed signer/service with original and any scheduled route authenticated. |
| Root validator coldkey/hotkey | Root stake custody and root service signing; separate from UR scoring by default. |
| UR validator hotkey and stake coldkey | UR scoring; may be the reviewed reserve target when explicitly selected. |
| Vault mapped coldkey | Immutable tail-pool and escrow custody. No human holds its private key. |
| Reserve mapped coldkey | Permanent reserve stake under the immutable sink. |

Role equivalence must be deliberate. In particular, naming a UR validator “owner validator” does not prove that it is the native `SubnetOwnerHotkey`. The planner checks the actual mapping and does not assume UID 0. Require the existing deployer's distinct owner/guardian/oracle constraints. Inspect Safe singleton bytecode, owners, threshold, enabled modules, guards, fallback handler and pending transactions; an address merely implementing `getOwners` and `getThreshold` is insufficient.

Freeze compiler and dependency versions, creation and deployed bytecode,
link/immutable locations, constructor encodings, source identities and storage
layout. The [current size check](evidence/contract-size-candidate-20260927.md)
puts `STCoordinator` at 24,564 runtime bytes, just **12 bytes** below Foundry's
24,576-byte limit; any source or build-input change requires a fresh size and
exact-artifact deployment rehearsal. The selected live runtime's code-size rule
still needs authentication. For a dedicated deployer starting at nonce `n`, the
existing core sequence is:

| Nonce | Action | Required postcondition before its dependants |
| --- | --- | --- |
| `n` | CREATE `STReserveSink` | Exact predicted address, bytecode, netuid, reserve hotkey, bootstrap, mapped coldkey. |
| `n+1` | CREATE `STSettlementVault` | Exact custody identities, claim horizon, minimum transfer and bootstrap. |
| `n+2` | CREATE `STCoordinator` implementation | Exact implementation bytecode; implementation initializer disabled. |
| `n+3` | `vault.registerEscrow(maxBurnRao)` | Escrow registration under the vault's mapped coldkey, correct UID/ownership and bounded debit/refund. |
| `n+4` | CREATE ERC1967 proxy with initialization calldata | Initialization occurs in the constructor; approved Safe, guardian, oracle, custody links and initial policy are set atomically. |
| `n+5` | `reserve.setRecorderOnce(proxy)` | Exact one-shot recorder. |
| `n+6` | `vault.setCoordinatorOnce(proxy)` | Exact one-shot coordinator. |

The escrow registration deliberately consumes a nonce before proxy creation. Predict all addresses before construction because the mapped coldkeys are immutable. Native rao-to-EVM-value conversion uses `1 rao = 10^9 wei` here; bounds and conversions must reject overflow. Authenticate `blake2_256("evm:" || H160)` against the runtime mapping before custody is funded.

Deploy and anchor [STValidatorEvidence](../evm/src/STValidatorEvidence.sol) as a separately planned additive contract with its genesis/deployment domain and coordinator/vault identities. The current release uses the coordinator's one-shot `fixValidatorEvidence`; an existing foreign anchor is a hard conflict. Use the fresh mainnet nonce graph, not the sim-testnet graph's extra upgrade, fleet-helper or adversarial contracts. Mainnet artifacts must not include those test fixtures by default. [Evidence deployment reference](../sim-testnet/evidence_deployment.go), [readback reference](../sim-testnet/evidence_deployment_runtime.go)

Only then register the approved operator pool hotkeys under vault custody, establish reserve-target eligibility, activate evidence identities and future bindings, and fund reviewed stake/deposit positions. Provider-owned head miners register through their own authorized identities; bootstrap cannot sign for unrelated miners. Every registration is present in the spend/count plan. Reconcile pool/escrow collateral and minimum-transfer semantics before the first production capture; immutable custody must not become stranded by an unqualified runtime change.

Use the actual [mainnet policy validation](../protocol/policy.go): a UR settlement epoch is **50,400 native blocks**, with the reviewed production root-commit/finalization/close windows and claim retention. The deploy script's mainnet reference windows are 1,200 / 14,400 / 120 blocks and 8 claim epochs plus 1 grace epoch. Encode all fields explicitly in the signed mainnet policy; do not inherit accelerated 300- or 360-block testnet settings. Mainnet economic caps, deposit tiers, theta, minimum operator/validator counts and binding horizons need independent review.

Preserve the current guarantees: the coordinator owns neither custody position, the sink has no outbound path, and valid earned vault claims survive coordinator pause or upgrade. Pausing new application activity is not a native emission kill switch. Initial contracts establish their epoch clock at deployment, so the plan must include sufficient time to finish setup and a future activation boundary; it cannot assume a dormant deployment has no running clock.

## Running both validators

### Root validator on netuid 0

The signer-free `root-preview` and bounded `root-monitor` commands now supply a
separate [root observation policy and service seam](ROOT-VALIDATOR.md). They bind
independently approved mainnet genesis/full runtime/code/metadata to an exact
finalized seat, stake, delegation and strategy census; ID 945 is rejected before
root storage. Their `ready` status means only read-only observation policy
readiness, and `activation_ready` remains false. Registration, bounded signing,
effective custom-weight eligibility and complete basket custody remain gates.
The [existing-seat action owner](ROOT-ACTION.md) now provides an offline-qualified
mortal root basket encoder, durable one-request signing/nonce ownership and
receipt/expiry recovery. Its read-only chain adapter reconstructs canonical
native inclusion and receipt evidence from the approved owned RPC, with exact
historical execution-runtime checks; it does not independently prove GRANDPA
finality or storage. Production signing authority, global custody and submission
are absent; there is no signing command or active root service. A signed root
call does not bind registration generation, so pending-action seat changes need
custody exclusion or separately authenticated incident reconciliation. The
accumulate-in-place strategy needs no heartbeat transaction. Changing that
strategy, signing fees and distributed custody fencing require separate approval
and qualification; a local reserve is not a native maximum-fee argument.
The current [UR validator config](../validator/config.go) rejects netuid 0 and is
not a root-validator implementation.

For an existing root seat, verify hotkey/coldkey ownership, current membership and registration generation, stake, immunity, delegate take, children/parents, basket configuration and accrued rights before adoption. For a new seat, the inspected runtime uses burn-priced root registration without a prior-stake admission condition; a full root network prunes a lowest-staked eligible seat. Registration alone does not provide enough stake to retain a seat or submit basket weights. [Root registration implementation][subtensor-root]

One specific budget gap must not be hidden: native `root_register(hotkey)` has no maximum-burn argument, while `register_limit` rejects netuid 0. A fresh quote is not an atomic price ceiling. The inspected Neuron precompile also exposes `rootRegister(bytes32)` without a limit. [Native call definitions][subtensor-dispatches], [registration limits][subtensor-registration], [Neuron interface][neuron-interface]

The future adapter therefore needs one of these explicit admission paths:

- Adopt an already registered, approved root identity and prove its finalized receipt and current ownership.
- Qualify a bounded root registration mechanism. A possible EVM path is an approved root custody account that invokes `rootRegister` and atomically rejects a mapped-balance debit above the signed ceiling. This changes the coldkey custody model and requires proof of EVM/native rollback, fee separation, hotkey ownership, staking/withdrawal authority and child-delegation controls; no such helper is supplied by this design or the existing UR contracts.
- Record a separately authorized native root registration as an external prerequisite with its actual receipt and spend. Do not label an uncapped native call “max-burn protected” or silently replace its intended coldkey with a contract mirror.

Without a proven bounded path or retained seat, full automatic bootstrap is `ROOT_REGISTRATION_CAPABILITY_BLOCKED`. This is a concrete implementation requirement, not permission to omit the requested root validator.

Root registration can automatically delegate child weight to every existing subnet owner unless the identity opts out first. The proposed root policy explicitly disables automatic parent delegation before registration, then permits only a separately selected UR delegation if required. For a fresh hotkey, first establish its approved coldkey association through the supported `try_associate_hotkey` path: the opt-out call already requires that ownership. Verify the resulting child/parent maps, including pending changes. Existing roots with custom weights require an explicit preservation/reset decision; omitting a new write must not be mistaken for clearing old weights. [Root delegation dispatch notes][subtensor-dispatches]

Default the draft basket policy to **accumulate in place with no custom weight vector**. This is a real operating strategy, not simulated validation or a claim to control subnet issuance. Setting custom root weights turns on basket allocation behavior and must be separately plan-bound. The inspected call checks root membership, minimum stake, enablement, timing, distinct existing destinations, diversity and concentration. Read their live values; do not hardcode a 64-seat network, an 8-destination floor or a 1/16 cap. [Root weight implementation][subtensor-weights], [basket behavior][root-reborn]

Stake the root seat from its own explicit TAO allowance, retain fees/ED, and observe the actual retention margin. Stake or basket top-ups, re-registration, claims, take changes and weight changes are bounded planned actions, not an unlimited watchdog loop. The service monitors finality, seat ownership, stake rank, delegation, basket state and runtime identity. Claiming root yield and unstaking principal are distinct operations with runtime-dependent windows; neither is enabled automatically by “run a root validator.”

### UR subnet validator

Run the production [validator entry point](../cli/validator/main.go), using complete deployment, policy, runtime, genesis, operator API, evidence and signing inputs. It must acquire current UR eligibility and validate finalized usage/evidence before emitting native CRv4 weights. Preserve the signed weight cap, head/tail rules, deposit/quality calculation and full prefix/history admission. Reject testnet provisional-input deferrals on mainnet.

Observe effective alpha/root-stake contribution, child attribution, `TaoWeight`, stake threshold, permits, activity, CRv4 version, reveal schedule and mechanism state. Do not transplant the testnet stake target or an old 0.18/0.018 TAO multiplier. The inspected production epoch path gives the owner UID special eligibility treatment; another owned hotkey still needs its own proper eligibility. A configured process being alive does not prove it has a permit or that its weight row was revealed and applied. [Permit calculation][subtensor-epoch]

Both requested validators do **not** count as two UR validators: a seat on netuid 0 alone does not validate the UR subnet. The current UR safety policy requires at least two live validators and two healthy operators. Include a separately admitted second UR validator before production readiness. Do not generate synthetic peers to satisfy the count.

Capacity is computed from the union of actual UR hotkeys: head miners, one pool per operator, distinct UR validator identities, escrow and owner/other protected identities. Root-only membership consumes no UR slot. A validator-permit limit is not a reserved partition of UID space. Keep the release's one-mechanism requirement and approximately 200-head target only if the live capacity and all additional identities fit. Do not assume “200 miners + 56 validators” leaves space for pools and escrow.

Manage both services with independent state directories, signer permissions, logs, executable/config hashes, and one writer per hotkey. Coldkeys and Safe owners stay out of online validator processes. Supervision has bounded restart policy; ownership transfers require the old process and every child to be joined. Read-only health includes finalized lag, evidence/index gaps, permit activity, revealed weight rows, pool/escrow capture, signed cap violations and root seat/basket drift.

## Go CLI and action model

[mainnet/main.go](main.go) currently contains signer-free `inspect`, `monitor`,
`subnet-preview`, `root-preview`, `root-monitor`, `check-recycle-mode` and `economic-reference`
commands, plus the offline `source-lock`, `release-inventory` and blocked-review `plan` commands. `inspect --rpc URL`
emits a content-hashed identity snapshot. Supplying
any expectation requires all of `--expected-chain`, `--expected-genesis` and
`--expected-evm-chain-id`; `monitor` always requires all three. The monitor emits
JSON lines for `ok`, `rpc-error`, `rpc-integrity`, `identity-mismatch`,
`finality-conflict`, `finality-stalled` or `checkpoint-error`, with a default 30-second interval after
each completed sample and five-minute stall threshold. It detects identity/finality conflicts,
rechecks the prior finalized block when the head advances, validates JSON-RPC
response version/ID, and rejects response bodies exceeding 1 MiB. Hash comparisons
accept equivalent hexadecimal casing. Malformed, inconsistent or oversized RPC
evidence emits terminal `rpc-integrity`; that status, `identity-mismatch` and
`finality-conflict` exit with code 3. Availability failures remain `rpc-error`
observations and do not establish healthy state. Repeated read failures retain
their first observed time; `severity` becomes `warning` after two minutes and
`critical` after five, while the monitor keeps retrying. A host-clock rollback
escalates immediately rather than postponing the page threshold. The command
contains no signer or submitter. Focused normal/race tests and vet pass; the retained Snow rejection
demonstrates actual wrong-network refusal.

`monitor --checkpoint /absolute/path/monitor.json` adds a single-owner local
continuity checkpoint. Before reporting a newly finalized position as healthy,
it atomically persists the approved chain/genesis/EVM identity, last finalized height and hash, and
progress time with a content checksum. After a first healthy finalized sample,
the checkpoint also retains the start of a read outage and clears it only after
a complete identity and continuity read. An outage before the first healthy
sample has no finalized position to checkpoint and needs the independent
monitor dead-man alert. The v2 checkpoint reader accepts a valid prior v1
finality record and writes v2 on its next state change; a rollback to the old
binary requires an explicit compatible checkpoint migration, not silent file
replacement. Restart loads the retained position and checks
the prior finalized hash against the route; a regression or changed historical
hash is still visible after process restart. A corrupt, foreign or symlinked
checkpoint stops admission; an unavailable write emits `checkpoint-error` and
exits rather than reporting health. The file is local continuity evidence, not
independent node confirmation or an approval. The operator must place it on a
durable, backed-up volume and supervise the monitor; alert delivery and
cross-domain health remain open work.

An unchanged retained head with a checkpoint progress time ahead of the host
clock reports stalled until genuine finalized advancement resets the clock.
`root-preview` and finite `root-monitor` perform the separate [read-only root
census](ROOT-VALIDATOR.md). Their `ready` result means observation policy
readiness only; `activation_ready` is always false. They load no signer and do
not register, stake, submit root weights or authorize basket claims.
`subnet-preview` performs the [read-only SN25 reset feasibility
census](SUBNET-CENSUS.md), with explicit protected and removal generations;
`reset_ready` is always false and no UID is changed.
`owner-trim-plan` adds a bounded ranked partial-trim prediction and explicit
residuals, without changing that admission result.
`owner-trim-recheck` and `owner-trim-reconcile` extend the read-only evidence
through drift detection and exact post-state comparison, with the same blocked
execution status and no signer.
`owner-trim-qualify` separately evaluates a bounded approved-subset invariant;
its conditional result retains unproved window assumptions and no apply authority.

The [existing-seat root action owner](ROOT-ACTION.md) is an offline-qualified
one-action signing and recovery core, not a CLI command or live root validator.
It retains the original signed bytes, nonce and fee reservation across ambiguous
submissions. Production authority, custody, canonical receipt and supervisor
adapters must be supplied and qualified before it can publish a root basket;
the current accumulate-in-place strategy needs no periodic root transaction.

`check-recycle-mode --rpc URL --policy FILE` binds the finalized mode read to
independently supplied mainnet genesis, runtime code/metadata and complete
version pins. It validates the runtime-declared map, enum and Burn fallback and
reports whether the finalized value is Recycle. `economic-reference --input FILE`
computes cumulative integer 10% provider / 90% recycle references from
caller-supplied native miner tranches, carrying rounding between intervals.
Both retain `activation_ready=false`: the first proves only mode storage under
the approved artifact, and the second does not authenticate interval inputs or
actual Yuma payouts. [Command contract and remaining gates](ECONOMIC-GATE.md)
cover the source-to-code, owner-hotkey, allocation and observation work. Focused
normal/race tests and vet pass after the 2026-09-27 data-volume recovery.

`inspect`/`monitor` remain single-route identity/finality observers. The
separate `runtime-snapshot` captures raw finalized code/metadata without
granting authority. `inspect` and `monitor` validate the complete runtime tuple
internally, but their v1 JSON reports only spec/transaction numbers; emit a
versioned full-tuple observer artifact before using them as independent runtime
identity evidence. Unknown digest variants require reviewed decoding support.
These commands do not map EVM receipt finality, compare independent nodes, inspect SN25
custody/validator/settlement state, deliver alerts,
or execute repairs. Those are MG-07 and related production gates. An identity
snapshot hash proves the captured bytes, not operator approval or node truth.

The [pure plan foundation](PLAN.md) consumes one `finalized-snapshot`, a source
lock and release inputs by exact hashes. `plan --outline` exposes the unbound
dependency graph while approved mainnet identity is unavailable;
`plan --config FILE` accepts the separate strict JSON review schema and refuses
testnet EVM945 or an unexpected genesis. Both modes keep every action blocked,
with no apply authority. Supplied review manifests remain unvalidated until
their actual semantic/capability/custody checks are implemented.

Keep the executable plan builder pure after authenticated snapshot inputs are supplied.
Separate chain adapters, signer interfaces, state storage and supervisors so
preview cannot reach a transaction submission path.

Target command surface; `inspect`, `runtime-snapshot`, `finalized-mapping`, `finalized-snapshot`, `monitor`, `subnet-preview`, the two root observers and the two
limited economic preconditions above, `source-lock`, `release-inventory` and the blocked-review `plan` foundation exist, while the remaining commands are
designs:

| Command | Behavior |
| --- | --- |
| `inspect` | Extend the existing read-only identity capture with authority, census, capabilities, balances, custody and validators; emit a hashed snapshot. |
| `runtime-snapshot` | Existing signer-free capture of one finalized runtime's exact `:code` and metadata bytes, complete version and node identity; independent approval and source-to-Wasm review remain separate. |
| `finalized-mapping` | Existing signer-free capture of linked finalized native and raw EVM header commitments with an owned-RPC canonicality assertion; no signing or mainnet approval. |
| `finalized-snapshot` | Existing signer-free same-block capture of runtime code/metadata and native/EVM mapping; its output remains unapproved observation. |
| `monitor` | Extend the existing read-only identity/finality loop with durable checkpoints, independent comparisons, complete domain health and existing-stack alert delivery. |
| `subnet-preview` | Existing signer-free finalized SN25/root UID census and owner-trim candidate comparison; full custody and execution-time reset authority remain open. |
| `root-preview` / `root-monitor` | Existing signer-free finalized root seat and strategy census; an offline [existing-seat action core](ROOT-ACTION.md) exists, but production signing and activation remain separate work. |
| `check-recycle-mode` | Existing signer-free finalized storage-mode precondition; extend with an approved mainnet artifact and operational readback at activation/recovery. |
| `economic-reference` | Existing signer-free cumulative integer 10%/90% reference from caller-supplied native intervals; actual chain reconciliation remains a separate gate. |
| `source-lock` | Existing offline lock of clean SN and every local Go replacement Git commit, module checksums, Go version and tool hash. It binds source inputs only; artifacts, rollout approval and qualification remain separate. |
| `release-inventory` | Existing local candidate inventory of exact executable/contract/config/policy/migration/image/dependency/toolchain files, bound to the rechecked source lock; missing categories remain explicit, and release completeness/provenance/deployment approval stay false. |
| `plan` | Existing pure blocked-review graph via `--outline` or strict JSON `--config FILE`; hashes exact finalized-snapshot/source-lock/release inputs. Every action remains non-executable. Full semantic admission, payloads and executable authorization remain future work. |
| `apply --accept-plan HASH` | Execute only the exactly reviewed plan with matching signed authorization, prerequisites and ceilings. |
| `status` / `verify` | Read-only journal reconciliation and current/finalized postcondition verification. |
| `resume --accept-plan HASH` | Recover in-flight actions, verify retained receipts and continue the same approved graph without duplicate spend. |
| `services start` / `services stop` | Run or join the plan's admitted services; starting write-capable validators is an explicit authorized phase. |
| `report` | Produce a complete acceptance or incomplete/blocked report with evidence references and realized spend. |
| `repair plan` / `repair apply --accept-plan HASH` | Produce and execute only the exact approved, bounded repair graph; share the existing durable transaction owner and lifetime ledger. Not implemented. |

`source-lock --sn-dir /absolute/sn/path` emits a content-hashed JSON record of
the clean SN Git HEAD and every local `go.mod` replacement's clean Git HEAD,
the exact `go.mod`/`go.sum` hashes, current Go version and command-binary hash.
It refuses modified or untracked repository files, ignored replacement module
files and active `go.work` overrides; it rechecks each HEAD after hashing. It
reads no RPC and holds no signer. This is one input to the release
manifest, not an approval or a claim that compiled binaries, Foundry bytecode,
generated files, ignored files, configuration, migrations or the running images
match those commits. The release owner must bind those artifacts separately,
qualify the composed source and approve the resulting immutable manifest.

`release-inventory --config FILE` provides the [actual-file candidate
inventory](RELEASE-INVENTORY.md). It rechecks source closure and selected file
bytes, refusing stale per-artifact source bindings. It enumerates missing
categories but does not infer complete coverage from one file per category,
prove a build or image, inspect a deployed migration state, or approve a release.
Ignored Solidity libraries and compiler identities are explicit dependency and
toolchain inputs; they are not included by the Go source lock alone.

The [current composed local candidate](evidence/release-candidate-v11-20260927.md)
locks SN `265231f9`, server `77cb401e` and Connect `c68689c4` with all local
Go replacements. Its [partial actual-file inventory](evidence/release-inventory-candidate-v11-20260927.json)
hashes 85 selected files, including seven locally built executables, all eight
server Dockerfiles, all seven image-build Makefiles, the exact package lock,
40 Ubuntu payloads, six signed-index inputs and four copied contract artifacts.
It has no approved policy or
published/deployed OCI image identity and remains unapproved.
Mainnet inventory implementation passed 177 normal tests and all 177 race
test bodies in a bounded run plus exact continuation; the latter is not one
whole-package race pass. A subsequent validator test-fixture correction passed
87 affected tests normally and under race without changing production seed
custody. The [unbound blocked outline](evidence/blocked-plan-outline-20260927.json)
names ten non-executable actions and 24 missing requirements. Its negative
control rejects retained Snow testnet EVM ID 945; the earlier
[same-block Snow observation](evidence/source-lock-finalized-snapshot-20260927.md)
authenticates code, metadata and linked native/EVM headers but remains
unapproved. The earlier [composition](evidence/source-lock-composed-20260927.md)
separately qualified unchanged validator and receipt selectors. This is
offline qualification of those source paths, not a complete release or an
approved mainnet configuration. The prior
[v7 source and contract build](evidence/source-lock-blocked-plan-20260927.md)
remain linked evidence; subsequent evidence-only commits do not alter the
frozen earlier candidates' Git identities.

The earlier v8 binaries use a local exploratory Go profile. An offline
[production-style probe](/mnt/data/sn-testnet/evidence/mainnet-source-lock-v8-20260927/production-profile/RESULT.md)
also builds static, trimmed, version-stamped Linux/amd64 SN executables, but
they are not in the v8 inventory or an approved image. The pinned server API
and taskworker likewise build and repeat exactly from clean source; their
[build record](/mnt/data/sn-testnet/evidence/mainnet-server-binaries-20260927/RESULT.md)
is retained. Server commit `969d6c74` first pinned the six service Dockerfile
bases that still used a mutable Ubuntu tag. The subsequent [v9 image
probe](/mnt/data/sn-testnet/evidence/mainnet-images-v9-20260927/RESULT.md)
found that `apt-get` still read moving Ubuntu repositories. Server commit
`a211d56c` pins complete package payloads for seven service Dockerfiles,
including proxy's `curl` closure, and installs them offline. The
[package qualification](/mnt/data/sn-testnet/evidence/mainnet-server-package-pins-20260927/RESULT.md)
authenticates signed indexes and 40 payloads; three normal/race contract tests
and vet passed. Independent [amd64 API/proxy image
probes](/mnt/data/sn-testnet/evidence/mainnet-package-pin-probe-20260927/RESULT.md)
built those recipes and extracted exact selected binaries. The
[v10 build and inventory record](/mnt/data/sn-testnet/evidence/mainnet-source-lock-v10-20260927/RESULT.md)
binds the clean composition and replays its 81-file inventory byte-for-byte.
Choose approved production versions and architectures; archive exact package
and base inputs; qualify arm64 image execution, full service behavior,
source-to-image provenance, selected published OCI manifests and running-image
readback before MG-02 can close. Local binaries and image IDs are not an
approved rollout.
An [uncached API rebuild](/mnt/data/sn-testnet/evidence/mainnet-package-pin-probe-20260927/RESULT.md#forced-rebuild-result)
produced a different OCI digest from identical pinned inputs; package logs,
cache and timestamps varied. The release build must either normalize those
outputs and prove exact repeatability, or identify an independently reviewed
immutable image without claiming reproducible bytes.
Server `77cb401e` removes only the two volatile generated files and gives all
seven image recipes a fixed source epoch and timestamp-rewriting exporter.
The [no-cache OCI qualification](/mnt/data/sn-testnet/evidence/mainnet-server-image-repro-20260927/RESULT.md)
repeated API and proxy runnable amd64 platform manifests, configs and layers
exactly. Its top-level indexes remained distinct because provenance described
different invocations. The source-level contract tests passed normal/race,
with offline image smoke. A separate [v11 taskworker image check](/mnt/data/sn-testnet/evidence/mainnet-server-taskworker-image-20260927/RESULT.md)
repeated its exact runnable amd64 platform image twice and verified the embedded
candidate binary in an offline container. Keep arm64, remaining service images, independent builder,
full attestation/SBOM/scanner policy, owned archive, registry publication and
deployed readback open.

The [owner-recycle measured decision](../validator/OWNER-RECYCLE-MEASUREMENT.md)
now joins signed successor approval, exact native owner census and fully
replayed original V2 provider proofs in a distinct capsule. It reconstructs
the proposed 10% provider / 90% owner weight row, but emits only a blocked
unsigned intent. It cannot enter the existing native signing/submission path;
validator eligibility, operator health, complete history, drained activation,
custody, archive and observed final incentives remain MG-06 gates. Its affected
139-test race selector and final-source 58-test focused normal selector passed;
the [qualification record](/mnt/data/sn-testnet/evidence/mainnet-owner-recycle-decision-20260927/RESULT.md)
states the exact limits. The next operator-observed variant invokes the actual
canonical coordinator reader after full proof replay: it checks active registry,
pool and provider mappings, exact deposit/conviction amounts, policy and source
root/window against the measurement, then rechecks chain/genesis and the exact
EVM decision hash. Its distinct v2 capsule binds those retained facts to the
original provider bytes; v1 capsules remain byte-compatible. This resolves
decision-time coordinator claims without treating active registration as API
health or independently proving the native/EVM mapping. Native validator
eligibility, API/key/payout history and all activation/signing gates stay open.
Its [qualification record](/mnt/data/sn-testnet/evidence/mainnet-owner-recycle-readiness-20260927/RESULT.md)
retains 138 normal and 138 race passes, vet, cross-compile and the final
naming-only follow-up without claiming launch approval.

No implicit apply, automatic subnet creation, private-key CLI flags, “force” bypass, mutable `latest` artifact, or inherited network defaults. Every future mutating command takes an explicit run directory and accepted plan hash. Read-only discovery may run while identity or other gates remain unresolved; executable plans and mutating phases require their actual production prerequisites.

The canonical plan binds schema and action-format versions; exact config/policy bytes; resolved configuration roots and runtime routes; source/dependency/artifact/binary identities; owned-node and runtime identities; native/EVM snapshot hashes; all public roles; census and reset classifications; actual transaction payloads/origins; expected CREATE addresses and nonces; phase dependencies; validity windows; spend/count caps; and the chosen emission-denominator/remainder policy. Hash canonical bytes with domain separation. The signed authorization names that hash, network, expiry, allowed phases and ceilings. Reject duplicate fields, unknown schema versions, overflow, unexpanded substitutions and ambiguous addresses.

Implemented review commands, with no signing or submission:

```sh
sn-mainnet plan --outline > /secure/ur-mainnet/review/outline.json
sn-mainnet plan --config /secure/ur-mainnet/plan-config.json > /secure/ur-mainnet/review/blocked-plan.json
```

The JSON config and release-input schema are in [PLAN.md](PLAN.md). The resulting
blocked-plan hash cannot be passed as executable apply authority. The remaining
operator examples below are future interfaces requiring a separate executable
schema and complete semantic admission; the implemented `plan` does not accept
the draft YAML config, `--snapshot`, `--phase` or `--out` flags.

```sh
sn-mainnet inspect --config /secure/ur-mainnet/bootstrap.yml --out /secure/ur-mainnet/inspection
sn-mainnet apply --plan /secure/ur-mainnet/review/plan.json --accept-plan "$REVIEWED_MAINNET_PLAN_HASH" --authorization /secure/ur-mainnet/authorization.json --run-dir /secure/ur-mainnet/run
sn-mainnet status --run-dir /secure/ur-mainnet/run
sn-mainnet resume --run-dir /secure/ur-mainnet/run --accept-plan "$REVIEWED_MAINNET_PLAN_HASH" --authorization /secure/ur-mainnet/authorization.json
sn-mainnet verify --run-dir /secure/ur-mainnet/run --out /secure/ur-mainnet/verification
```

This future executable YAML config sketch is not the current JSON review input.
It intentionally contains `null` for unapproved identities and monetary values.
A real executable plan must reject them. Values represent required fields, not
suggested budgets or fake addresses; secret material is supplied through local
signer references rather than embedded here.

```yaml
schema: urnetwork-mainnet-bootstrap-v1
network: mainnet
deployment_id: null
netuid: 25
owned_node:
  substrate_url: null
  evm_url: null
  ownership_attestation: null
  expected_genesis_hash: null
  expected_evm_chain_id: 964
  runtime_artifact_manifest: null
  rpc_pacing: none
  fallback_urls: []
release:
  source_lock: null
  contract_manifest: null
  binary_manifest: null
  policy_file: null
  closed_testnet_report: sim-testnet/FINAL-4.md
  known_exceptions_manifest: null
  production_qualification_manifest: null
roles:
  subnet_owner: null
  evm_deployer: null
  coordinator_safe: null
  guardian: null
  commitment_oracle: null
  root_validator: null
  ur_validator: null
  independent_ur_validators: []
  reserve_hotkey: null
  escrow_hotkey: null
reset:
  mode: unresolved
  census_file: null
  remove_generations_file: null
  preserve_identities_file: null
  registration_during_cutover: null
emissions:
  denominator: native_miner_allocation_before_withholding
  provider_fraction: {numerator: 1, denominator: 10}
  assurance: observed-native-target
  quantization_tolerance_manifest: null
  remainder: owner-recycle
  mechanism_manifest: null
  activation_boundary: null
root_validator:
  registration_mode: null
  auto_parent_delegation: false
  basket_strategy: accumulate_in_place
  custom_weights: []
  stake_rao: null
  delegate_take: null
ur_validator:
  release_config: null
  stake_plan: null
  independent_validator_evidence: null
limits:
  total_tao_debit_rao: null
  total_alpha_commitment_units: null
  evm_fee_wei: null
  native_fee_rao: null
  per_registration_burn_rao: null
  root_registration_debit_rao: null
  maximum_registrations: null
  maximum_subnet_creations: 0
  maximum_transactions: null
  stake_price_limits: null
  expiry_finalized_block: null
operations:
  run_dir: /secure/ur-mainnet/run
  signer_manifest: null
  service_manifest: null
  worker_limit: null
  monitor_manifest: null
  independent_reader_manifest: null
  telemetry_and_alert_routes: null
  slo_manifest: null
  repair_authorization: null
  primary_on_call: null
  backup_on_call: null
  incident_evidence_store: null
  rollback_compatibility_manifest: null
```

The complete schema also requires action-level value/gas/fee bounds, collateral exposure, swap price/minimum-output limits, claim/deposit policy caps and fee reserves. Totals aggregate economic debits once across EVM and native representations. Refunds are recorded separately; they do not replenish lifetime authorization unless the plan explicitly defines that rule. Reverted transactions consume fee budget. New attempts, repairs and replacements retain the same lifetime ledger.

## Phases, finality and recovery

| Phase | Admission and work | Completion evidence |
| --- | --- | --- |
| 0. Evidence and production qualification | Preserve failed R48 and known exceptions; compose the launch release and close applicable MG-02 through MG-07 pre-activation checks on a controlled production-path rehearsal. Read-only identity discovery may proceed independently. | Exact release/qualification/exception manifests; no fabricated testnet pass or provisional authority carried into mainnet. |
| 1. Mainnet inspect and review | Verify node/runtime, complete census, owner authority, capabilities, keys, artifacts, reset method, selected owner-recycle mechanism and budgets. | Canonical feasible plan and exact operator authorization; any expected retained old miners have an explicit launch disposition. |
| 2. Cutover/reset | Execute the strongest safely admitted owner-key trim and configuration changes within their native windows. | Complete before/after census, preserved identities, actual removals, retained old miners and accounted locks; never label a partial trim a full reset. |
| 3. Contracts and registration | Deploy exact custody graph, anchor evidence, register approved pool/escrow/head/validator identities. | Canonical finalized receipts, code/getter proofs and registration ownership. |
| 4. Stake and service readiness | Apply bounded stake/deposit plans; admit both services, the second UR validator/operator safety set, independent monitor and on-call. | Current root membership, UR eligibility, authenticated runtime configs, single service ownership, delivered test alerts and qualified repair/rollout policy. |
| 5. Emission activation | At the approved native boundary, activate the qualified 10% mechanism and corresponding signed UR policy. | Native incentive outcome, remainder destination, weights, stake/collateral deltas and policy epoch agree. |
| 6. Acceptance and operations | Observe the specified production interval, settle/claim genuine accrued emission and reconcile all funds. | Self-contained final report; all four requested outcomes satisfied with no open cap/custody exceptions. |

Phases form a dependency graph, not a best-effort list. The final activation boundary may need a new snapshot and plan revision after lengthy setup; revisions authenticate the prior plan and completed receipts, preserve original immutable identities and lifetime caps, and explicitly authorize changed future actions. They do not rewrite the prior plan, retroactively approve execution, or reset spend.

Persist a write-ahead, hash-linked action journal with states such as `planned`, `intent-recorded`, `signed`, `submitted`, `included`, `finalized`, `postcondition-verified`, `failed` and `canceled`. Append and fsync the exact intent and signed payload before broadcast, with mode-0600 protection for recoverable signed bytes. Public evidence contains no secrets. Journal records include parent plan/action hashes, full origin/domain, payload hash, nonce, value/fees, transaction hash, actual block/receipt/event positions and the authenticated postcondition snapshot.

Use atomic manifest/config writes and one exclusive run owner. Native account and EVM/Safe nonce ownership must be explicit, including whether two representations share an underlying account. Serialize one-shot setup dependencies. Pipeline independent operations only after adapter-specific evidence shows nonce, finality and spend recovery remain correct. A timeout, canceled watch or lost RPC response is not proof that a transaction failed; search for the exact signed transaction before deciding to rebroadcast or replace it.

Native success requires finalized inclusion and successful dispatch at the correct extrinsic index. EVM success requires a canonical receipt and the corresponding native finality mapping, plus exact event/getter/code readback. Decode events with that block's authenticated metadata. Where batched calls can partially succeed, record every child result; prefer an actually atomic batch when the desired action requires all-or-nothing behavior. Never infer child success from the outer batch alone.

Recovery rules:

- For CREATE, find the original nonce transaction and compare exact address, code and immutable parameters; do not redeploy because a local marker is missing.
- For registration, compare hotkey ownership and generation, not only UID presence. A foreign occupant is a conflict, not an idempotent success.
- For one-shot links and initialization, an exact existing value is reusable only with authentic receipt/precondition history. A conflicting initialized value stops recovery.
- For stake, deposits and capture, reconcile actual deltas and retained-source balances before any retry. Never repeat an economic action because its terminal response was lost.
- For local rendered configuration, bind format, resolved route, paths and authority/capacity inputs in the action intent. An approved new local render can converge stopped services; it cannot pretend an old receipt already proves new bytes.
- For superseded actions, authenticate the historical postcondition at its source and the specific finalized successor that authorizes current state. Do not demand obsolete live equality after an approved successor, and do not globally waive current checks.
- On cancellation, stop new signing, join submitted transaction owners and service children, persist the actual terminal state, and report `CANCELED`, not `PASS`. An incomplete deployment or native inclusion remains recoverable work.

There is no automatic rollback of a finalized UID removal, registration burn or immutable deployment. Recovery uses a newly reviewed bounded forward action where supported. Old custody contracts and claim artifacts remain served until their obligations have actually ended. An emergency service stop does not imply that native emission stopped or that the vault may stop honoring claims.

## Continuous monitoring and repair

Mainnet operation needs three separate owners: an independent read-only monitor,
service supervisors, and a bounded repair controller. The monitor observes and
reports; supervisors recover an approved process generation; the controller
executes only already authorized actions. Implement and rehearse this separation
under MG-07/PH-28 before production activation. The current `monitor` command
supplies only the identity/finality foundation described above.

### Independent observations and existing telemetry

Run the monitor separately from bootstrap and validator/taskworker lifecycles,
without signing keys, database write credentials or authority to stop those
services. Give it its own bounded read budget, durable finalized-block cursor,
incident store and health signal. Replay from the last verified checkpoint after
restart; never replace missing observations with an assumed healthy interval.
Subscriptions wake readers but do not establish finality. Pin events, storage,
runtime interpretation and native/EVM mapping to the same authenticated block.

Compare the approved owned mainnet RPC against a separately operated,
independently authorized canonical source at the **same finalized block hash**.
Different latest heights alone are lag, not a reorganization. Validate both
identities, their available archive scope and the same transaction/event/storage
facts; preserve disagreements. The second source is read-only and cannot become
a submission fallback. Until it is provisioned, expose `independent_rpc=false`;
Snow and the LAN alias of one backend provide no independent confirmation.

Export SN metrics and structured incident events into the existing xops
Grafana/Mimir/Loki stack, using its Prometheus-compatible exporter and host
Fluent Bit paths. Reuse [deployment infrastructure](../../xops/main/ansible/playbook-dbs.yml)
and [telemetry isolation requirements](../../xops/VULNSCAN2.md): restricted
telemetry identity, scoped credentials, bounded journald retention and durable
log cursors. Do not mount signer material, a full vault or Docker control into
the observer or dashboard. Keep bounded metric labels to deployment, component,
role and error class; put transaction hashes, client-level detail and exact
evidence references in the incident store. Monitor telemetry delivery itself
through a separately hosted dead-man alert and named escalation route.

Every dashboard distinguishes unavailable, pending, healthy and failed facts:

| Domain | Evidence and progress to observe |
| --- | --- |
| Chain and authority | Genesis/EVM domain, approved runtime capabilities and code/metadata, finalized age/height, native/EVM mapping, node agreement, endpoint/config/release drift and archive availability. |
| Validators | Both UR validators' hotkey ownership, permits, non-self eligibility, fresh proof domains through each operator, native source/EMA continuity, durable intents and finalized revealed/applied weight rows. Root seat, stake/retention margin, child delegation and basket are a separate role. |
| Operators and providers | Current policy and evidence activations, migration version, processed client-key registrations and peer pins, ready provider count, fresh signed usage and bounded queues. HTTP 200 and process liveness do not establish registration or proof success. |
| Settlement and treasury | Exact source epoch/root/artifact, immutable usage snapshots and uncredited debt; required/observed deposits under the selected policy; pool capture, carry, commitments, finalization, claims and outstanding liabilities. Reconcile native units, collateral/principal and fee/lifetime allowances. Measure the 10% native target and approved tolerance independently of claimed payouts. |
| Transactions and deadlines | Every signed attempt, nonce owner, uncertain send, replacement/cancellation, canonical receipt and postcondition; blocks remaining to policy, commit, reveal, renewal, claim and evidence-retention deadlines. |
| Services and resources | Process generation and restart count, last useful checkpoint, database/artifact health, CPU/memory, RPC concurrency, queue age, log byte lag and disk bytes/inodes. No healthy status from a stale lock file. |

### Alert taxonomy and initial SLOs

The following are proposed starting targets. Freeze them in the operations
manifest after a representative load/recovery rehearsal and before activation.
They are not claims of measured availability. Protocol deadlines remain exact
block boundaries; human response targets cannot extend them. Use a lightweight
health loop while expensive replay proceeds under a separate finite budget.

| Alert class | Starting detection/SLO target | Response |
| --- | --- | --- |
| Integrity, authority or accounting conflict | Emit immediately on an authenticated wrong-chain/domain, finalized-hash conflict, invalid signature, custody/conservation mismatch or unauthorized spend. No averaging or transient-error allowance. | Critical page; suspend dependent new signing through its owner and preserve evidence. Continue independent observation and valid claim service where safe. Primary acknowledges within 5 minutes; backup escalation after 5 minutes without acknowledgement. |
| Monitor or alert path absent | Target a health/progress event at least every 30 seconds; warn after 90 seconds, page after 2 minutes without one. Test alert delivery before activation and after routing changes. | Independent dead-man page; restore observation first. Missing monitor samples remain a gap, not a healthy interval. |
| RPC/read availability or stalled finality | Record every error as unavailable; warn after 2 minutes of persistent read failure, page after 5. Warn at 3 minutes without finalized advance and page at 5, after calibrating to admitted chain cadence. | Bounded retries/reconnect on approved routes, inspect chain-wide versus node-local failure, and block new actions lacking required fresh evidence. Never compare an unread default value with an approved one. |
| Deadline or readiness risk | Recompute at least every 30 seconds and on each new finalized block. Warn when remaining blocks fall below the greater of 20% of the window and twice measured p95 completion/finality cost. Page when the admitted completion margin is no longer available, or a required role remains unavailable for 2 minutes. | Resume the exact pending action if authorized; otherwise escalate a concrete forward plan. Record a missed boundary as missed. Both UR validators and every required operator/domain must remain independently visible. |
| Settlement and reward deviation | Evaluate every due finalized event/epoch and native emission interval; immediate critical alert for conservation or authority failure, deadline alert for missing work, explicit alert for a 10% result outside approved `Q(k)`. | Trace source usage through liabilities and receipts. Do not fabricate usage, increase a governed deposit to a native minimum, or count a late root as timely. |
| Resource exhaustion or replay backlog | Warn below 20% free bytes/inodes or when forecast capacity is under 24 hours; page below 10% or a shorter time than safe intervention. Alert if log/queue lag exceeds its approved window or foreground work loses its completion margin. | Reduce bounded background admission or restore capacity within policy; never delete signed evidence or increase spend/capacity authority silently. |
| Recovered incident / recurring degradation | Retain first failure, retry count, recovery evidence and recurrence by stable incident ID. Noncritical pages acknowledged within 15 minutes; unresolved incidents carry an owner and next action. | Review open incidents daily and recurrence/capacity/runtime-change trends weekly. Create a scoped fix with causal regression and affected-path qualification. Recovery does not erase the failure. |

Select recovery-time objectives from real replay and protocol windows before
launch. Durable intent and signed-transaction recovery has **zero tolerated loss
of acknowledged records**; a retry may repeat observation but may not repeat an
economic effect. Local crash recovery requires fsync and restore evidence. A
zero-loss host-failure objective additionally requires independent durable
replication before acknowledging/broadcasting signed work; periodic backups
alone cannot provide it. Bind that recovery design before enabling automated
spend. Publish actual recovery times and observation gaps alongside the target
after each exercise or incident.

### Repair authority and durable execution

Provide a standing signed repair envelope for routine operations the operator
chooses to automate. It binds chain/deployment, immutable release, allowed
action kinds and exact targets, signers/nonce domains, prerequisites, expiry,
maximum attempts and action counts, per-action value/gas/fees, total lifetime
debits, price/minimum-output limits and permitted postconditions. Automation can
continue within that envelope without asking again for each identical retry.
Changing its scope or exceeding a bound requires a new exact reviewed plan.
Default monetary limits are zero until supplied; alert severity never grants
transaction authority.

| Action | Automation boundary |
| --- | --- |
| Reconnect/retry reads, replay authenticated immutable evidence | Allowed within the monitor/worker's finite budget and unchanged authority. Preserve successful checkpoints and typed failures. |
| Restart an approved service or resume a stopped worker | Allowed by its service manifest only after joining the old process/children and retaining signer, volume and configuration identity. Cap restarts; escalate exhaustion. |
| Reconcile a previously signed transaction | Read receipts, dispatch, postconditions and nonce state automatically. Rebroadcast the exact bytes or make a replacement only when the owning approved action explicitly permits it and its bounds still hold. |
| Scheduled renewals, routine claims or approved funding repairs | Automatic only under their own signed targets, amount/count/price/deadline caps and lifetime ledger. A schedule alone is not spending permission. |
| Policy/rate/source/native-history changes, new registration or stake, destructive reset, custody/contracts, runtime admission, release/schema or endpoint changes | Operator-gated exact plan with the actual required coldkey/Safe/governance authority. No self-approval, permission widening, backdated success, historical signature rewrite or guessed SQL credit. |

Use one durable action/nonce owner shared with the production submitter. Append
and fsync an incident-bound intent before signing, then exact signed bytes before
broadcast. Journal `planned → intent-recorded → signed → submitted → finalized
→ postcondition-verified` with explicit failed, canceled and unresolved branches.
Store replacement/cancellation attempts separately; retain all paid fees and
outstanding liabilities across releases and retries. Before any repeat, locate
the original exact hash and reconcile canonical inclusion, dispatch, finality,
nonce and economic postcondition. A timeout is an unknown outcome. A new nonce
or a local database status is not evidence that the old action failed.

The [operator receipt-census correction](evidence/operator-recovery-census-20260927.md)
preserves this boundary in the production account reconciler: if a retained
candidate's receipt cannot be read and no other candidate is canonical, the
intent remains unresolved. An advanced nonce cannot erase that unknown outcome,
and elapsed replacement time cannot turn the failed read into new signing
authority. MG-03/PF-03 still require complete discovery across both operator
databases and evidence stores, historical-status reconciliation and full fees.

The independent monitor confirms the repair's postcondition at finalized state.
Only then close the incident, retaining its history and action receipts. A local
repair success with missing chain evidence stays pending. Recovery cannot erase
failed acceptance assertions or turn an observation gap into a completed epoch.

### On-call, incident evidence and rollout

Before activation, name a primary and backup operator, establish the alert route
and access to read-only diagnostics and the appropriate signing process, and
rehearse this runbook:

1. Acknowledge the incident and pin its deployment/release, actual process owners,
   last good checkpoint and current native/EVM blocks. Distinguish an unavailable
   read from an authenticated mismatch before deciding containment.
2. Stop only dependent new signing or unsafe work through its existing owner.
   Keep monitor, immutable history and valid earned claims available. Preserve
   signed and in-flight transactions for reconciliation; stopping a process does
   not stop native emission or remove custody obligations.
3. Seal an incident bundle: exact config/plan/runtime hashes, raw RPC responses
   and receipts, relevant signed artifacts, log byte ranges/cursors, queue state,
   liabilities, alert timeline and attempted repairs. Restrict signed transaction
   bytes and redact credentials; hash public evidence separately.
4. Reproduce the actual failure and inspect adjacent callers. Use the standing
   repair only if every precondition still holds; otherwise prepare the concrete
   bounded forward plan or code fix, qualify it, and obtain its required authority.
5. Reconcile transactions and databases before retry or service replacement.
   Verify the result independently, observe sustained proof/settlement progress,
   and retain unresolved consequences as open incidents. Record detection,
   response and recovery times and the follow-up owner.

Roll out immutable images with the qualified source/dependency/config manifest.
Run read-only shadow checks, then a canary with no duplicate signer and enough
capacity to preserve the required validator/operator quorum. Stage additive
database migrations before compatible consumers; specifically rehearse populated
client-key policy rollover and immutable usage writer/reader compatibility.
Advance only after current-domain readiness, resource bounds, fresh proofs and
finalized chain postconditions are observed. Thresholds and canary duration are
approved in the rollout manifest before execution.

Keep the previous image and an explicit state-format compatibility matrix.
Rollback is allowed only if the old reader/writer can safely interpret the
current schema, policies and signed state. An irreversible migration, finalized
registration, immutable deployment or economic transfer requires forward
recovery; restoring an old filesystem cannot undo it. Always reconcile in-flight
transactions and join old owners before changing images. Restore tests must
prove journals, keys and artifacts remain usable with no duplicate spend. End a
rollout with an independent state comparison and updated incident/capacity
records, not just a green process list.

## Integration with this repository

Reuse importable production packages: [crv4](../crv4) for authenticated runtime/native reads and transaction evidence; [stabi](../stabi) for the release ABI surface; [protocol](../protocol) for policy and domain encoding; [validator](../validator) for UR validation and evidence; and existing cryptographic/address/Merkle primitives where their units and domains match.

Do not import `sim-testnet` as a production dependency: it is a `package main` campaign with fixture, finance-repair, historical migration and adversarial machinery. Extract only a required, qualified generic facility into a neutral internal package when implementation begins. Keep mainnet capability adapters explicit, and keep the new root-validator implementation separate from UR scoring. The old [stctl configuration](../stctl/config.go) identifies itself as legacy pre-1.0 and rejects the current deployment domain; it is not the release-1.0 mainnet control plane.

Suggested future package boundary:

```text
mainnet/main.go                  argument parsing and command dispatch
internal/mainnetbootstrap/       config, snapshots, capabilities, plans, executor, reports
internal/rootvalidator/          root membership/basket observer and approved action loop
internal/chainactions/           only extracted, proven transaction/journal primitives
validator/                      existing production UR validator
```

Use interfaces for `FinalizedReader`, `NativeSigner`, `EvmSigner`, `SafeSigner`, `Submitter`, `Journal` and `ServiceSupervisor`. Follow the [UR Go style guide](../../connect/CODESTYLE.md): owned Go identifiers use `Evm`, `Rpc`, `Uid` and `Id` casing, `self` receivers and the prescribed field naming. Preserve externally required, generated and wire-format names. Packages may import a parent or peer, never their own child; the proposed internal packages are peers of the command package, not a bypass for that rule. A signer returns an identity-bound signed payload; it does not decide policy or fall back to another account. Mainnet configuration decoding must finish before any durable worker or RPC connection starts. Keep testnet provisional admission flags, deterministic fixture keys, fabricated identities, accelerated epochs, simulator routes and faucet/funding assumptions outside this interface.

## Acceptance evidence and implementation qualification

The closed testnet campaign requested 33 roles, 1,000 providers, 202 candidates,
200 head positions and two UR validators, five accelerated epochs and a later
production-policy observation. Those requirements were not completed by R48:
its original result is failed with zero complete acceptance epochs, and later
retained recovery remains non-accepting. Preserve that report and its explicit
exceptions as inputs. Mainnet is not blocked on reopening that campaign; it is
blocked on the [production gates](PRELAUNCH-FIXES.md#production-gates-in-execution-order)
and the actual capabilities, accounting and operating evidence required here.

Qualify the composed production release on a controlled integration deployment
with the relevant failure/recovery cases. Reuse historical tests only with exact
source/dependency and requirement mapping. A simulator-only patch, canceled
setup, signed plan, read-only monitor or partially observed epoch cannot replace
the required evidence. Keep one manifest of implemented, qualified, deployed
and operationally observed states, with every remaining exception explicit.
Mainnet has its own release identity, budgets and 50,400-block policy; testnet
provisional authority and accelerated timing do not carry over.

The future mainnet acceptance bundle contains:

| Outcome | Evidence required |
| --- | --- |
| Reset | Signed target/preserve census, exact finalized removal mechanism, before/after hotkey generations and UID mapping, unchanged required custody/validator ownership, residual stake/lock report. |
| Contracts | Source/toolchain/artifact hashes, predicted/actual addresses and nonces, creation receipts, runtime bytecode and immutable getters, Safe authority, one-shot links and evidence anchor, preserved custody invariants. |
| 10% miner rewards | Explicit denominator and activation boundary; exact native interval accounting; finalized incentive outcomes including collateral; direct-head and tail entitlement reconciliation; verified 90% owner-recycle outcome and mode, with no reserve credit; runtime-derived quantization tolerance and actual target result; proof of a stronger hard cap only if that assurance was selected. |
| Root validator | Real netuid-0 membership and owner mapping, bounded admission receipt, stake/retention observation, child policy, actual basket strategy, live owned service and runtime identity. |
| UR validator | Real UR eligibility and live applied/revealed CRv4 rows, authenticated evidence/usage, service signer, second UR validator and operator safety minima. |
| Financial/finality closure | Actual spend including failures, remaining allowance, all transaction owners joined, native/EVM finality mapping, open liabilities and future operations clearly reported. |

Measure activation across at least three complete native emission intervals, and observe a full mainnet UR settlement/claim cycle before declaring settlement acceptance. The 50,400-block cycle cannot be replaced by accelerated testnet timing. Bootstrap may report `DEPLOYED_AWAITING_SETTLEMENT` while that observation is pending; it must not call the whole requested program accepted early.

Implementation qualification is owned by **Terra medium for tests, race runs, builds and formatting; Astra max diagnoses and authors fixes**, following the [Go style guide's bug-fix and testing policy](../../connect/CODESTYLE.md). Every actual fix needs a regression that deterministically reproduces the pre-fix failure and verifies corrected behavior at the observable failing layer. Use explicit barriers, hooks or state transitions for ordering; sleeps, negative timeouts, queue polling and scheduler luck are not the primary proof. Review surrounding code, sibling call sites and similar patterns before declaring the root cause fixed, and record any affected adjacent paths.

Keep regression data visibly synthetic: generated test-only identities, `.example` hosts and reserved documentation addresses; sanitize captures before turning them into fixtures and retain necessary raw evidence outside source. Tests are top-level `func TestXxx(t *testing.T)` declarations. Use separate top-level tests or plain table loops for ordinary cases; use `t.Run` only when isolating and asserting a deliberately failing subtest is itself the subject. This document does not execute or request execution of new tests. When implementation is authorized, cover meaningful boundaries and recovery:

1. Command-level read-only preview cannot acquire a signer or submit, including malformed/duplicate config, wrong genesis/chain, testnet identity, route substitution, stale snapshots and runtime upgrades. Also cover a separately attested mainnet identity subsequently hosted at a previously testnet address.
2. Real pinned metadata/call encoding proves owner versus Root origins and both root/UR call domains. Negative cases include unsupported trim, wrong signer, root `register_limit`, missing price protection and native/EVM rollback failure for any new registration wrapper.
3. Reset census and actual pruning/trim replay cover dual-role neurons, validator-without-permit preservation, custody identities, immunity boundary, tie ordering, concurrent registration, minimum capacity, UID renumbering and surviving collateral. A smaller UID count alone cannot pass.
4. Full emission-path tests demonstrate why scaling weights or theta does not impose a cap; then prove the selected 10% target and runtime-derived tolerance through quantization, consensus, independent-validator rows, no-incentive fallback, multiple mechanisms, denominator-boundary accrual, locked rewards and cumulative dust. Distinguish actual target observation from any selected hard-cap guarantee. Cover burn and recycle accounting separately and refuse an unsupported reserve transfer.
5. Deployment tests use real creation payloads and contract execution for nonce `n+3` escrow registration, atomic proxy initialization, mapped coldkeys, Safe checks, refund/fee bounds, one-shot conflicts and evidence anchoring. A changed custody contract requires its own conservation/claim and adversarial review.
6. Crash/restart tests inject failure before and after signing, submission, finality and fsync; prove no duplicate burn/deposit/stake and no lost finalized success. Cover nonce collision, reorg before finality, failed batch children, canceled owners and authenticated successor history.
7. Process tests prove both distinct validator roles, no silent endpoint fallback, single signer ownership, complete child joining, current UR permit/reveal checks, root custom/default strategy handling and no unbounded restaking/re-registration loop.

Run bounded normal tests on the frozen implementation, then appropriate race tests for shared state, journal ownership and supervisors. Retain exact source, binary, selector, package working directory and terminal evidence. Diagnose any actual failure on that capture before retry; preserve failed evidence and use the established confirmation protocol. Reuse unaffected qualification only with an explicit source/dependency mapping; changed custody/runtime economics require their relevant full tests and owned-node rehearsal. There are no mainnet tests against public RPC and no broadcast hidden in a test command.

## Open inputs before an executable mainnet plan

Snow VPN `172.28.208.185:9944` is the intended mainnet route, but the node
operator reports it is still being prepared; the latest read-only inspection
returned HTTP 502 while the preceding one returned testnet chain ID 945. Reinspect it after cutover, and do not
construct or sign mainnet actions until it serves the approved mainnet identity.
Obtain an independently approved mainnet genesis/runtime identity and complete
SN25 census. Compose and qualify the production source/dependency release with
the retained R48/R46 lessons, then implement the bootstrap mutation paths and
separate root-validator service. Resolve the actual reset capability and implement the selected 90% owner-recycle
policy with the observed 10% native allocation and runtime tolerance, root custody and
registration protection, real identities and budgets. Install independent
monitoring, existing-stack alerts, bounded repair authority and the on-call
runbook before activation. Each unresolved item remains visible in the plan and
report; the closed testnet effort is not relabeled as a pass.

[subtensor-commit]: https://github.com/RaoFoundation/subtensor/commit/67dcf7f791dc495064c293f080a0702cb433e51e
[subtensor-admin]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/admin-utils/src/lib.rs
[subtensor-storage]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/lib.rs#L1661
[subtensor-uids]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/subnets/uids.rs#L171
[subtensor-registration]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/subnets/registration.rs
[subtensor-root]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/coinbase/root.rs#L88
[subtensor-weights]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/subnets/weights.rs#L875
[subtensor-dispatches]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/macros/dispatches.rs
[subtensor-coinbase]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/coinbase/run_coinbase.rs
[subtensor-alpha-accounting]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/coinbase/alpha.rs#L38
[subtensor-shares]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/coinbase/subnet_emissions.rs#L354
[subtensor-alpha-precompile]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/precompiles/src/alpha.rs#L297
[subtensor-epoch]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/pallets/subtensor/src/epoch/run_epoch.rs
[neuron-interface]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/precompiles/src/solidity/neuron.sol#L206
[root-reborn]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/docs/guides/root-reborn.mdx
[collateral-guide]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/docs/guides/mining/collateral.mdx
[max-uids]: https://github.com/RaoFoundation/subtensor/blob/67dcf7f791dc495064c293f080a0702cb433e51e/docs/hyperparameters/max-allowed-uids.mdx
