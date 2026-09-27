// Mainnet production authenticates fresh chain identity and one exact finalized
// runtime window before binding its private signing or historical read view.
package validator

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/urfoundation/sn/crv4"
)

// A caller must own this Chain view. The original binding remains unchanged
// until the complete production-purpose and closing canonical checks succeed.
func authenticateOwnerRecycleProductionRuntimeAtContext(ctx context.Context, native *crv4.Chain, cfg *ReleaseConfig, block types.Hash) error {
	if ctx == nil {
		return errors.New("production runtime caller context is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	artifact, number, err := authenticateOwnerRecycleProductionArtifactAtContext(ctx, native, cfg, block, false)
	if err != nil {
		return err
	}
	view := *native
	if err := view.BindValidatorProducerRuntimeArtifactContext(ctx, artifact); err != nil {
		return err
	}
	var canonical types.Hash
	if err := native.API.Client.CallContext(ctx, &canonical, "chain_getBlockHash", number); err != nil {
		return err
	}
	if canonical != block {
		return errors.New("production runtime canonical block changed during purpose authentication")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	*native = view
	return nil
}

// Historical authority is a finite list of original exact artifacts. This
// helper returns authenticated evidence; only the caller selects its bind purpose.
func authenticateOwnerRecycleProductionArtifactAtContext(ctx context.Context, native *crv4.Chain, cfg *ReleaseConfig, block types.Hash, historical bool) (crv4.AuthenticatedRuntimeArtifact, uint64, error) {
	empty := crv4.AuthenticatedRuntimeArtifact{}
	if err := validateReleaseProductionRuntimeHistory(cfg); err != nil {
		return empty, 0, err
	}
	if ctx == nil || native == nil || native.API == nil || native.API.Client == nil || block == (types.Hash{}) ||
		native.ProvisionalRuntimeCompatibilityEnabled() || !slices.Contains(cfg.Substrate, native.API.Client.URL()) {
		return empty, 0, errors.New("production runtime requires an approved non-provisional connection and exact block")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return empty, 0, err
	}
	approved, err := ownerRecycleProductionApproval(cfg)
	if err != nil {
		return empty, 0, err
	}
	expectedGenesis, err := types.NewHashFromHexString(cfg.GenesisHash)
	if err != nil || native.GenesisHash != expectedGenesis {
		return empty, 0, errors.New("production runtime connection genesis differs from signed authority")
	}
	call := native.API.Client.CallContext
	var nativeChain, evmChainId string
	var genesis types.Hash
	if err := call(ctx, &nativeChain, "system_chain"); err != nil {
		return empty, 0, err
	}
	if err := call(ctx, &genesis, "chain_getBlockHash", uint64(0)); err != nil {
		return empty, 0, err
	}
	if err := call(ctx, &evmChainId, "eth_chainId"); err != nil {
		return empty, 0, err
	}
	if nativeChain != approved.Approval.NativeChain || genesis != expectedGenesis || evmChainId != "0x3c4" {
		return empty, 0, errors.New("production runtime fresh native chain, genesis or EVM964 differs")
	}
	finalized, err := crv4.FinalizedHeadContext(ctx, native)
	if err != nil {
		return empty, 0, err
	}
	finalizedHeader, err := native.HeaderAtContext(ctx, finalized)
	if err != nil {
		return empty, 0, err
	}
	header := finalizedHeader
	if block != finalized {
		header, err = native.HeaderAtContext(ctx, block)
		if err != nil {
			return empty, 0, err
		}
	}
	number := uint64(header.Number)
	if number == 0 || number > uint64(finalizedHeader.Number) {
		return empty, 0, errors.New("production runtime block is not finalized")
	}
	expected, err := releaseProductionRuntimeAt(cfg, number, historical)
	if err != nil {
		return empty, 0, err
	}
	checkCanonical := func(hash types.Hash, height uint64) error {
		var canonical types.Hash
		if err := call(ctx, &canonical, "chain_getBlockHash", height); err != nil {
			return err
		}
		if canonical != hash {
			return errors.New("production runtime block is not canonical at its original height")
		}
		return ctx.Err()
	}
	if err := checkCanonical(finalized, uint64(finalizedHeader.Number)); err != nil {
		return empty, 0, err
	}
	if err := checkCanonical(block, number); err != nil {
		return empty, 0, err
	}
	artifact, err := crv4.AuthenticateRuntimeArtifactAtContext(ctx, native, block, expected)
	if err != nil {
		return empty, 0, fmt.Errorf("production runtime at %s: %w", block.Hex(), err)
	}
	if err := crv4.ValidateValidatorProducerRuntimeArtifactContext(ctx, native, artifact); err != nil {
		return empty, 0, err
	}
	if err := checkCanonical(block, number); err != nil {
		return empty, 0, err
	}
	artifact.GenesisHash = genesis
	return artifact, number, nil
}
