//go:build linux || darwin

package main

import (
	"testing"
	"time"
)

// Compare two genuine complete setup plans. An extra slot must move gas from
// the ordinary campaign to the same keeper, not spend the ceiling twice or
// create a fresh wallet/funding role.
func TestEvidenceRelayReserveIsSubtractedOnceAndFundsExistingKeeper(t *testing.T) {
	fixture := newRuntimeEvidenceProvisionV2TestFixture(t)
	first := fixture.plan
	fixture.cfg.Config.ValidatorEvidenceRelay.MaxSlots++
	var err error
	fixture.cfg.ConfigHash, err = releaseConfigHash(fixture.cfg.Config, fixture.cfg.Public, fixture.cfg.Hyperparameters)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := derivePublicRoles(fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	facts := first.LiveFacts
	second, err := buildPlan(fixture.cfg, &facts, roles, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePlanBudget(second); err != nil {
		t.Fatal(err)
	}
	find := func(plan *SetupPlan, id string) Action {
		for _, action := range plan.Actions {
			if action.ID == id {
				return action
			}
		}
		t.Fatalf("approved action %s is absent", id)
		return Action{}
	}
	oldReserve, newReserve := find(first, evidenceRelayReserveId), find(second, evidenceRelayReserveId)
	want := multiplyUint64Decimal(fixture.cfg.Config.ValidatorEvidenceRelay.GasUnits, fixture.cfg.Config.Budgets.MaximumEVMFeePerGasWei)
	delta, err := subtractDecimalUint(newReserve.Spend.EVMGasWei, oldReserve.Spend.EVMGasWei)
	if err != nil || delta != want {
		t.Fatal("reserve did not add exactly one call", delta, want, err)
	}
	oldCampaign, newCampaign := find(first, "campaign.evm-gas-reserve"), find(second, "campaign.evm-gas-reserve")
	delta, err = subtractDecimalUint(oldCampaign.Spend.EVMGasWei, newCampaign.Spend.EVMGasWei)
	if err != nil || delta != want {
		t.Fatal("relay gas was not subtracted exactly once", delta, want, err)
	}
	if first.MaximumSpend.EVMGasWei != second.MaximumSpend.EVMGasWei || len(first.Actions) != len(second.Actions) || first.PlanHash == second.PlanHash {
		t.Fatal("relay allowance lost plan identity or duplicated total gas/funding actions")
	}
	oldKeeper, newKeeper := find(first, "evm.fund-keeper"), find(second, "evm.fund-keeper")
	weight := uint64(4 + 3*fixture.cfg.Config.Topology.Operators)
	remaining, err := multiplyDivideDecimalUint(want, weight-4, weight)
	if err != nil {
		t.Fatal(err)
	}
	extraRao, err := ceilDivideDecimalUintToUint64(remaining, 1_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if oldKeeper.Target != newKeeper.Target || newKeeper.Spend.TAORao-oldKeeper.Spend.TAORao != extraRao {
		t.Fatal("the existing keeper was not funded for its exact relay share")
	}
}
