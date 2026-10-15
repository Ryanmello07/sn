package commitmentoracle

// Root-cause tests for the SN25 validator mirror incident (2026-10-15): a
// one-field Sha256 commitment does not say whose it is, so the oracle must
// identify a fleet commitment positively by its own format, one Raw40 field of
// "urfleet1" and the hash, and mirror nothing else. The decision must not
// depend on a validator permit, the shape of the writing extrinsic, or any
// read that can fail. setRole, setBody and failRoles describe chain state the
// oracle must not depend on (fake_test.go).

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

const (
	// "urfleet1" in ASCII.
	fleetTagHex = "7572666c65657431"
	// The exact mainnet bytes of the 7906/7916 miner's commitment: one Sha256
	// field (variant 0x83), no tag. A legacy fleet commitment.
	legacyGoldenRegistrationHex = "0000000000000000" + "614b8d00" + "04" + "83" + goldenCommitmentHex
)

// A registration at block with the given CommitmentInfo hex.
func formatRegistration(block uint64, infoHex string) (string, string) {
	last := hex.EncodeToString(binary.LittleEndian.AppendUint32(nil, uint32(block)))
	return "0000000000000000" + last + infoHex, last
}

func taggedInfoHex(hash string) string {
	return "04" + "29" + fleetTagHex + hash
}

// The incident's finalized head with no commitments yet.
func formatChain() *fakeNative {
	native := newFakeNative(nativeHead{number: incidentFinalized, hash: goldenHeadHash()})
	return native
}

// Puts hotkey's registration, written at block (hash blockHash), at the head.
func formatPut(t *testing.T, native *fakeNative, hotkey [32]byte, block uint64, blockHash [32]byte, infoHex string) {
	registration, last := formatRegistration(block, infoHex)
	native.block(block, blockHash)
	native.put(goldenHeadHash(), hotkey, mustHex(t, registration), mustHex(t, last))
}

// Commitments.set_commitment(25, info) as the SN25 runtime encodes it.
func formatSetCommitmentCall(t *testing.T, infoHex string) []byte {
	return append([]byte{18, 0, 25, 0}, mustHex(t, infoHex)...)
}

// Utility.batch_all([calls...]).
func formatBatchAll(calls ...[]byte) []byte {
	batch := []byte{11, 2, byte(len(calls) << 2)}
	for _, call := range calls {
		batch = append(batch, call...)
	}
	return batch
}

func assertNothingMirrored(t *testing.T, harness *testHarness) {
	t.Helper()
	if len(harness.evm.broadcasts) != 0 || len(harness.evm.mined) != 0 {
		t.Fatalf("broadcast %d, mined %d", len(harness.evm.broadcasts), len(harness.evm.mined))
	}
	if harness.oracle.metrics.pending != 0 || harness.oracle.metrics.fleet != 0 {
		t.Fatalf("pending %d fleet %d", harness.oracle.metrics.pending, harness.oracle.metrics.fleet)
	}
}

func assertMirrored(t *testing.T, harness *testHarness, hotkey [32]byte, hash string, block uint64, blockHash [32]byte) {
	t.Helper()
	want := mirroredRecord{commitmentHash: mustHash(t, hash), finalizedBlockHash: blockHash, finalizedBlock: block}
	if record := harness.evm.recordAt(hotkey, harness.evm.latest); record != want {
		t.Fatalf("hotkey 0x%x record %+v, want %+v", hotkey, record, want)
	}
}

// The incident: the validator's source commitment (one Sha256 field, written
// by Utility.batch_all with its CRv4 commit) is never mirrored, with or
// without a validator permit.
func TestFormatNeverMirrorsIncidentValidatorSourceCommitment(t *testing.T) {
	for _, permit := range []bool{true, false} {
		native := formatChain()
		validator := mustHash(t, validatorHotkeyHex)
		native.block(validatorBlock, mustHash(t, validatorBlockHashHex))
		native.put(goldenHeadHash(), validator, mustHex(t, validatorRegistrationHex), mustHex(t, validatorLastHex))
		native.setRole(validator, validatorUid, permit)
		native.setBody(mustHash(t, validatorBlockHashHex), mustHex(t, validatorTimestampExtrinsicHex), mustHex(t, validatorSourceExtrinsicHex))
		harness := newIncidentHarness(t, native, false)
		if err := harness.oracle.cycle(context.Background()); err != nil {
			t.Fatalf("permit %v: %v", permit, err)
		}
		assertNothingMirrored(t, harness)
	}
}

// The lapse case: the permit is absent (lapsed, or the owner UID steering
// without one) and the one-field Sha256 commitment was written standalone,
// so no permit or extrinsic-shape heuristic can exclude it. The format alone
// keeps it out.
func TestFormatNeverMirrorsSha256CommitmentWithoutPermit(t *testing.T) {
	native := formatChain()
	validator := mustHash(t, validatorHotkeyHex)
	blockHash := mustHash(t, validatorBlockHashHex)
	formatPut(t, native, validator, validatorBlock, blockHash, "04"+"83"+validatorCommitmentHex)
	native.setRole(validator, validatorUid, false)
	native.setBody(blockHash, mustHex(t, validatorTimestampExtrinsicHex), synthSignedExtrinsic(validator, formatSetCommitmentCall(t, "04"+"83"+validatorCommitmentHex)))
	harness := newIncidentHarness(t, native, false)
	if err := harness.oracle.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertNothingMirrored(t, harness)
	if record := harness.evm.recordAt(validator, harness.evm.latest); record.finalizedBlock != 0 {
		t.Fatalf("validator record %+v", record)
	}
}

// A legacy untagged fleet commitment, the miner's real mainnet bytes with its
// real standalone write, is not mirrored: the oracle accepts no legacy
// format, so a Sha256 commitment can never be mirrored. The miner republishes.
func TestFormatNeverMirrorsLegacyFleetCommitment(t *testing.T) {
	native := formatChain()
	miner := mustHash(t, goldenHotkeyHex)
	native.block(goldenBlock, mustHash(t, goldenBlockHashHex))
	native.put(goldenHeadHash(), miner, mustHex(t, legacyGoldenRegistrationHex), mustHex(t, goldenLastHex))
	native.setRole(miner, goldenUid, false)
	native.setBody(mustHash(t, goldenBlockHashHex), mustHex(t, goldenTimestampExtrinsicHex), mustHex(t, goldenFleetExtrinsicHex))
	harness := newIncidentHarness(t, native, false)
	if err := harness.oracle.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertNothingMirrored(t, harness)
}

// A tagged fleet commitment is mirrored whoever wrote it and however: from a
// hotkey that holds a validator permit, from a Utility.batch_all, and from a
// proxy's extrinsic signed by another account.
func TestFormatMirrorsTaggedFleetFromPermitHotkeyAndBatchedWrites(t *testing.T) {
	native := formatChain()
	type fleet struct {
		hotkey    [32]byte
		hash      string
		block     uint64
		blockHash [32]byte
	}
	permitHolder := fleet{hotkey: mustHash(t, validatorHotkeyHex), hash: strings.Repeat("a1", 32), block: 9286001, blockHash: [32]byte{0xb1}}
	batched := fleet{hotkey: [32]byte{0x62}, hash: strings.Repeat("b2", 32), block: 9286002, blockHash: [32]byte{0xb2}}
	proxied := fleet{hotkey: [32]byte{0x63}, hash: strings.Repeat("c3", 32), block: 9286003, blockHash: [32]byte{0xb3}}
	for _, f := range []fleet{permitHolder, batched, proxied} {
		formatPut(t, native, f.hotkey, f.block, f.blockHash, taggedInfoHex(f.hash))
	}
	native.setRole(permitHolder.hotkey, validatorUid, true)
	native.setBody(permitHolder.blockHash, synthSignedExtrinsic(permitHolder.hotkey, formatSetCommitmentCall(t, taggedInfoHex(permitHolder.hash))))
	native.setRole(batched.hotkey, 40, false)
	native.setBody(batched.blockHash, synthSignedExtrinsic(batched.hotkey, formatBatchAll(formatSetCommitmentCall(t, taggedInfoHex(batched.hash)), []byte{0, 7, 0})))
	native.setRole(proxied.hotkey, 41, false)
	// Proxy.proxy(real, None, call), signed by the proxy account.
	proxyCall := append(append([]byte{0x10, 0x00, 0x00}, proxied.hotkey[:]...), 0x00)
	native.setBody(proxied.blockHash, synthSignedExtrinsic([32]byte{0x99}, append(proxyCall, formatSetCommitmentCall(t, taggedInfoHex(proxied.hash))...)))
	harness := newIncidentHarness(t, native, false)
	if err := harness.oracle.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, f := range []fleet{permitHolder, batched, proxied} {
		assertMirrored(t, harness, f.hotkey, f.hash, f.block, f.blockHash)
	}
	if len(harness.evm.mined) != 3 {
		t.Fatalf("mined %d", len(harness.evm.mined))
	}
}

// No per-hotkey read gates mirroring, so a role read that would fail cannot
// stop the tagged fleets.
func TestFormatRoleReadFailureDoesNotStopTaggedFleets(t *testing.T) {
	native := formatChain()
	hotkey := [32]byte{0x64}
	hash := strings.Repeat("d4", 32)
	formatPut(t, native, hotkey, 9286004, [32]byte{0xb4}, taggedInfoHex(hash))
	native.setRole(hotkey, 42, false)
	native.setBody([32]byte{0xb4}, synthSignedExtrinsic(hotkey, formatSetCommitmentCall(t, taggedInfoHex(hash))))
	native.failRoles(errors.New("ValidatorPermit read timed out"))
	harness := newIncidentHarness(t, native, false)
	if err := harness.oracle.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertMirrored(t, harness, hotkey, hash, 9286004, [32]byte{0xb4})
}

// Malformed tags, variants and lengths are never mirrored; the one valid
// tagged commitment among them is.
func TestFormatRejectsMalformedTagsAndLengths(t *testing.T) {
	native := formatChain()
	hash := strings.Repeat("e5", 32)
	malformed := map[string]string{
		"other tag":             "04" + "29" + "7572666c65657432" + hash,
		"upper-case tag":        "04" + "29" + "5552464c45455431" + hash,
		"tag without hash":      "04" + "09" + fleetTagHex,
		"raw32 hash, no tag":    "04" + "21" + hash,
		"raw39 short hash":      "04" + "28" + fleetTagHex + hash[:62],
		"raw41 extra byte":      "04" + "2a" + fleetTagHex + hash + "00",
		"raw40 trailing byte":   "04" + "29" + fleetTagHex + hash + "00",
		"raw40 truncated":       "04" + "29" + fleetTagHex + hash[:62],
		"two tagged fields":     "08" + "29" + fleetTagHex + hash + "29" + fleetTagHex + hash,
		"sha256 then tagged":    "08" + "83" + hash + "29" + fleetTagHex + hash,
		"zero hash":             "04" + "29" + fleetTagHex + strings.Repeat("00", 32),
		"tag after hash":        "04" + "29" + hash + fleetTagHex,
		"one-field sha256 only": "04" + "83" + hash,
	}
	index := byte(0)
	for _, info := range malformed {
		index += 1
		hotkey := [32]byte{0x70, index}
		formatPut(t, native, hotkey, 9286100+uint64(index), [32]byte{0xc0, index}, info)
		native.setRole(hotkey, uint16(100+index), false)
		native.setBody([32]byte{0xc0, index}, synthSignedExtrinsic(hotkey, formatSetCommitmentCall(t, info)))
	}
	valid := [32]byte{0x7f}
	formatPut(t, native, valid, 9286200, [32]byte{0xcf}, taggedInfoHex(hash))
	native.setRole(valid, 200, false)
	native.setBody([32]byte{0xcf}, synthSignedExtrinsic(valid, formatSetCommitmentCall(t, taggedInfoHex(hash))))
	harness := newIncidentHarness(t, native, false)
	if err := harness.oracle.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertMirrored(t, harness, valid, hash, 9286200, [32]byte{0xcf})
	if len(harness.evm.mined) != 1 || harness.oracle.metrics.fleet != 1 {
		t.Fatalf("mined %d fleet %d", len(harness.evm.mined), harness.oracle.metrics.fleet)
	}
	// A registration whose u64/u32 prefix is cut short is not fleet format.
	for name, raw := range map[string]string{
		"no deposit": "614b8d00" + taggedInfoHex(hash),
		"short":      "00000000000000" + "614b8d00" + taggedInfoHex(hash),
		"long":       "000000000000000000" + "614b8d00" + taggedInfoHex(hash),
	} {
		if _, err := parseFleetCommitment(rawCommitment{hotkey: valid, raw: mustHex(t, raw)}); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}
