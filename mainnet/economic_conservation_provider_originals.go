// Provider admission retains original portable evidence, never a caller's
// complete flag. The independently selected per-pool policy supplies authority;
// the existing receipt reader supplies exact original epoch and contract pins.
package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/urfoundation/sn/payoutartifact"
	"github.com/urfoundation/sn/protocol"
	"github.com/urfoundation/sn/validator"
)

const economicProviderMeasurementPolicySchema = "urnetwork-economic-provider-measurements-v1"

// The signer approves a prospective complete SDK/provider roster; the pinned
// attempt source supplies independent policy/registry/key history. Neither is
// selected from an artifact or from the observed current database directory.
type economicProviderMeasurementPolicy struct {
	Schema                   string                          `json:"schema"`
	AttributionSigner        string                          `json:"attribution_signer,omitempty"`
	WholeWorkAuthoritySigner string                          `json:"whole_work_authority_signer"`
	WalletEndpoint           string                          `json:"wallet_endpoint"`
	AttemptAuthority         validator.ReleaseEvidenceV2File `json:"attempt_authority"`
	MaximumOriginalBytes     uint64                          `json:"maximum_original_bytes"`
}

// Each chain is independently selected by the signed expected-provider head.
// The complete raw history remains available after the API becomes unavailable.
type economicProviderWalletOriginal struct {
	ClientId  [16]byte                        `json:"client_id"`
	Originals []protocol.WalletMappingConsent `json:"originals"`
}

// Portable originals are bounded by the original physical and logical profile.
// A compact projection cannot replace any one of these independent components.
type economicProviderOriginals struct {
	Work     *payoutartifact.WholeWorkInventory        `json:"work"`
	Wallets  []economicProviderWalletOriginal          `json:"wallets"`
	Bindings *validator.ProviderAttemptBindingOriginal `json:"bindings"`
	Attempts json.RawMessage                           `json:"attempts"`
}

// Private cached results are reachable only through an admitted original
// census identity. Retaining only newly reconciled terminal contracts keeps
// historical index cost proportional to real originals rather than windows.
type economicProviderAdmitted struct {
	Domain        protocol.ClientKeyHistoryDomain
	Epoch         uint64
	InventoryHash string
	Measurement   economicConservationProviderMeasurement
	ClosedWork    economicConservationClosedWork
	NewContracts  map[[16]byte]payoutartifact.WholeWorkPriorContract
}

// The index is private to the held admission owner. An entry selects a retained
// census identity, never a proposed exclusion or a mutable publisher directory.
type economicProviderContractKey struct {
	Domain     protocol.ClientKeyHistoryDomain
	ContractId [16]byte
}

// Nil preserves exact legacy policy bytes and its unknown measurement result.
func (self *economicProviderMeasurementPolicy) validate(policy economicConservationPolicy) error {
	if self == nil {
		return nil
	}
	ref := self.AttemptAuthority
	if self.AttributionSigner != "" && !monitorEvmAddress(self.AttributionSigner) {
		return errors.New("economic provider attribution authority is invalid")
	}
	if self.Schema != economicProviderMeasurementPolicySchema || !monitorEvmAddress(self.WholeWorkAuthoritySigner) || self.MaximumOriginalBytes == 0 || self.MaximumOriginalBytes > uint64(policy.storageMaximum()) || !filepath.IsAbs(ref.Path) || filepath.Clean(ref.Path) != ref.Path || !planSha256(ref.SHA256) || ref.Bytes == 0 || ref.Bytes > 16*1024*1024 {
		return errors.New("economic provider original authority or finite physical profile is incomplete")
	}
	_, err := validator.NewHttpWalletMappingReader(self.WalletEndpoint)
	return err
}

// The selected provider head is independent of both transport and the payout
// row. Missing original mapping never becomes a zero coldkey or a missing wallet.
func economicProviderWalletExpected(authority *payoutartifact.WholeWorkAuthority, member payoutartifact.WholeWorkExpectedProvider) (protocol.WalletMappingHistoryExpectation, error) {
	if member.WalletHeadHash == "" || member.WalletGeneration == 0 {
		return protocol.WalletMappingHistoryExpectation{}, protocol.ErrWalletMappingUnavailable
	}
	raw, err := hex.DecodeString(member.WalletHeadHash)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != member.WalletHeadHash || member.WalletGeneration > protocol.MaxWalletMappingHistory {
		return protocol.WalletMappingHistoryExpectation{}, protocol.ErrWalletMappingIntegrity
	}
	return protocol.WalletMappingHistoryExpectation{Domain: authority.Domain, ClientId: member.ClientId, HeadHash: [32]byte(raw), Generation: member.WalletGeneration, Epoch: authority.Epoch}, nil
}

// Reserve actual serialized evidence, including JSON framing, before keeping
// another external response in the single per-pool handoff slot.
func (self *economicProviderOriginals) bounded(maximum uint64) error {
	raw, err := json.Marshal(self)
	if err != nil {
		return err
	}
	if maximum == 0 || uint64(len(raw)) > maximum {
		return errMonitorEconomicCapacity
	}
	return nil
}

// Every current window contract is looked up independently, including contracts
// omitted from the proposed exclusion list. Only a published original census
// may establish prior credit; failed candidate caches cannot acquire that role.
func (self *economicConservationArchiveView) providerPriorContracts(ctx context.Context, state *economicConservationState, authority *payoutartifact.WholeWorkAuthority, inventory *payoutartifact.WholeWorkInventory) ([]payoutartifact.WholeWorkPriorContract, error) {
	if self == nil || state == nil || authority == nil || inventory == nil || inventory.Window == nil {
		return nil, payoutartifact.ErrClosedWorkUnavailable
	}
	candidates := make(map[[16]byte]struct{}, len(inventory.Window.Records)+len(authority.PriorContracts))
	for _, row := range inventory.Window.Records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := row.ContractId
		if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' || strings.ToLower(id) != id {
			return nil, payoutartifact.ErrClosedWorkIntegrity
		}
		raw, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
		if err != nil || len(raw) != 16 || [16]byte(raw) == ([16]byte{}) {
			return nil, payoutartifact.ErrClosedWorkIntegrity
		}
		candidates[[16]byte(raw)] = struct{}{}
	}
	for _, proposed := range authority.PriorContracts {
		candidates[proposed.ContractId] = struct{}{}
	}
	if len(candidates) > payoutartifact.MaxClosedWorkRecords {
		return nil, payoutartifact.ErrClosedWorkCapacity
	}
	knownContracts := make(map[[16]byte]payoutartifact.WholeWorkPriorContract, len(candidates))
	for contractId := range candidates {
		for key := range self.providerContracts[economicProviderContractKey{Domain: authority.Domain, ContractId: contractId}] {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			known := self.providerCensuses[key]
			if known == nil || known.Domain != authority.Domain {
				return nil, payoutartifact.ErrClosedWorkIntegrity
			}
			id := economicConservationEntitlementId(strconv.FormatUint(known.Epoch, 10), strconv.FormatUint(authority.Domain.NoID, 10))
			var record economicConservationEntitlement
			if index, ok := state.entitlementIds[id]; ok {
				record = state.Entitlements[index]
			} else {
				record = self.entitlements[id]
			}
			if record.Id != id || record.censusHash() != key || self.entitlementVerified[key] != economicEntitlementFundingHash(record) {
				continue
			}
			original, exists := known.NewContracts[contractId]
			if !exists || original.ReconciledEpoch != known.Epoch || original.InventoryHash != known.InventoryHash || known.Epoch >= authority.Epoch {
				return nil, payoutartifact.ErrClosedWorkIntegrity
			}
			if prior, exists := knownContracts[contractId]; exists && prior != original {
				return nil, payoutartifact.ErrClosedWorkIntegrity
			}
			knownContracts[contractId] = original
		}
	}
	for _, proposed := range authority.PriorContracts {
		original, exists := knownContracts[proposed.ContractId]
		if !exists {
			return nil, payoutartifact.ErrClosedWorkUnavailable
		}
		if original != proposed {
			return nil, payoutartifact.ErrClosedWorkIntegrity
		}
	}
	result := make([]payoutartifact.WholeWorkPriorContract, 0, len(knownContracts))
	for _, original := range knownContracts {
		result = append(result, original)
	}
	slices.SortFunc(result, func(a, b payoutartifact.WholeWorkPriorContract) int {
		return bytes.Compare(a.ContractId[:], b.ContractId[:])
	})
	return result, ctx.Err()
}

// Convert original row authorities only after all expected owners and requests
// were verified. The complete work roster supplies idle and failed providers;
// absence from the complete trial census then means exactly zero exposure.
func economicProviderTrialProjection(ctx context.Context, artifact *payoutartifact.Artifact, work *payoutartifact.VerifiedWholeWorkInventory, attempts *validator.VerifiedProviderAttemptMeasurement, wallets map[[16]byte]*protocol.VerifiedWalletMapping, bindings []validator.VerifiedProviderAttemptBinding) (*economicProviderTrialValues, error) {
	if attempts == nil || attempts.VerifiedProviderAttemptWindow == nil || !attempts.CutCensusComplete || !attempts.OwnedRequestsComplete || attempts.ReliabilityAMin == 0 {
		return nil, protocol.ErrProviderAttemptsUnavailable
	}
	domain := work.Domain
	if attempts.Domain != economicProviderAttemptDomain(domain) || attempts.Window.Epoch != artifact.Epoch || attempts.Window.StartBlock != artifact.Start.Number || attempts.Window.EndBlock != artifact.End.Number || attempts.ReliabilityAMin != artifact.ReliabilityAMin {
		return nil, protocol.ErrProviderAttemptsIntegrity
	}
	if len(bindings) != len(work.ExpectedProviders) || len(wallets) != len(work.ExpectedProviders) {
		return nil, protocol.ErrProviderAttemptsUnavailable
	}
	counts := map[[16]byte]economicProviderTrialValue{}
	for _, expected := range work.ExpectedProviders {
		counts[expected.ClientId] = economicProviderTrialValue{clientId: expected.ClientId, networkId: expected.NetworkId}
	}
	for _, row := range attempts.Providers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if row.NoId != artifact.NoID {
			continue
		}
		value, exists := counts[row.ClientId]
		if !exists || row.Confirmations > row.Assignments || row.Assignments > math.MaxUint64-value.assignments || row.Confirmations > math.MaxUint64-value.confirmations {
			return nil, protocol.ErrProviderAttemptsIntegrity
		}
		value.assignments += row.Assignments
		value.confirmations += row.Confirmations
		counts[row.ClientId] = value
	}
	result := &economicProviderTrialValues{artifactHash: artifact.ContentHash, registryHash: "sha256:" + hex.EncodeToString(attempts.RegistryHash[:]), windowHash: "sha256:" + hex.EncodeToString(attempts.WindowHash[:]), minimumAssignments: attempts.ReliabilityAMin, complete: true}
	for index, provider := range work.ExpectedProviders {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		wallet, binding := wallets[provider.ClientId], bindings[index]
		if wallet == nil || wallet.Statement.ClientId != provider.ClientId || wallet.Statement.NetworkId != provider.NetworkId || binding.ClientId != provider.ClientId {
			return nil, protocol.ErrProviderAttemptsIntegrity
		}
		value := counts[provider.ClientId]
		value.coldkey, value.headExcluded, value.bindingGeneration = wallet.Statement.Coldkey, binding.HeadExcluded, binding.BindingGeneration
		value.eligible = provider.UsageBytes > 0 && value.assignments >= attempts.ReliabilityAMin && value.confirmations > 0 && !value.headExcluded
		if value.headExcluded {
			value.exclusionReason = "head_fleet_active"
		} else if !value.eligible {
			value.exclusionReason = "reliability_exposure_floor"
		}
		result.providers = append(result.providers, value)
	}
	return result, ctx.Err()
}

// Registry scope excludes the per-operator lane; the full original replay
// verifies all lanes before the selected pool's counts are projected above.
func economicProviderAttemptDomain(domain protocol.ClientKeyHistoryDomain) protocol.ProviderAttemptDomain {
	return protocol.ProviderAttemptDomain{ChainId: domain.ChainID, GenesisHash: domain.GenesisHash, Netuid: domain.Netuid, Coordinator: domain.Coordinator, SettlementVault: domain.SettlementVault, DeploymentIdHash: domain.DeploymentIDHash, PolicyHash: domain.PolicyHash}
}

// Stable numeric ordering ensures older present originals are admitted before
// a later census names them, independent of active array or SQL ordering.
func economicProviderAdmissionOrder(records []economicConservationEntitlement) []economicConservationEntitlement {
	result := slices.Clone(records)
	slices.SortFunc(result, func(a, b economicConservationEntitlement) int {
		aEpoch, bEpoch := uint64(math.MaxUint64), uint64(math.MaxUint64)
		if a.Census != nil {
			aEpoch = a.Census.Artifact.Epoch
		}
		if b.Census != nil {
			bEpoch = b.Census.Artifact.Epoch
		}
		if aEpoch < bEpoch {
			return -1
		}
		if aEpoch > bEpoch {
			return 1
		}
		return bytes.Compare([]byte(a.Id), []byte(b.Id))
	})
	return result
}
