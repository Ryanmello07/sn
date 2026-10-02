# Server settlement statistics fixture qualification — October 2

The original full model run at server `025802a50e2dc56dc56c3cb749db7375aa9f72be`
and SN `6cfc4773038fae8b1de07807eeeb4a97c32d07d3` executed all 1,345 eligible
roots: 1,334 passed, one failed, and ten optional-input tests skipped. No root
lacked a terminal result. Private PostgreSQL/Redis cleanup succeeded. The run
completed in 7,192.793 seconds; its package verdict remains failed.

`TestOpenContractStatsKeepsExactSmallPopulation` failed when its shared fixture
inserted a terminal contract without the immutable provider-usage snapshot
required by the settlement trigger (`P0001`). The production constraint was
correctly enforced. The allocated 1,024 bytes are not evidence of completed work.

The isolated test-only repair is server
`a3fc427066f037447971c90a51a80c527ee83162`, tree
`0408736564f2eabded81037067d6d2547c1acd50`, directly based on `025802a5`.
Only `model/contract_stats_model_test.go` changes. Terminal fixture rows now carry
an explicit zero-byte, zero-provider usage snapshot and terminal timestamp;
open/disputed rows retain null terminal facts. No production, module, trigger,
allocation, or statistics behavior changes.

Independent qualification reproduced the original failure in both normal and
race modes. All eight repair/helper-caller roots pass normal and race tests,
with `./model` vet exit zero, no skips, and successful private fixture cleanup:

- `TestContractStatsFixtureRetainsExplicitZeroSettlement`
- `TestCountOpenContracts`
- `TestContractHourWindowCountsAndCache`
- `TestContractHourBucketSettles`
- `TestContractHourBucketsAreSharedAcrossWindows`
- `TestOpenContractStatsCapsLargePopulation`
- `TestOpenContractStatsSamplesNewestRowsBeforeExtenderMembership`
- `TestOpenContractStatsKeepsExactSmallPopulation`

The byte-identical [original full-model receipt](server025-full-model-independent-20261002.json)
has SHA256 `4e580a88539860f503f8ac0f0d77a3978634c731a9ce5a3dd18565c7a6e92373`.
The separate [repair receipt](server-stats-fixture-independent-20261002.json)
has SHA256 `dc951c14366b4afac730ecbc8e68606c23c008c1dac4edaff0cea924d2d7ba4e`.
Raw logs, exact commands, optional-skip reasons, dependency pins and cleanup
receipts remain under
`/mnt/data/sn-testnet/sol-server025-integration-independent-20261002/`;
each receipt binds its own raw evidence hashes.

There was no patched-source full-model rerun. The ten original skips remain
unqualified. This test-only repair does not qualify the later local-storage
adoption, a new composed release, a remote physical restore, or any live
deployment. The frozen SN `258e25b4` / server `0aa1e244` release remains unchanged.
