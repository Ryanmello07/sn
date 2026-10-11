// The settlement v2 block names STSettlement2 and its vault beside the
// unchanged v1 identities. The chain must confirm each address before use.
package validator

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// Both addresses are spelled as lowercase 0x hex, the form signed evidence
// uses, so the signed config hash names exactly one spelling of each.
type ReleaseSettlementV2Config struct {
	Settlement string `yaml:"settlement" json:"settlement"`
	Vault      string `yaml:"vault" json:"vault"`
}

// A malformed, zero or non-lowercase address is refused, never normalized.
func (self ReleaseSettlementV2Config) validate() error {
	for _, field := range []struct{ name, value string }{{"settlement", self.Settlement}, {"vault", self.Vault}} {
		if len(field.value) != 42 || !strings.HasPrefix(field.value, "0x") || field.value != strings.ToLower(field.value) ||
			!common.IsHexAddress(field.value) || common.HexToAddress(field.value) == (common.Address{}) {
			return fmt.Errorf("settlement_v2.%s must be a nonzero lowercase 0x address", field.name)
		}
	}
	return nil
}

// SettlementV2Addresses returns the STSettlement2 proxy and vault2 addresses;
// ok is false when the config has not adopted settlement v2.
func (self *ReleaseConfig) SettlementV2Addresses() (settlement, vault common.Address, ok bool) {
	if self == nil || self.SettlementV2 == nil {
		return common.Address{}, common.Address{}, false
	}
	return common.HexToAddress(self.SettlementV2.Settlement), common.HexToAddress(self.SettlementV2.Vault), true
}

// The pinned getters each return one address word.
var (
	settlementV2VaultSelector         = evmSelector("vault()")
	settlementV2V1CoordinatorSelector = evmSelector("v1Coordinator()")
	settlementV2CoordinatorSelector   = evmSelector("coordinator()")
)

// The exact-block eth_call ChainClient already performs; tests supply a fake.
type releaseSettlementV2Reader interface {
	ethCallAtHashContext(ctx context.Context, to common.Address, calldata []byte, block uint64, blockHash [32]byte) ([]byte, error)
}

// At the caller's pinned finalized block, STSettlement2 must name the
// configured vault and v1 coordinator, and the vault must name STSettlement2.
// A config without the block reads nothing.
func verifyReleaseSettlementV2Context(ctx context.Context, chain releaseSettlementV2Reader, cfg *ReleaseConfig, block uint64, blockHash [32]byte) error {
	settlement, vault, ok := cfg.SettlementV2Addresses()
	if !ok {
		return nil
	}
	for _, check := range []struct {
		name     string
		target   common.Address
		selector [4]byte
		expected common.Address
	}{
		{name: "STSettlement2.vault()", target: settlement, selector: settlementV2VaultSelector, expected: vault},
		{name: "STSettlement2.v1Coordinator()", target: settlement, selector: settlementV2V1CoordinatorSelector, expected: common.HexToAddress(cfg.Coordinator)},
		{name: "STSettlementVault2.coordinator()", target: vault, selector: settlementV2CoordinatorSelector, expected: settlement},
	} {
		output, err := chain.ethCallAtHashContext(ctx, check.target, check.selector[:], block, blockHash)
		if err != nil {
			return fmt.Errorf("settlement_v2 %s: %w", check.name, err)
		}
		if !bytes.Equal(output, common.BytesToHash(check.expected[:]).Bytes()) {
			return fmt.Errorf("settlement_v2 %s at block %d returned 0x%x, not %s", check.name, block, output, strings.ToLower(check.expected.Hex()))
		}
	}
	return nil
}
