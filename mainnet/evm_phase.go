// One independent approval seals a bounded installation graph. This first
// executor admits only its reserve CREATE; later actions remain unexecuted.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const evmPhaseSchema = "urnetwork-mainnet-contract-phase-v1"
const evmPhaseApprovalSchema = "urnetwork-mainnet-contract-phase-approval-v1"
const evmPhaseConfigSchema = "urnetwork-mainnet-contract-phase-config-v1"
const evmCreateStateFile = "reserve-create.json"

// All native transaction fields are explicit; no live quote may replace an
// approved nonce, fee, target, value or byte of calldata during recovery.
type evmPhaseAction struct {
	Id        string          `json:"id"`
	Sender    common.Address  `json:"sender"`
	Nonce     uint64          `json:"nonce"`
	To        *common.Address `json:"to"`
	Data      string          `json:"data"`
	ValueWei  string          `json:"value_wei"`
	Gas       uint64          `json:"gas"`
	FeeCapWei string          `json:"fee_cap_wei"`
	TipCapWei string          `json:"tip_cap_wei"`
}

// The graph can be approved once for all prepared actions. This implementation
// never treats an unimplemented descendant as completed or executable.
type evmPhasePlan struct {
	Schema              string               `json:"schema"`
	DeploymentId        string               `json:"deployment_id"`
	Network             planNetwork          `json:"network"`
	Runtime             rootReceiptProfile   `json:"runtime"`
	Route               ownedSubmissionRoute `json:"route"`
	RunDirectory        string               `json:"run_directory"`
	Artifacts           planFileReference    `json:"artifacts"`
	SourceLockHash      string               `json:"source_lock_hash"`
	CustodyId           string               `json:"custody_id"`
	CustodyFenceHash    string               `json:"custody_fence_hash"`
	CutoverEvidenceHash string               `json:"cutover_evidence_hash"`
	StartNativeNumber   uint64               `json:"start_native_number"`
	StartNativeHash     string               `json:"start_native_hash"`
	ValidThroughNative  uint64               `json:"valid_through_native"`
	MaximumAttempts     uint8                `json:"maximum_attempts"`
	MaximumTotalWei     string               `json:"maximum_total_wei"`
	Netuid              uint16               `json:"netuid"`
	ReserveHotkey       string               `json:"reserve_hotkey"`
	Actions             []evmPhaseAction     `json:"actions"`
}

// Trust is independently provisioned, never learned from retained state.
// The approval signs the whole graph, not an individual resume invocation.
type evmPhaseConfig struct {
	Schema            string       `json:"schema"`
	ApprovalPublicKey string       `json:"approval_public_key_ed25519"`
	Plan              evmPhasePlan `json:"plan"`
	Signature         string       `json:"approval_signature_ed25519"`
}

// A canonical unsigned integer has no signs, whitespace or alternate spelling.
func evmWei(encoded string) (*big.Int, error) {
	value, ok := new(big.Int).SetString(encoded, 10)
	if !ok || value.Sign() < 0 || value.BitLen() > 256 || value.String() != encoded {
		return nil, errors.New("EVM amount is not a canonical uint256")
	}
	return value, nil
}

// Unsigned envelope construction is the sole transaction-format policy. Only
// EIP-1559 with an empty access list is admitted by this bounded first version.
func (self evmPhaseAction) unsigned() (*types.Transaction, error) {
	value, e1 := evmWei(self.ValueWei)
	fee, e2 := evmWei(self.FeeCapWei)
	tip, e3 := evmWei(self.TipCapWei)
	data, e4 := rootReceiptHex(self.Data, 64*1024)
	if err := errors.Join(e1, e2, e3, e4); err != nil {
		return nil, err
	}
	if self.Sender == (common.Address{}) || self.Gas < 21000 || self.Gas > 100_000_000 || fee.Sign() == 0 || tip.Cmp(fee) > 0 || len(data) == 0 || self.Data != "0x"+hex.EncodeToString(data) || self.To != nil && *self.To == (common.Address{}) {
		return nil, errors.New("EVM action identity or envelope is invalid")
	}
	return types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(mainnetEvmChainId), Nonce: self.Nonce, To: self.To, Data: data, Value: value, Gas: self.Gas, GasFeeCap: fee, GasTipCap: tip, AccessList: types.AccessList{}}), nil
}

// Public signed bytes are decoded, sender-recovered and compared to every
// approved envelope field. Alternate valid signatures cannot replace custody.
func (self evmPhaseAction) signed(raw []byte) (*types.Transaction, error) {
	if len(raw) == 0 || len(raw) > 128*1024 {
		return nil, errors.New("signed EVM transaction exceeds its bound")
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return nil, err
	}
	canonical, err := tx.MarshalBinary()
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, errors.New("signed EVM bytes are not canonical")
	}
	expected, err := self.unsigned()
	if err != nil {
		return nil, err
	}
	signer := types.LatestSignerForChainID(big.NewInt(mainnetEvmChainId))
	from, err := types.Sender(signer, &tx)
	if err != nil || from != self.Sender || !tx.Protected() || tx.Type() != types.DynamicFeeTxType || tx.ChainId().Cmp(big.NewInt(mainnetEvmChainId)) != 0 || signer.Hash(&tx) != signer.Hash(expected) {
		return nil, errors.Join(errors.New("signed EVM transaction differs from exact approved envelope"), err)
	}
	return &tx, nil
}

// Typed JSON and a dedicated domain prevent native approval reuse.
func (self evmPhasePlan) signingBytes() ([]byte, error) {
	raw, err := json.Marshal(self)
	return append([]byte(evmPhaseApprovalSchema+"\x00"), raw...), err
}

// The plan hash identifies all graph authority and does not depend on a file's
// whitespace or an imported signature. Raw artifact identity is separately pinned.
func (self evmPhasePlan) hash() string {
	raw, _ := self.signingBytes()
	return rootObjectHash(struct {
		Domain string
		Bytes  []byte
	}{Domain: evmPhaseSchema, Bytes: raw})
}

// The graph is finite and preserves its original sender/nonce and total cost
// reservations. Known descendants remain sealed but cannot execute in this slice.
func (self evmPhaseConfig) validate() error {
	p := self.Plan
	if self.Schema != evmPhaseConfigSchema || p.Schema != evmPhaseSchema || !planLabel(p.DeploymentId) || p.Network.NativeChain == "" || p.Network.EvmChainId != mainnetEvmChainId || !rootCanonicalHash(p.Network.GenesisHash) || p.Netuid != 25 || !rootCanonicalHash(p.ReserveHotkey) || !planLabel(p.CustodyId) || !planSha256(p.CustodyFenceHash) || !planSha256(p.CutoverEvidenceHash) || !planSha256(p.SourceLockHash) || !planSha256(p.Artifacts.Sha256) || !bootstrapRootAbsolutePath(p.Artifacts.Path) || !bootstrapRootAbsolutePath(p.RunDirectory) || p.StartNativeNumber == 0 || !rootCanonicalHash(p.StartNativeHash) || p.ValidThroughNative <= p.StartNativeNumber || p.ValidThroughNative-p.StartNativeNumber > 7200 || p.MaximumAttempts == 0 || p.MaximumAttempts > 8 || len(p.Actions) == 0 || len(p.Actions) > 9 {
		return errors.New("contract phase lacks exact identity, scope or finite bounds")
	}
	profile := p.Runtime
	if profile.RuntimeSourceCommit != frontierMappingSourceCommit || profile.RuntimeVersion.SpecName == "" || profile.RuntimeVersion.SpecVersion == 0 || !rootCanonicalHash(profile.RuntimeCodeHash) || !rootCanonicalHash(profile.RuntimeMetadataHash) {
		return errors.New("contract phase runtime/source approval is incomplete")
	}
	if p.Route.ReadRetrySeconds < 60 || p.Route.ReadRetrySeconds > 900 || p.Route.SendTimeoutSeconds == 0 || p.Route.SendTimeoutSeconds > 60 {
		return errors.New("contract phase transport bounds are invalid")
	}
	if err := p.Route.validate(); err != nil {
		return err
	}
	ids := []string{"reserve-create", "vault-create", "coordinator-create", "escrow-register", "proxy-create", "reserve-link", "vault-link", "evidence-create", "evidence-anchor"}
	total := new(big.Int)
	seen := map[string]bool{}
	for i, action := range p.Actions {
		if action.Id != ids[i] {
			return errors.New("contract phase action sequence differs")
		}
		tx, err := action.unsigned()
		if err != nil {
			return err
		}
		key := action.Sender.Hex() + ":" + new(big.Int).SetUint64(action.Nonce).String()
		if seen[key] {
			return errors.New("contract phase reuses a sender nonce")
		}
		seen[key] = true
		total.Add(total, new(big.Int).Add(tx.Value(), new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasFeeCap())))
	}
	maximum, err := evmWei(p.MaximumTotalWei)
	if err != nil || total.BitLen() > 256 || total.Cmp(maximum) > 0 {
		return errors.Join(errors.New("contract phase exceeds original total cost reservation"), err)
	}
	if p.Actions[0].To != nil || p.Actions[0].ValueWei != "0" {
		return errors.New("reserve action must be a zero-value CREATE")
	}
	key, err := rootReceiptHex(self.ApprovalPublicKey, 32)
	if err != nil || len(key) != 32 {
		return errors.New("contract phase approval key is invalid")
	}
	signature, err := rootOfflineSignatureBytes(self.Signature)
	message, msgErr := p.signingBytes()
	if err != nil || msgErr != nil || !ed25519.Verify(key, message, signature) {
		return errors.New("contract phase independent approval is invalid")
	}
	return nil
}

// The actual executable projection retains all approved graph reservations but
// exposes only the first reserve action until its descendants are implemented.
type evmCreatePlan struct {
	Config  evmPhaseConfig
	Address common.Address
	Runtime []byte
	Getters []contractGetter
}

// Rebuilding the constructor and every immutable rejects a signed arbitrary
// CREATE hidden behind the reserve action name. No external operation occurs.
func loadEvmCreatePlan(ctx context.Context, path string) (evmCreatePlan, error) {
	var result evmCreatePlan
	raw, _, err := readBootstrapRootFile(ctx, path, 2*1024*1024)
	if err != nil {
		return result, err
	}
	if err := decodePlanJson(raw, &result.Config); err != nil {
		return result, err
	}
	if err := result.Config.validate(); err != nil {
		return result, err
	}
	p := result.Config.Plan
	statePath := filepath.Join(p.RunDirectory, evmCreateStateFile)
	for _, input := range []string{path, p.Artifacts.Path} {
		if input == statePath || input == statePath+".lock" {
			return result, errors.New("contract phase input aliases its journal")
		}
	}
	if err := bootstrapRootDirectory(p.RunDirectory); err != nil {
		return result, err
	}
	artifacts, err := loadContractRelease(ctx, p.Artifacts)
	if err != nil {
		return result, err
	}
	var artifact contractReleaseArtifact
	for _, candidate := range artifacts.Artifacts {
		if candidate.Name == "ReserveSink" {
			artifact = candidate
		}
	}
	var hotkey [32]byte
	decoded, _ := hex.DecodeString(strings.TrimPrefix(p.ReserveHotkey, "0x"))
	copy(hotkey[:], decoded)
	action := p.Actions[0]
	data, runtime, getters, err := contractReservePayload(artifact, p.Netuid, hotkey, action.Sender, action.Nonce)
	if err != nil || action.Data != "0x"+hex.EncodeToString(data) {
		return result, errors.Join(errors.New("approved reserve constructor differs from release and custody identities"), err)
	}
	result.Address, result.Runtime, result.Getters = crypto.CreateAddress(action.Sender, action.Nonce), runtime, getters
	return result, nil
}
