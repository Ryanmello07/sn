//go:build linux || darwin

// The settlement v2 block is an addition: today's signed configs keep their
// hashes, and an adopted block is spelled exactly and confirmed by the chain.
package validator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"gopkg.in/yaml.v3"
)

// The block every settlement v2 fixture adopts.
var settlementV2TestBlock = ReleaseSettlementV2Config{Settlement: "0x5e77000000000000000000000000000000000001", Vault: "0x5e77000000000000000000000000000000000002"}

// Computed by the binary before settlement_v2 existed (f92aec6e), when it
// signed the fixture in testdata/settlement-v2-today: the owner-recycle
// original inside the history bundle, and today's treasury successor that
// retains it. Never regenerate these with a later binary.
const (
	settlementV2TodayConfigHash         = "0xbaaaef11cada1538e70bcfef4a4746813bfbf4a864ebd27393bc76dd0e088b78"
	settlementV2TodayOriginalConfigHash = "0x8b430b102c5f689f259c62afd6d6676cdf2fead245611c0f840bf9631d971d22"
	settlementV2TodayConfigSeal         = "0xe69e1dbeb34472dddbe9534c673324fb8d6dd98e68cdb9885b5496679170f8a4"
)

// VA-6: today's validator.yml and its history bundle, written and signed by
// the binary before settlement_v2 existed, load unchanged through the server
// staging loader. Each signature names the hash that earlier binary computed.
func TestSettlementV2TodayConfigAndHistoryHashUnchanged(t *testing.T) {
	staging := identityTestStateDir(t)
	source := filepath.Join("testdata", "settlement-v2-today")
	entries, err := os.ReadDir(source)
	if err != nil || len(entries) != 3 {
		t.Fatal("today's fixture is incomplete", err)
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(staging, entry.Name()), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(staging, "validator.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := decodeReleaseConfigBytesMode(path, raw, releaseConfigLoadMode{stagingConfigDirectory: staging})
	if err != nil {
		t.Fatal("today's signed config or history bundle no longer loads", err)
	}
	if cfg.SettlementV2 != nil || cfg.TreasuryApproval == nil || len(cfg.productionAuthorityHistory.entries) != 1 {
		t.Fatal("fixture is not today's treasury config with one original")
	}
	original := cfg.productionAuthorityHistory.entries[0].config
	current, currentErr := OwnerRecycleConfigHash(cfg)
	prior, priorErr := OwnerRecycleConfigHash(original)
	seal, sealErr := json.Marshal(cfg)
	if err := errors.Join(currentErr, priorErr, sealErr); err != nil {
		t.Fatal(err)
	}
	if attemptHex32(current) != settlementV2TodayConfigHash || attemptHex32(prior) != settlementV2TodayOriginalConfigHash ||
		attemptHex32(sha256.Sum256(seal)) != settlementV2TodayConfigSeal || bytes.Contains(seal, []byte("settlement_v2")) {
		t.Fatal("today's config, its original or its history seal hashes differently")
	}
}

// A signed schema-3 config carrying block, loaded by the producer loader.
func settlementV2TestLoad(t *testing.T, block *ReleaseSettlementV2Config) (*ReleaseConfig, error) {
	t.Helper()
	runtime := newProductionRuntimeTestFixture(t, false)
	cfg, approval := productionPendingTestClone(t, *runtime.cfg), productionPendingTestClone(t, runtime.approval)
	productionPendingTestTreasury(&cfg, &approval)
	cfg.TreasuryApproval.Approval = ReleaseEvidenceV2File{Path: filepath.Join(identityTestStateDir(t), "approval.json")}
	cfg.SettlementV2 = block
	path, _ := productionPendingTestSign(t, cfg, approval, runtime.private)
	return LoadReleaseConfig(path)
}

// VA-6: an adopted block loads with its exact spelling, appears in both wire
// forms and is the only source of the v2 addresses.
func TestSettlementV2ConfigLoadsAndNamesAddresses(t *testing.T) {
	block := settlementV2TestBlock
	cfg, err := settlementV2TestLoad(t, &block)
	if err != nil {
		t.Fatal(err)
	}
	settlement, vault, ok := cfg.SettlementV2Addresses()
	if !ok || settlement != common.HexToAddress(block.Settlement) || vault != common.HexToAddress(block.Vault) || *cfg.SettlementV2 != block {
		t.Fatal("loaded config lost its settlement_v2 addresses")
	}
	encoded, err := json.Marshal(cfg)
	if err != nil || !bytes.Contains(encoded, []byte(`"settlement_v2":{"settlement":"`+block.Settlement+`","vault":"`+block.Vault+`"}`)) {
		t.Fatal("JSON omits the adopted block", err)
	}
	document, err := yaml.Marshal(cfg)
	if err != nil || !bytes.Contains(document, []byte("settlement_v2:\n    settlement: "+block.Settlement+"\n    vault: "+block.Vault+"\n")) {
		t.Fatal("YAML omits the adopted block", err, string(document))
	}
	if _, _, ok := (&ReleaseConfig{}).SettlementV2Addresses(); ok {
		t.Fatal("a config without the block named v2 addresses")
	}
	if _, _, ok := (*ReleaseConfig)(nil).SettlementV2Addresses(); ok {
		t.Fatal("an absent config named v2 addresses")
	}
}

// VA-6: a non-lowercase, zero or malformed address is refused at load, even
// when the approval key signed it.
func TestSettlementV2ConfigRefusesMalformedAddresses(t *testing.T) {
	checksum := common.HexToAddress(settlementV2TestBlock.Settlement).Hex()
	if checksum == settlementV2TestBlock.Settlement {
		t.Fatal("control address has no checksum case")
	}
	for name, item := range map[string]struct {
		block ReleaseSettlementV2Config
		field string
	}{
		"settlement checksum case": {block: ReleaseSettlementV2Config{Settlement: checksum, Vault: settlementV2TestBlock.Vault}, field: "settlement"},
		"vault uppercase":          {block: ReleaseSettlementV2Config{Settlement: settlementV2TestBlock.Settlement, Vault: "0x" + strings.ToUpper(settlementV2TestBlock.Vault[2:])}, field: "vault"},
		"vault 0X prefix":          {block: ReleaseSettlementV2Config{Settlement: settlementV2TestBlock.Settlement, Vault: "0X" + settlementV2TestBlock.Vault[2:]}, field: "vault"},
		"settlement zero":          {block: ReleaseSettlementV2Config{Settlement: "0x" + strings.Repeat("0", 40), Vault: settlementV2TestBlock.Vault}, field: "settlement"},
		"vault zero":               {block: ReleaseSettlementV2Config{Settlement: settlementV2TestBlock.Settlement, Vault: "0x" + strings.Repeat("0", 40)}, field: "vault"},
		"settlement without 0x":    {block: ReleaseSettlementV2Config{Settlement: settlementV2TestBlock.Settlement[2:], Vault: settlementV2TestBlock.Vault}, field: "settlement"},
		"vault short":              {block: ReleaseSettlementV2Config{Settlement: settlementV2TestBlock.Settlement, Vault: settlementV2TestBlock.Vault[:41]}, field: "vault"},
		"settlement not hex":       {block: ReleaseSettlementV2Config{Settlement: settlementV2TestBlock.Settlement[:41] + "g", Vault: settlementV2TestBlock.Vault}, field: "settlement"},
		"vault absent":             {block: ReleaseSettlementV2Config{Settlement: settlementV2TestBlock.Settlement}, field: "vault"},
	} {
		block := item.block
		if _, err := settlementV2TestLoad(t, &block); err == nil || !strings.Contains(err.Error(), "settlement_v2."+item.field+" must be a nonzero lowercase 0x address") {
			t.Errorf("%s: malformed address admitted or refused for another cause: %v", name, err)
		}
	}
}

// VA-6: only the authenticated production config can adopt settlement v2.
func TestSettlementV2ConfigRequiresProductionSchema(t *testing.T) {
	cfg := validReleaseConfig(t)
	block := settlementV2TestBlock
	cfg.SettlementV2 = &block
	if _, err := LoadReleaseConfig(writeReleaseConfig(t, cfg)); err == nil || !strings.Contains(err.Error(), "settlement_v2 requires an authenticated production schema 3 config") {
		t.Fatal("a legacy config adopted settlement v2", err)
	}
}

// A getter of one target, keyed as the fake chains below answer it; the
// selector is go-ethereum's Keccak-256 of the pinned signature.
func settlementV2TestKey(target, signature string) string {
	return target + "/" + common.Bytes2Hex(crypto.Keccak256([]byte(signature))[:4])
}

// One ABI address word.
func settlementV2TestWord(address string) []byte {
	return common.LeftPadBytes(common.HexToAddress(address).Bytes(), 32)
}

// The chain's honest answers for settlementV2TestBlock under coordinator.
func settlementV2TestAnswers(coordinator string) map[string][]byte {
	settlement, vault := settlementV2TestBlock.Settlement, settlementV2TestBlock.Vault
	return map[string][]byte{
		settlementV2TestKey(settlement, "vault()"):         settlementV2TestWord(vault),
		settlementV2TestKey(settlement, "v1Coordinator()"): settlementV2TestWord(coordinator),
		settlementV2TestKey(vault, "coordinator()"):        settlementV2TestWord(settlement),
	}
}

// Records each exact-block eth_call and answers it by target and calldata.
type settlementV2TestReader struct {
	answers map[string][]byte
	err     error
	calls   []string
}

func (self *settlementV2TestReader) ethCallAtHashContext(_ context.Context, to common.Address, calldata []byte, block uint64, blockHash [32]byte) ([]byte, error) {
	if block != 77 || blockHash != ([32]byte{0x77}) {
		return nil, errors.New("settlement v2 read escaped its pinned block")
	}
	key := strings.ToLower(to.Hex()) + "/" + common.Bytes2Hex(calldata)
	self.calls = append(self.calls, key)
	if self.err != nil {
		return nil, self.err
	}
	return self.answers[key], nil
}

// VA-6: the three pinned getters agree at the pinned block; a config without
// the block reads nothing.
func TestSettlementV2IdentityCheckReadsThreePinnedGetters(t *testing.T) {
	cfg := &ReleaseConfig{Coordinator: "0x1111111111111111111111111111111111111111"}
	reader := &settlementV2TestReader{answers: settlementV2TestAnswers(cfg.Coordinator)}
	if err := verifyReleaseSettlementV2Context(t.Context(), reader, cfg, 77, [32]byte{0x77}); err != nil || len(reader.calls) != 0 {
		t.Fatal("a config without settlement_v2 read the chain", err)
	}
	block := settlementV2TestBlock
	cfg.SettlementV2 = &block
	if err := verifyReleaseSettlementV2Context(t.Context(), reader, cfg, 77, [32]byte{0x77}); err != nil {
		t.Fatal(err)
	}
	if len(reader.calls) != 3 {
		t.Fatal("identity check did not read exactly the three getters", reader.calls)
	}
	for _, call := range reader.calls {
		if _, exists := reader.answers[call]; !exists {
			t.Fatal("identity check read an unpinned getter", call)
		}
	}
}

// VA-6: each getter's mismatch is refused with its own cause, and a read
// failure is never mistaken for agreement.
func TestSettlementV2IdentityCheckRefusesEachMismatch(t *testing.T) {
	coordinator := "0x1111111111111111111111111111111111111111"
	block := settlementV2TestBlock
	cfg := &ReleaseConfig{Coordinator: coordinator, SettlementV2: &block}
	for getter, key := range map[string]string{
		"STSettlement2.vault()":            settlementV2TestKey(block.Settlement, "vault()"),
		"STSettlement2.v1Coordinator()":    settlementV2TestKey(block.Settlement, "v1Coordinator()"),
		"STSettlementVault2.coordinator()": settlementV2TestKey(block.Vault, "coordinator()"),
	} {
		reader := &settlementV2TestReader{answers: settlementV2TestAnswers(coordinator)}
		reader.answers[key] = settlementV2TestWord("0x5e77000000000000000000000000000000000003")
		if err := verifyReleaseSettlementV2Context(t.Context(), reader, cfg, 77, [32]byte{0x77}); err == nil || !strings.Contains(err.Error(), "settlement_v2 "+getter+" at block 77 returned") {
			t.Errorf("%s mismatch admitted or refused for another cause: %v", getter, err)
		}
	}
	reader := &settlementV2TestReader{answers: settlementV2TestAnswers(coordinator)}
	reader.answers[settlementV2TestKey(block.Settlement, "vault()")] = common.HexToAddress(block.Vault).Bytes()
	if err := verifyReleaseSettlementV2Context(t.Context(), reader, cfg, 77, [32]byte{0x77}); err == nil || !strings.Contains(err.Error(), "settlement_v2 STSettlement2.vault() at block 77 returned") {
		t.Fatal("an unpadded address word was admitted", err)
	}
	canary := errors.New("synthetic settlement v2 read unavailable")
	reader = &settlementV2TestReader{answers: settlementV2TestAnswers(coordinator), err: canary}
	if err := verifyReleaseSettlementV2Context(t.Context(), reader, cfg, 77, [32]byte{0x77}); !errors.Is(err, canary) || len(reader.calls) != 1 {
		t.Fatal("a failed read was not returned at the first getter", err)
	}
}
