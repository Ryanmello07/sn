# Durable storage composition checkpoint — October 2, 2026

The inventory primitives and public inventory commands now have independent scoped
receipts. The composed production candidate is still **unqualified**: actual
public root observation and successor execution exposed the failures below.
The frozen release `258e25b4` / server `0aa1e244` and all earlier source receipts
retain their original scopes. No live service, mount, signer, broadcast,
deployment or activation is authorized by this checkpoint.

## Newly retained independent receipts

| Exact source | Independent result | Scope and limitation |
| --- | --- | --- |
| Connect `0a5cda0ebe78f6200c4cff6fd3d1aa172a2b172c` | [Inventory-v3 receipt](durable-inventory-v3-independent-20261002.json), SHA-256 `ea6af484fafd960cbd8ad28cf322cce905fd9b3a5f40a01a8a3ac397395a5cb9`; 57 normal and 57 race roots, vet passes | Bounded approved owner-attribute inventory and report-only verification. Three old-source controls fail in both modes. It does not qualify a CLI, adopter or restore. |
| SN `7fb6b6a1bd52cf243fcbd717f0f48a9cf86027ab` with Connect `0a5cda0e` | [Public inventory CLI receipt](durable-inventory-cli-independent-20261002.json), SHA-256 `0ba0a3e0edcbc0f358c850594a4d6d3fc8b8d7023700a9c1cfafb04ffabb9a01`; nine normal and nine race roots, three-package vet passes | Actual daemon and explicitly selected owner-local inventory/verify dispatch, bounded inventory input and short-output refusal. Three old-source controls fail in both modes. The rebound control first fails because the baseline lacks v3 bounds; this is not an isolated proof of rebound behavior. |
| Connect `71df099caf3c3a5b2d2512a761b5ad51abd213e6` | [Constructor observation receipt](durable-constructor-independent-20261002.json), SHA-256 `bcf6105e7d0a16b97139c95002dbcadd218da6ac0f3960ae5eeb2c41d8779aae`; 59 normal and 59 race roots, vet passes | Includes the earlier 57 roots. Initial filesystem observation EIO/EMFILE refuses admission as retryable unavailability; invalid-descriptor caller errors remain distinct. The old-source observation control fails and the caller-error control passes in both modes. |

Connect `0a5cda0e` and its constructor successor `71df099c` are merged upstream.
The current composed SN candidate still selects `0a5cda0e`; standalone server
`005a9066` selects the earlier `6cd720cf`. Neither consumer inherits the later
constructor fix merely because it is merged in Connect. A separate module-only
consumer successor and actual startup transient-observation control are pending.
The independent receipts above are copied byte-for-byte; their raw logs and
referenced source/evidence files remain in the retained external workspaces.

## Actual public root observer failures

The source join at composed SN `f12c190757f5e88336257dd18b82707c02584e6f`
and server `10a8f4d8ab73822b4c796f9035486f0507e09bd2` verifies the changed
file bytes against the qualified component sources and preserves unrelated
upstream source. It does not establish composition correctness. Its bounded
mainnet attempt selected 22 roots: 21 passed and the new passive-bootstrap to
monitor control failed. The earlier wrapper attempt failed before tests because
it inherited `GO111MODULE=off`; both attempts remain retained.

Test-only SN `ea2bb37f0d43e76d506f69c6f2ad8f95d91993a8` adds two further
controls without changing that production source. All three fail in both normal
and race modes, with no skips:

- `TestDurableCompositionPassiveBootstrapStartsRetainedMonitor`: after the
  public passive service records a completed monitor checkpoint, removing only
  that checkpoint allows a later public run to recreate it.
- `TestDurableCompositionRootPersistentEntrypointsRequireDeclaration`: direct
  persistent `root-monitor` opens checkpoint/metrics custody without the required
  declaration. The checkpoint and metrics calls in `root_command.go` omit the
  retained context and select their historical constructor path.
- `TestDurableCompositionPassivePreparationRequiresRetainedHead`: the passive
  preparation reader accepts the original marker and record after the retained
  snapshot-head attribute is removed. Its manual read bypasses the snapshot
  custody admission.

The two additional controls were added after the first 22-root attempt; they
were not omitted from that attempt's historical source. A census of the four
optional-context custody constructors found every other non-test caller passes
or forwards context. The pending fix retains the context at both observer
constructors and retains a shared, read-only snapshot guard for the passive
service lifetime. Temporary observation unavailability must retry the same
owner within a finite policy, while proven custody loss stops the affected
role. Valid-declaration checkpoint loss, runtime preparation loss, cancellation
and shared-reader composition still need the successor's exact-source gate.

## Separate successor execution admission failure

Fixture-only SN `22d4ee3ed274b5ad4ecc14a30c656414809d8cc7`, tree
`ad057cbffda2cdb2af07fee76b6bf1258c5fba18`, leaves the independently tested
`eb0abe22` production files unchanged. Its terminal 75-root normal scope has
25 passes and 50 failures, with no skips and package duration 802.917 seconds.
Of those failures, 48 encounter `durable volume owner does not permit writes`.
Exclusive successor execution borrows a preparation reader whose durable
owner was always opened read-only, so publication cannot pass write admission.
The remaining two failures are outdated error-string assertions at
`TestBootstrapSuccessorPreparationFencesRootReplacementDuringPublication` and
`TestBootstrapSuccessorPreparationReaderRejectsUnsafeOrReplacedCustody`; the
actual error correctly reports physical identity loss.

The separate `f3c8a618` successor changes three production files to select
read/write ownership only for exclusive execution and recheck admission before
claim/payload/completion publication. Its eight new controls, including public
shared-reader refusal, joined resume and no-send assertions, pass the initial
author normal gate (78.471 seconds); its remaining selected normal/race and
causal gates are pending. This partial result does not qualify the complete
successor. That source is separate
from the root observer/passive-preparation fix. The original
[111-root adopter receipt](durable-adopter-qualification-20261002.md) remains
valid for its selected scope and does not prove this newly exercised route.

## Retained evidence and remaining work

[The causal census](durable-composition-open-causes-20261002.json), SHA-256
`5b15d71ab534b659a8acf55b5fd30c34368ae82691fa4de02edb6e5125ec3689`,
binds exact baseline commits/trees, test-only source comparisons, raw log
paths/hashes, test names/counts and failing exits. This is a readback of retained
author controls, not an independent test rerun. The raw root logs remain under
`/mnt/data/sn-testnet/mainnet-durable-volume-20261002/integration/evidence/`;
the successor logs remain under
`/mnt/data/sn-testnet/mainnet-storage-owner-adoption-20261002/fixture-successor/evidence/`.

The next qualification must combine the fixed public observer routes with all
eight successor admission controls and the affected CLI/lifecycle neighbors.
It must preserve the earlier failed attempts, then obtain a separate independent
composition receipt. Unchanged successful 201/114/111-root component scopes are
not summed into a full-package result or rerun merely to enlarge a count.

Production [offline preparation](durable-storage-preparation-design-20261002.md),
immutable-member census, bounded capacity/rotation policy, actual restore and a
new exact composed release remain open. PH-09 is one part of the full mainnet
goal: remaining runtime/retry/cache paths, domain monitors and bounded repair,
native proof/fee attribution, all-role qualification and live authority gates
must still be handled. Inventory output remains report-only and cannot
authorize restart, rebind, signing or deployment.
