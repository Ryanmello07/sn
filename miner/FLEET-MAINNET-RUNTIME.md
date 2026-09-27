# Mainnet fleet runtime authority

Production `provider fleet register`, `publish`, `bind`, `status`, and `revoke`
require a separately reviewed authority document and its approved SHA-256:

```text
--mainnet-runtime-authority=/absolute/path/reviewed-fleet-runtime.json
--mainnet-runtime-authority-sha256=<64 lowercase hex digits from approval>
```

There is no shipped mainnet genesis, runtime pin, or approval. An endpoint
observation, testnet release lock, testnet profile receipt, or successful dry
run cannot supply approval. The digest argument is the operator's explicit
selection of previously reviewed bytes; it is not a signature or a replacement
for the external approval process. Do not obtain it automatically from the
endpoint or from a newly generated observation.

The document is a strict JSON object, at most 16 KiB, with these fields:

| Field | Required meaning |
| --- | --- |
| `schema` | `urnetwork-mainnet-fleet-runtime-authority-v1` |
| `native_chain` | Independently approved exact native chain name |
| `genesis_hash` | Independently approved native genesis; lowercase nonzero `0x` plus 64 hex digits; known testnet genesis refused |
| `evm_chain_id` | `964` |
| `netuid` | Approved subnet, exactly matching the fleet manifest |
| `coordinator` | Approved coordinator, exactly matching the manifest; lowercase nonzero `0x` plus 40 hex digits |
| `runtime_source_commit` | Reviewed immutable runtime source commit; 40 lowercase nonzero hex digits |
| `runtime_review_scope` | `urnetwork-fleet-register-commitment-frontier-v1` |
| `runtime_review_sha256` | SHA-256 of the independently retained source/build/interface review; 64 lowercase nonzero hex digits |
| `runtime_version` | Complete object with `specName`, `specVersion`, `transactionVersion`, `stateVersion`; nonempty name and nonzero versions |
| `runtime_code_hash` | Exact reviewed `:code` BLAKE2b-256, lowercase nonzero `0x` plus 64 hex digits |
| `runtime_metadata_hash` | Exact reviewed SCALE metadata BLAKE2b-256, same hash grammar |

The review must establish source-to-Wasm provenance and bind those exact code
and metadata bytes. It must cover the consumed signed extensions, account and
registration storage/calls, fee quotation, commitment encoding/storage, finality
and dispatch events, plus Frontier's native/EVM height relationship and contract
execution used by fleet commands. The loader verifies the approval document's
bytes and required coordinates; it does not perform that source/build audit.
Version numbers alone, or matching endpoint hashes without the independent
review, are insufficient. The reviewed coordinator deployment and subnet are
separate necessary deployment inputs; runtime admission is not a contract
implementation/deployment audit.

Each command loads the document once. Native genesis/name and the exact runtime
version/code/metadata tuple are checked at explicit finalized blocks. Registration
passes the independent artifact to the existing bounded-burn/fee signer, checks
again before signing and broadcast, and authenticates receipt state before its
registration readback. Publication similarly checks before signing and broadcast
and uses authenticated metadata at the exact write block. Status authenticates
both the native commitment and the EVM route.

Bind and revoke use native identity/runtime methods on the **actual EVM
connection**, as well as EVM chain ID 964. Therefore those routes must expose the
Substrate read methods too. They authenticate before producing client/hotkey
consent, on the submitter's connection before preflight/signing/broadcast, and
at the native height corresponding to the finalized EVM receipt. The coordinator
address must match the approved manifest scope. Revoke also compares the
remote finalized digest against the exact locally encoded revocation domain.
A reached wrong EVM identity does not trigger fallback to another route.

Chain 945 retains its exact release pin and explicit testnet provisional path.
Production flags cannot combine with testnet provisional flags, and production
cannot use a connection carrying provisional authority. A new runtime requires
a new independent review and approved document; no runtime is auto-adopted.
The offline `fleet manifest` command remains independent of network admission.

## Receipt and reconciliation limits

An upgrade or network mismatch after submission can leave a real, finalized
transaction whose runtime/readback approval fails. The command returns an error
and does not automatically send again. Native registration keeps its existing
exact-byte journal; EVM submission keeps its prepared transaction/hash output.
Those record submission/inclusion, not mainnet runtime or economic acceptance.

RT-03/PF-03 remains open for complete durable intent/result reconciliation across
process death, unknown send outcomes, and fleet publication/EVM restart. This
change does not add such a journal or make manual reruns safe. Reconcile the
original transaction before resubmission; a receipt-admission error is not proof
that nothing happened. No new approval can retroactively reinterpret an old
signature or receipt as approved under different runtime bytes.

No mainnet transaction, deployment, or production pin selection is part of this
source qualification. Independent genesis, runtime source/build review, exact
code/metadata/version approval, approved coordinator/subnet, and live economic
and deployment qualification remain required.
