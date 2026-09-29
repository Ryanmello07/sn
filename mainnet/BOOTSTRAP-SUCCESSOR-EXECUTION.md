# Successor execution custody and owner

The successor now has a separate signed execution domain, durable adoption and
nonce ownership, and a one-send execution state machine. Public commands can
preview, claim and recover that custody offline. **The production canonical
adapter is still unimplemented. There is no public online or submit option.**
This source change is awaiting independent behavioral qualification.

The owner can execute behind `bootstrapSuccessorExecutionChain` in deterministic
tests. That interface is a trust boundary implemented by code; a JSON report,
boolean authority flag, operator address label or passed review cannot implement
it. The test adapter supplies explicitly synthetic canonical observations.
No live Safe authority, mainnet transaction or contract installation is claimed.

## Independent execution approval

First retain the original eight successful receipts, the
[signed local preparation](BOOTSTRAP-SUCCESSOR-PREPARATION.md), and its
[exact offline Safe review](BOOTSTRAP-SUCCESSOR-SAFE-REVIEW.md). Independent
signing systems supply two Safe signatures and one signed relayer transaction.
These commands never read private keys or create a signature.

The private execution request uses schema
`urnetwork-mainnet-successor-execution-request-v1` and these fields:

| Field | Meaning |
| --- | --- |
| `safe_review_hash` | Exact reconstructed review seal. |
| `registry_directory` | Precreated private, dedicated nonce registry shared by every cooperating execution owner and signing system. |
| `owners` | Three distinct, sorted nonzero owner addresses. Current membership is still unverified. |
| `singleton` | Independently selected singleton address for the reviewed Safe release. Current code and storage remain unverified. |
| `safe_signatures` | `path` and `sha256` of exactly 130 binary signature bytes. |
| `relayer_transaction` | `path` and `sha256` of the canonical binary signed EIP-1559 transaction. |

This first profile permits two EIP-712 ECDSA signatures from the approved census,
threshold two, no modules, zero guard/module guard and zero fallback handler.
Contract signatures, approved-hash signatures, `eth_sign`, trailing signature
bytes, replacements and fee changes are unsupported. The outer envelope must
match every retained relayer field and the exact encoded `execTransaction` call.
Its chain, sender, target, nonce, value, gas, fee caps and empty access list are
authenticated by signature recovery. This admission is mathematical; it does not
prove current owner membership, relayer cutover, funding or permission to send.

```sh
sn-mainnet bootstrap-chain contract-successor-execution-preview \
  --config /private/chain.json --run-dir /private/original-custody \
  --accept-plan-hash sha256:ORIGINAL_V3_DIGEST \
  --request /private/successor-request.json \
  --safe-request /private/safe-review-request.json \
  --execution-request /private/execution-request.json
```

Preview reconstructs the original graph and completed preparation while holding
the five original shared marker locks. It verifies the pinned complete Safe
archive, imports the two pinned binary files and records both physical directory
identities. It emits `plan`, `execution_plan_hash` and hexadecimal
`execution_signing_bytes`. Those bytes are the ASCII domain
`urnetwork-mainnet-successor-execution-approval-v1`, NUL, then compact Go JSON of
the entire execution plan. This includes the exact eight adoption seals, original
attempts and reservations, proposed additive ceilings, retained review, separate
nonces, complete signed payloads, selected owners and registry identity.

The original config's independently pinned Ed25519 key approves the envelope
`{"schema":"urnetwork-mainnet-successor-execution-envelope-v1","plan":EXACT_PLAN_OBJECT,"signature_ed25519":"128_LOWERCASE_HEX"}`.
Neither original contract approval nor local preparation approval can substitute
for this domain. Importing signatures for preview grants no additional authority
to an external signing system; its own independent approval/custody rules apply.

Use `contract-successor-execution-claim` with all preview flags plus `--approval`,
`--approval-sha256` and `--accept-execution-hash`. Use
`contract-successor-execution-resume` with those exact inputs after interruption.
Exit 0 confirms the local command only. Exit 1 means unresolved custody, inputs,
cancellation or output; exit 2 means flags or execution approval are invalid;
exit 3 means the accepted execution hash differs. The original inputs, archive
and pinned signature files must remain available for every public reopen.

## Durable ownership and recovery

The original five shared locks precede an exclusive original-directory lock,
then an exclusive registry-directory lock. The original reserve marker already
fences all cooperating original action writers. Original markers and receipts
are never rewritten. One fixed `contract-successor-execution.claim` retains the
complete approval before any nonce or incremented allowance can be used.
Two immutable registry files independently fence `(chain, Safe, uint256 nonce)`
and `(chain, relayer, uint64 nonce)`. Each also binds the complete approval, signed
payload identities and original physical root. Changing one nonce cannot free
the other. A failed competing claim remains retained evidence.

The owner publishes an adoption event containing the original attempt floor and
the sum of completed original maximum envelopes, any original unexecuted
reservation, and the new outer maximum envelope. It then publishes a fixed ready
marker. Subsequent numbered events each have a full immutable `.intent` followed
by an identical `.json` record. Their seals bind the previous record and approval.
Eight original attempts plus two approved additions allow at most ten cumulative
attempts. Identical rebroadcasts keep one outer maximum liability because sender,
nonce and transaction bytes cannot change; each transport call consumes another
attempt. No receipt gas estimate or transport error reduces a reservation.

Descriptor-relative no-replace publication syncs a claimant's staged filename,
then full file contents, then the renamed directory entry. An empty synced stage
already reserves its claimant. Exact prefixes can resume under the same approval.
A partial attempt intent recovers as a consumed attempt, even if no send occurred.
A partial terminal outcome blocks sending until the exact canonical outcome can
finish it. A complete intent recovers its missing final record. Missing completed
adoption or nonce custody, changed bytes, unknown stages, sequence gaps, links,
extra hardlinks and nonprivate files refuse recovery. No files are deleted.

Copied or replaced root/registry inodes cannot inherit the approval. These are
cooperating local-filesystem fences, not cross-host, unerasable or anti-rollback
custody. Signer enforcement of the single approved registry remains a production
gate. An operator with power to rewrite or delete all custody can destroy that
evidence. A new root, restore, different registry, replacement transaction, fee
increase or later successor requires a separately implemented and qualified
migration preserving every previous seal and liability; this schema cannot do it.

## Execution machinery and remaining production code

The owner authenticates the exact ordered eight original full-record seals
through the canonical adapter, then reconciles the exact retained outer
transaction. Pending and unavailable results cannot permit another send.
Reconciliation precedes expiry and attempt checks, so exhausted or expired
custody can still retain a previously counted canonical success. An inclusion
without a preceding counted local attempt is refused; independently proved
external-send adoption/disposition remains unsupported.

Before a send, current finalized Safe proxy/singleton code, selected owners,
threshold, modules, both guards, fallback, pending Safe operations, inner nonce,
relayer confirmed/pending nonces, funding, coordinator owner and unbound evidence
slot must match. Evidence runtime and immutable getter digest must equal the
adopted original CREATE receipt. Funding also preserves original unexecuted
reservations belonging to the same relayer. After durably reserving an attempt,
the owner repeats current-state admission and checks retained physical/nonce
custody before at most one exact transport write. Every ambiguous outcome keeps
the same signatures, counters and financial liability.

Completion requires canonical native/EVM inclusion of that exact outer
transaction, the exact Safe inner-success event/digest and independently read
coordinator/evidence runtime/domain binding at inclusion. Outer status one alone
is insufficient. Outer revert is terminal for that outer transaction and keeps
the potentially live inner signature fenced. Installation never implies native
economy activation, either UR validator or the root role. Local resume reports
canonical authority unresolved even for a retained terminal event.

Before exposing a production execution command, implement and independently
qualify a concrete adapter that:

1. Reopens original signed transaction bytes and canonically reauthenticates all
   eight historical receipts/postconditions and current evidence immutable domain
   through the approved mainnet native/EVM mapping and source/runtime authority.
2. Authenticates independent Safe compiler/release review, selected live singleton
   and storage layout, complete finalized/pending Safe state, signer cutover to the
   one registry, and every original unexecuted reservation/disposition.
3. Performs canonical historical transaction search and verifies complete Safe
   receipt logs plus exact one-shot coordinator/evidence binding at inclusion.
4. Uses the independently approved owned route for one bounded submission, with
   no hidden retry, key selection, account substitution or fee replacement.
5. Wires explicit online/read/submit options only after these adapter checks and
   real contract/proxy execution, interrupted transport and finalized native/EVM
   fixture tests pass under independent normal/race qualification.

Live mainnet genesis/runtime, actual Safe/custody selection, independent rebuild
approval, owner and relayer signatures and funding are also required. They are
not substitutes for the remaining adapter implementation.
