# Runtime rehearsal node

Activation gate 2 (`VERSION2-FINAL.md` §14) runs the exact release bytecode of
the settlement contracts against the real Subtensor precompiles. Anvil cannot
run them, the testnet campaign closed and there is no localnet, so this
directory builds a node from the upstream source of runtime 475, the runtime
mainnet (EVM chain 964) runs, and starts a one-node dev chain on it. The Go
harness and its tests are in `rehearsal/`.

## Build

```sh
PROTOC=/path/to/protoc CARGO_TARGET_DIR=/path/to/target \
  scripts/runtime-rehearsal/build.sh /path/to/subtensor-checkout
```

`build.sh` clones `https://github.com/opentensor/subtensor.git` into the
checkout directory if it does not exist, and checks that the tag `v475` names
`d1718c99c34cf96abbf2bf09c0e9e48b945c76f5`. It checks out that commit, requires
a clean tree, `spec_version: 475` in `runtime/src/lib.rs` and every digest of
`docs/spec/runtime-v475-source.sha256`, then builds:

```sh
cargo build --locked --profile release -p node-subtensor \
  --features "metadata-hash fast-runtime" -j 10
```

(`RUNTIME_REHEARSAL_JOBS` overrides the `-j 10`.)

Upstream's localnet (`scripts/localnet.sh`, `fast_runtime=True`) builds the
workspace with `pow-faucet metadata-hash fast-runtime`. `metadata-hash` is also
in mainnet's release build, and `fast-runtime` gives fast blocks. `pow-faucet`
only adds a proof-of-work faucet call and drops the default subnet-registration
rate limit to 0, so it is left out and the runtime keeps mainnet's call set.

The script runs cargo inside the checkout, so rustup selects its
`rust-toolchain.toml` (1.89, target `wasm32v1-none`), and sets
`WASM_BUILD_WORKSPACE_HINT` to the checkout. Without the hint, a target
directory outside the checkout hides `Cargo.lock` from the runtime's Wasm
build, which then resolves unlocked dependencies and fails
(`ruint@1.20.1 requires rustc 1.90`). The build also needs `protoc`, `clang`,
`jq`, `xxd` and `b2sum`. Finally the script checks that the dev chain's
genesis `:code` is the built Wasm byte for byte and writes the record
`release/runtime-rehearsal-build.txt` in the target directory.

Measured on 2026-10-11 on a 24-thread host shared with other jobs, at `-j 10`:
the build from scratch took about 37 minutes (2,243 s wall clock, including one
restart that kept every compiled crate, after the missing workspace hint
above), and a rebuild of the node crate alone 266 s. The runtime Wasm of that
build is:

| Artifact | Bytes | SHA-256 | BLAKE2b-256 |
| --- | ---: | --- | --- |
| `node_subtensor_runtime.compact.compressed.wasm` | 2,862,456 | `4bdb6b5dada78e742db06c94523dea8734d737832b5f6a118ecf8d26f6813ce4` | `a486ccc60a0ff5c2a3362e19470186fda365edcda1dfc688ba3d88e36e60fca6` |

The BLAKE2b-256 is the chain's `state_getStorageHash(":code")`. It differs from
mainnet's 475 Wasm (2,632,482 bytes, SHA-256 `00ef6bf4…`), as expected:
`fast-runtime` changes constants, and `docs/spec/runtime-475-audit.md` explains
why even a production-feature rebuild is not expected to match byte for byte.

## Run

```sh
RUNTIME_REHEARSAL_NODE=/path/to/target/release/node-subtensor scripts/runtime-rehearsal/run-node.sh
```

`run-node.sh` starts the `dev` chain spec: one authority, Alice as sudo, and
Alice, Bob and Charlie with 1,000,000 TAO each. It patches two raw genesis
values: the EVM chain ID becomes mainnet's 964 (`EVMChainId.ChainId`), and
contract creation is open to every account (`EVM.DisableWhitelistCheck`).
Blocks are sealed on demand, and finalized at once, by the RPC
`engine_createBlock(true, true, null)`; there is no GRANDPA. Substrate and
Ethereum JSON-RPC share one port on 127.0.0.1. Optional environment:
`RUNTIME_REHEARSAL_BASE_PATH` (default a new temporary directory),
`RUNTIME_REHEARSAL_RPC_PORT` (9944), `RUNTIME_REHEARSAL_P2P_PORT` (30333) and
`RUNTIME_REHEARSAL_SEALING` (`manual`; `instant`, or a period in milliseconds
to seal on a timer).

Differences from mainnet: `fast-runtime` sets 250 ms slots, a default subnet
tempo of 10 (mainnet 360), up to 32 epochs per block (mainnet 2) and short
rate limits and swap delays. The weight formulas, and so the gas schedule,
are mainnet's (40,000 weight per gas). The dev genesis has no SN25 state. With
tempo 10, the default 10-block admin freeze window closes a subnet's admin
calls in 9 of every 10 blocks; set `AdminUtils.sudo_set_admin_freeze_window(0)`
first. A second subnet within 7,200 blocks needs
`AdminUtils.sudo_set_network_rate_limit(0)`.

## Go harness and smoke test

```sh
RUNTIME_REHEARSAL_NODE=/path/to/target/release/node-subtensor \
  go test -tags runtime_rehearsal ./rehearsal/ -run TestRehearsalSmoke -v
```

Run it from the repository root with `forge` on `PATH`. The test files of
`rehearsal/` carry the `runtime_rehearsal` build tag. Each test starts its own
node through `run-node.sh` on free ports in a temporary directory and stops it
at the end; the node is ready in about 10 s. The
helpers sign extrinsics with the repository's signer (`chain.EncodeSignedCall`
over a `crv4.DialChainContext` connection), seal each transaction into its own
block, check dispatch success with `crv4.VerifyFinalizedExtrinsicContext`, and
read storage with the `chain` readers. They fund SS58 and EVM accounts (an EVM
account's balance is its mapped account's), create a subnet, register
hotkeys, stake for any coldkey, including a contract's mapped coldkey
blake2b-256("evm:" ‖ h160) (`ss58.EvmMirrorPubkey`), dispatch as sudo, seal
blocks (100 empty blocks take about 1 s), build contracts with `forge` and the
pinned toolchain and deploy their creation bytecode, send and simulate EVM
calls, and read stake through the staking precompile (`getStake`) and through
storage (`TotalHotkeyAlpha`).

The smoke test deploys `evm/test/rehearsal/RehearsalProbe.sol`, which calls a
precompile as itself, measures the call's gas and reports the call value it
receives. Its mapped coldkey owns stake on an emitting subnet. After two tempos
of dividends the staked hotkeys' share price is above 1; tempo 0 then stops
epochs, so each delta is the operation's alone, and a hotkey registered after
that keeps share price 1.
Measured on 2026-10-11 (the test takes about 18 s):

- **getStake** through the probe equals the direct precompile read. The
  hotkey's `TotalHotkeyAlpha` is the probe's stake plus its owner's take plus
  1 rao of share-pool floor.
- **Moves and transfers** (eight amounts each, from 100,000,007 to
  5,000,000,003 rao): the source lost exactly the amount, in `getStake` and in
  `TotalHotkeyAlpha`, and the destination's `TotalHotkeyAlpha` gained exactly
  the amount, every time. The destination's `getStake` was one rao short in 1
  of 8 `moveStake` and 1 of 8 `transferStake` calls onto a hotkey at share
  price above 1, and in 3 of 4 whole-position calls there; it was exact in 16
  of 16 calls onto a hotkey at share price 1.
- **Floor**: below 100,000 rao of TAO the runtime refuses a same-subnet
  `moveStake` or `transferStake` with `AmountTooLow`. At price 9.014779842 TAO
  per alpha, 11,092 alpha rao was refused and 11,093 accepted, for both calls,
  which is ceil(100,000 / price). The price stayed the same across blocks of
  emission.
- **Whole position**: `moveStake` with 2^64−1, 2^64 and 2^256−1, and
  `transferStake` with 2^64−1, each moved the whole position.
  2^64−2 was refused with `NotEnoughStakeToWithdraw`.
- **Rollback**: a transaction that reverts after a successful `moveStake`
  leaves both positions unchanged.
- **Registration**: `registerLimit` and `burnedRegister` from the probe
  register a hotkey owned by the probe's mapped coldkey, which pays the burn
  (1,417,205,419 and 1,782,243,972 rao) with zero call value; `getUid` then
  returns `(true, uid)` with the stored UID, and `(false, 0)` for an
  unregistered hotkey.
- **Registration collateral** (`pallets/subtensor/src/subnets/collateral.rs`;
  `CollateralLockShare` defaults to 0, and the subnet owner or root sets it
  with `AdminUtils.sudo_set_collateral_lock_share`): at lock share
  2^15/65535 a `registerLimit` from the probe paid the full price,
  2,236,996,529 rao, swapped it into alpha, burned half of that alpha and
  staked and locked the other half, 115,828,883 alpha rao, on the (hotkey,
  probe coldkey) position. `getStake` returned exactly the lock, and the
  precompile's `getMinerCollateral(uint16,bytes32,bytes32)` returned
  `(true, 115828883, 2^64, 0, 0)` (exists, locked, drain ratio 1 as Q64.64
  bits, minimum locked, earned), and `(false, 0, 0, 0, 0)` for a position
  without collateral. With free stake added on top, `moveStake` and
  `transferStake` of the free amount plus 1 rao, and of 2^64−1, were refused
  with `StakeUnavailable`; exactly `getStake − locked` moved, leaving the lock.
- **Locked alpha** (`staking/lock.rs`, `AccountFlags`): every coldkey,
  including a contract's mapped coldkey, rejects incoming conviction-locked
  alpha unless it opts in (`getRejectLockedAlpha` returned true for the
  probe). A third party that locked 4,949,971,706 of its 9,899,943,412 rao
  could not `transfer_stake` the whole position to the probe
  (`AccountRejectsLockedAlpha`); its unlocked part moved.
- **Call value** (`SubtensorEvmBalanceConverter`, `runtime/src/lib.rs`): the
  runtime moves floor(value / 10^9) rao. Sent 1, 999,999,999, 1,000,000,000
  and 1,999,999,999 wei, the contract saw that exact `msg.value`, while its
  balance and its mapped account gained 0, 0, 1 and 1 rao, and the sender was
  debited the same whole rao beyond the fee: the sub-rao part never moves.
- **Refusals**: an `eth_call` from the contract's address to the precompile
  returns `evm error: Other("dispatch execution failed: <Error>")`. Inside a
  contract the call fails with empty return data and consumes all the gas
  forwarded to it: a refused transaction used 9,844,446 of its 10,000,000 gas.
- **Gas** (maximum over the move and transfer sweeps; transaction figures
  vary by a few gas units with the random keys in calldata):

  | Call | In the probe | Transaction used | `eth_estimateGas` limit |
  | --- | ---: | ---: | ---: |
  | `getStake` (view) | 4,748 | | |
  | `getUid` (view) | 995 | | |
  | `getMinerCollateral` (view) | 1,013 | | |
  | `moveStake` | 81,753 | 107,665 | 2,427,799 |
  | `transferStake` | 79,321 | 105,233 | 2,420,524 |
  | `registerLimit` | 1,409,550 | 1,434,798 | 1,493,416 |
  | `burnedRegister` | 1,396,194 | 1,421,266 | 1,479,461 |

  The runtime charges a dispatch's actual weight as gas, but admits it only if
  the gas forwarded covers its declared weight, which for `move_stake` and
  `transfer_stake` includes walking a full 256-entry `StakingHotkeys` list. A
  transaction that moves stake therefore needs a gas limit of about 2.4M
  while it uses about 105,000.
- **Emission**: no validator set weights, and Yuma's fallback paid the staked
  hotkeys validator emission in proportion to their stake every tempo. In an
  epoch without weight-derived emission it pays active permit holders by stake
  (every UID by stake if none is active), so the escrow hotkey stays at share
  price 1 only while validators set weights; R-3 needs one.
