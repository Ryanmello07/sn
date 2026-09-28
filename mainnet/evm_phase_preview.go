// Unsigned review exports exact public approval material without acquiring
// custody, loading a signing key, opening a journal or creating an RPC adapter.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/ethereum/go-ethereum/crypto"
)

const evmPhasePreviewSchema = "urnetwork-mainnet-contract-phase-preview-v1"

// The reviewable typed plan and the exact domain-separated bytes are exported
// together. Hashes identify content; only the independent signature can approve.
type evmPhasePreview struct {
	Schema                       string       `json:"schema"`
	Plan                         evmPhasePlan `json:"plan"`
	PlanHash                     string       `json:"plan_hash"`
	ApprovalPublicKey            string       `json:"approval_public_key_ed25519"`
	ApprovalSigningMessageHex    string       `json:"approval_signing_message_hex"`
	ApprovalSigningMessageSha256 string       `json:"approval_signing_message_sha256"`
	ApprovalVerified             bool         `json:"approval_verified"`
	ExecutableAction             string       `json:"executable_action"`
	ReserveAddress               string       `json:"reserve_address"`
	ExpectedReserveRuntimeHash   string       `json:"expected_reserve_runtime_hash"`
	InstallationComplete         bool         `json:"installation_complete"`
}

// An unsigned config must explicitly lack a signature; already signed or
// malformed approval material cannot be silently treated as an unsigned draft.
func loadEvmPhasePreview(ctx context.Context, path string) (evmPhasePreview, error) {
	var result evmPhasePreview
	config, err := readEvmPhaseConfig(ctx, path)
	if err != nil {
		return result, err
	}
	if config.Signature != "" {
		return result, errors.New("contract preview requires an unsigned config; use plan to verify an approval")
	}
	if err := config.validateStructure(); err != nil {
		return result, err
	}
	plan, err := buildEvmCreatePlan(ctx, config, path)
	if err != nil {
		return result, err
	}
	message, err := config.Plan.signingBytes()
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(message)
	result = evmPhasePreview{Schema: evmPhasePreviewSchema, Plan: config.Plan, PlanHash: config.Plan.hash(), ApprovalPublicKey: config.ApprovalPublicKey, ApprovalSigningMessageHex: hex.EncodeToString(message), ApprovalSigningMessageSha256: "sha256:" + hex.EncodeToString(digest[:]), ExecutableAction: "reserve-create", ReserveAddress: plan.Address.Hex(), ExpectedReserveRuntimeHash: crypto.Keccak256Hash(plan.Runtime).Hex()}
	return result, ctx.Err()
}
