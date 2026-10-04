// The combined original policy selects the immutable earning identity. The
// full epoch evidence remains independently verified on live and cold paths.
package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/urfoundation/sn/payoutartifact"
)

// Network values are synthetic; the transition semantics are the exact
// independently approved production grammar, including the UTC cutoff.
func economicProviderEarningPolicyFixture(t *testing.T) (*economicProviderMeasurementPolicy, economicConservationPolicy, *payoutartifact.Artifact) {
	t.Helper()
	selected, _ := economicProviderAttemptPolicyFixture(t)
	selected.EarningIdentity = &payoutartifact.WholeWorkEarningIdentity{Schema: "urnetwork-provider-payout-transition-v1", CutoffUtc: "2026-10-06T00:00:00Z", Attribution: "close_time", LegacyUsdc: "before_cutoff_only", Profile: "mainnet", ChainId: 945, GenesisHash: "0x" + strings.Repeat("ab", 32), Netuid: 521}
	policy := economicConservationPolicy{Vault: monitorEconomicEvmPolicy{Network: planNetwork{EvmChainId: 945, GenesisHash: selected.EarningIdentity.GenesisHash}, Netuid: 521}}
	artifact := &payoutartifact.Artifact{ChainID: 945, GenesisHash: selected.EarningIdentity.GenesisHash, Netuid: 521}
	return selected, policy, artifact
}

// A witness-controlled declaration is not consulted when reconstructing the
// original expectation. Reopening the serialized policy derives the same hash.
func TestEconomicProviderEarningSelectionRetainsIndependentPolicy(t *testing.T) {
	selected, policy, artifact := economicProviderEarningPolicyFixture(t)
	if err := selected.validate(policy); err != nil {
		t.Fatal("approved earning identity refused", err)
	}
	priors := []payoutartifact.WholeWorkPriorContract{{ContractId: [16]byte{1}, ReconciledEpoch: 2, InventoryHash: "sha256:" + strings.Repeat("cd", 32)}}
	expected, err := selected.workExpectation(artifact, "0x0000000000000000000000000000000000007890", priors)
	if err != nil || expected.EarningSelection == nil || !expected.EarningSelection.StartTime.Equal(time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)) || len(expected.PriorContracts) != 1 || expected.PriorContracts[0] != priors[0] {
		t.Fatal("original earning selection or retained predecessor missing", expected, err)
	}
	artifact.ClosedWork = &payoutartifact.ClosedWorkCensus{EarningStart: "2026-10-07T00:00:00Z", EarningSelectionHash: "sha256:" + strings.Repeat("ef", 32)}
	encoded, err := json.Marshal(selected)
	if err != nil {
		t.Fatal(err)
	}
	var reopened economicProviderMeasurementPolicy
	if err := json.Unmarshal(encoded, &reopened); err != nil {
		t.Fatal(err)
	}
	again, err := reopened.workExpectation(artifact, "0x0000000000000000000000000000000000007890", priors)
	if err != nil || again.EarningSelection == nil || *again.EarningSelection != *expected.EarningSelection {
		t.Fatal("publisher declaration replaced cold original earning identity", again, err)
	}
}

// Every semantic and network coordinate comes from the original policy;
// declaration aliases and other deployments cannot borrow its cutoff.
func TestEconomicProviderEarningSelectionRejectsForeignIdentity(t *testing.T) {
	selected, policy, artifact := economicProviderEarningPolicyFixture(t)
	original := *selected.EarningIdentity
	changes := []func(*payoutartifact.WholeWorkEarningIdentity){
		func(value *payoutartifact.WholeWorkEarningIdentity) { value.Schema = "publisher-selected" },
		func(value *payoutartifact.WholeWorkEarningIdentity) { value.CutoffUtc = "2026-10-07T00:00:00Z" },
		func(value *payoutartifact.WholeWorkEarningIdentity) { value.Attribution = "observed_time" },
		func(value *payoutartifact.WholeWorkEarningIdentity) { value.LegacyUsdc = "all_time" },
		func(value *payoutartifact.WholeWorkEarningIdentity) { value.Profile = "foreign" },
		func(value *payoutartifact.WholeWorkEarningIdentity) { value.ChainId++ },
		func(value *payoutartifact.WholeWorkEarningIdentity) {
			value.GenesisHash = "0x" + strings.Repeat("cd", 32)
		},
		func(value *payoutartifact.WholeWorkEarningIdentity) { value.Netuid++ },
	}
	for index, change := range changes {
		candidate := original
		change(&candidate)
		selected.EarningIdentity = &candidate
		if err := selected.validate(policy); err == nil {
			t.Fatal("foreign earning identity admitted", index)
		}
		if _, err := selected.workExpectation(artifact, "", nil); err == nil {
			t.Fatal("foreign earning identity reached witness reader", index)
		}
	}
	selected.EarningIdentity = &original
	artifact.ChainID++
	if _, err := selected.workExpectation(artifact, "", nil); !errors.Is(err, payoutartifact.ErrClosedWorkIntegrity) {
		t.Fatal("foreign artifact borrowed independent earning policy", err)
	}
}

// Missing policy remains missing even if a supplied census includes a cutoff.
// The nil profile keeps the original policy serialization unchanged.
func TestEconomicProviderEarningDeclarationCannotCreatePolicy(t *testing.T) {
	selected, _, artifact := economicProviderEarningPolicyFixture(t)
	selected.EarningIdentity = nil
	artifact.ClosedWork = &payoutartifact.ClosedWorkCensus{EarningStart: "2026-10-06T00:00:00Z", EarningSelectionHash: "sha256:" + strings.Repeat("ef", 32)}
	expected, err := selected.workExpectation(artifact, "", nil)
	if err != nil || expected.EarningSelection != nil {
		t.Fatal("publisher earning declaration acquired independent authority", expected, err)
	}
	encoded, err := json.Marshal(selected)
	if err != nil || strings.Contains(string(encoded), "earning_identity") {
		t.Fatal("legacy nil earning profile changed serialized policy", err)
	}
}
