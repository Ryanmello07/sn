# Contract size candidate, 2026-09-27

The current source at `cb44dfdd` passes `forge test --root evm -q` and
`forge build --root evm --sizes` with Foundry 1.7.1, solc 0.8.24, Cancun,
optimizer 200 and IR codegen. This is a source/build check, not an approved
mainnet deployment or an attestation of the live Subtensor EVM limit.

| Deployable contract | Runtime bytes | Foundry margin to 24,576 bytes |
| --- | ---: | ---: |
| `STCoordinator` | 24,564 | **12** |
| `STReserveSink` | 1,558 | 23,018 |
| `STSettlementVault` | 9,797 | 14,779 |
| `STValidatorEvidence` | 12,192 | 12,384 |

The coordinator has effectively no size headroom. Freeze its exact build
inputs and generated deployment bytecode in the approved release manifest;
rerun this check after any source, compiler, optimizer or dependency change.
Before mainnet deployment, confirm the selected live runtime enforces the
expected code-size rule and exercise creation/readback with the exact artifact.
A successful local build alone does not establish that the live deployment will
succeed.

Raw build output and artifact checksums are retained under
`/mnt/data/sn-testnet/evidence/mainnet-contract-size-20260927/`.
The `STCoordinator.json` SHA-256 is
`87b4a6b2a4a8140fcb5a64d50d5eb3772c109f8696aba957d8976d638d850054`.
The raw `forge-build-sizes.log` SHA-256 is
`62771fe93335e0bcb6f8ac1595af140650f524758e80d33a6b4a8694728ff597`.
