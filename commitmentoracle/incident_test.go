package commitmentoracle

// The 2026-10-15 incident: a manual mirror of the SN25 validator's own source
// commitment (UID 1, 5CyQFykVpfa9xgZVGgBsiNUpjbo1EihBTSkPvDC1FZbL1wxG, tx
// 0x4bb7941d1f344609b60f1ab0edb6548b18a9ee53cef1b5c7f7556f3886d222b5) halted
// its steering, because a validator source commitment had the same one-field
// Sha256 storage format as a fleet commitment. These fixtures are public
// finalized mainnet values read on 2026-10-15 at native finalized 9,286,811
// (lite and archive agreed): storage and the exact extrinsic bytes of each
// commitment block. format_test.go uses them.

import (
	"context"
	"testing"
	"time"
)

const (
	// The 7906/7916 miner's UID; it holds no validator permit.
	goldenUid = uint16(145)
	// Block 9,259,873 extrinsic 11: the miner's standalone signed
	// Commitments.set_commitment (call 0x1200, netuid 25, one Sha256 field).
	goldenFleetExtrinsicHex = "39028400ca2b48b25720aef399910675153afb697370e8983a45856fb220bb8a83a08d1d01921e05452556d68b3bdc3921265daf4d94a378c74938adcfb323a7e75793d842cd816a50c399e94eb6952c9eecf43a7c47de948f92c5647a35dc34b2651f5f82e605040000120019000483f8cfc29663a79e23c8a41fe6a2ca21b7bc7f8a0463526607140ce2aa5766defb"
	// Block 9,259,873 extrinsic 0, the bare v5 timestamp inherent.
	goldenTimestampExtrinsicHex = "280502000bc004e62aa101"

	// The SN25 validator, UID 1, with a validator permit.
	validatorHotkeyHex     = "2830c416368bc539694600e9ed2cb276764452814c591d6740cc737eb673c67b"
	validatorUid           = uint16(1)
	validatorCommitmentHex = "5877e91464305ea2f2c1299ff1bce3f2117d9e24f30a7a50a1bc9edd3ae79030"
	validatorBlock         = uint64(9286328)
	validatorBlockHashHex  = "74fc6cf9810ad4de80e9978d8930d4ff3ee3d3135187a1d7a908b6ba3644bfc0"
	// Block 9,286,328 encoded as a little-endian u32.
	validatorRegistrationHex = "0000000000000000" + "b8b28d00" + "04" + "83" + validatorCommitmentHex
	validatorLastHex         = "b8b28d00"
	// Block 9,286,328 extrinsic 0, the bare v5 timestamp inherent.
	validatorTimestampExtrinsicHex = "280502000bc041d23da101"
	// The native finalized head of the incident reads.
	incidentFinalized = uint64(9286811)
	// Block 9,286,328 extrinsic 12, the validator source write: Utility.batch_all
	// (0x0b02, two calls) of Commitments.set_commitment and SubtensorModule
	// commit_timelocked (0x0771), signed by the validator hotkey.
	validatorSourceExtrinsicHex = "" +
		"310784002830c416368bc539694600e9ed2cb276764452814c591d6740cc737eb673c67b01485bfd7b4de4455d0651502afe050529cd6a33669348958f2140d6" +
		"1caf87f27700167023980bb0b5c98de08299de71467e41c8c7211680d2a63a075fe1c81c8600210400000b02081200190004835877e91464305ea2f2c1299ff1" +
		"bce3f2117d9e24f30a7a50a1bc9edd3ae7903007711900ad04b3222556261c88849d20d29f6912c862e837afa26645be15aded38c4265743a59bf00cf6d8a4c4" +
		"f0a04460418fc942e20e0bda567d42db40d155ab6565e0482b65e2d8b0a9db4bdf943cee3947b06d190d661b6681fdc92b7adb7f5c640687ae20000000000000" +
		"00a412460b03366cfb0b4b66b25f355b569a8496fd594cdab993e490a7cf623f4720000000000000006b0f9b7c22dd308e4fcb7c9a9e58ab67f6e9811432427b" +
		"c66da355ff0c0f1c7d6300000000000000470000000000000055fd74177a944a4f441c3e4caea3cb576e9b11683f9bde7ca73ba90d5444f8367e2c312b6b304b" +
		"2597d412ee892408b48e74b3de181e554ff88261facc063c2492f235eb141efa0c0000000000000094acaafc10430f1628bc6f0608000000000000004145535f" +
		"47434d5f79c1f801000000000400"
)

func newIncidentHarness(t *testing.T, native *fakeNative, dryRun bool) *testHarness {
	settings := testSettings(t.TempDir())
	settings.DryRun = dryRun
	key := testOracleKey(t)
	address := keyAddress(key)
	evm := newFakeEvm(t, settings.Coordinator, address, incidentFinalized)
	harness := newTestHarness(t, native, evm, settings, key, address, &testClock{now: time.Unix(1760500000, 0)}, &testLog{})
	harness.oracle.sleep = func(ctx context.Context, duration time.Duration) error {
		harness.clock.now = harness.clock.now.Add(duration)
		harness.evm.mine()
		harness.evm.finalize()
		return nil
	}
	return harness
}
