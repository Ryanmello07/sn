// Recovery must gate a forward fleet transition without rewriting an earlier
// deployment or a borrowed dependency slice. Historical layouts remain valid.
package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

// Exercise the complete revision builder after deployment became an ancestor
// of evidence/config setup. Every later fleet mutation must remain gated.
func TestVoluntaryConvictionRecoveryGatesFleetAfterEvidenceDeployment(t *testing.T) {
	cfg, stateDir, prior, current, entries, recovery := testVoluntaryConvictionDuplicateRecovery(t)
	batcher := actionByID(t, prior, "fleet.refresh.deploy-batcher")
	entries = append(entries, JournalEntry{PlanHash: prior.PlanHash, ActionID: batcher.ID, IntentHash: batcher.IntentHash, Stage: StageVerified})
	priorBytes, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	revised, err := buildPlanRevisionFromFactsWithMigrationAndRecoveries(cfg, stateDir, prior, current, entries, time.Unix(3, 0), nil, []voluntaryConvictionDuplicateRecovery{recovery})
	if err != nil {
		t.Fatalf("forward fleet recovery produced an invalid revision: %v", err)
	}
	reconciliation := actionByID(t, revised, voluntaryConvictionReconciliationActionID)
	repairId := reconciliation.Parameters[voluntaryRecoveryRepairActionParameter]
	if carried := actionByID(t, revised, batcher.ID); carried.IntentHash != batcher.IntentHash || !slices.Equal(carried.DependsOn, batcher.DependsOn) {
		t.Fatalf("recovery rewrote an already verified earlier deployment: %+v", carried)
	}
	if original := actionByID(t, revised, voluntaryConvictionActionID); original.IntentHash != recovery.OriginalIntentHash {
		t.Fatal("recovery replaced the authenticated one-shot conviction")
	}
	seenIds := map[string]bool{}
	gatedIds := map[string]bool{repairId: true}
	fleetCount := 0
	for _, action := range revised.Actions {
		for _, dependency := range action.DependsOn {
			if !seenIds[dependency] {
				t.Fatalf("recovery introduced a backward dependency: %s -> %s", action.ID, dependency)
			}
			gatedIds[action.ID] = gatedIds[action.ID] || gatedIds[dependency]
		}
		seenIds[action.ID] = true
		if action.ID == "fleet.refresh.oracle-activate" || strings.HasPrefix(action.ID, "fleet.commitment.") ||
			strings.HasPrefix(action.ID, "fleet.install.") || strings.HasPrefix(action.ID, "fleet.mirror.") ||
			strings.HasPrefix(action.ID, "fleet.bind.") || strings.HasPrefix(action.ID, "fleet.refresh.commitment.") ||
			strings.HasPrefix(action.ID, "fleet.refresh.batch.") || action.ID == "topology.launch" {
			fleetCount++
			if !gatedIds[action.ID] {
				t.Fatalf("fleet action %s can bypass the alpha repair", action.ID)
			}
		}
	}
	if fleetCount < cfg.Config.Topology.HeadFleets {
		t.Fatalf("incomplete fleet dependency census: %d", fleetCount)
	}
	after, err := json.Marshal(prior)
	if err != nil || !bytes.Equal(after, priorBytes) {
		t.Fatalf("revision mutated the authenticated source plan: %v", err)
	}
	if err := validatePlanBudget(revised); err != nil {
		t.Fatalf("recovery changed approved economics or topology: %v", err)
	}
}

// The pre-companion plan can deploy the batcher after conviction. Keep that
// earlier forward barrier instead of unnecessarily changing oracle activation.
func TestVoluntaryConvictionRecoveryRetainsHistoricalForwardBatcher(t *testing.T) {
	cfg, stateDir, prior, current, entries, recovery := testVoluntaryConvictionDuplicateRecovery(t)
	roles, err := derivePublicRoles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := buildPlan(cfg, current, roles, time.Unix(3, 0))
	if err != nil {
		t.Fatal(err)
	}
	revised := validatorEvidenceLegacyPlanTest(t, fresh)
	batcher := actionByID(t, revised, "fleet.refresh.deploy-batcher")
	activation := actionByID(t, revised, "fleet.refresh.oracle-activate")
	revised.Actions = slices.DeleteFunc(revised.Actions, func(action Action) bool { return action.ID == batcher.ID })
	for index, action := range revised.Actions {
		if action.ID == voluntaryConvictionActionID {
			revised.Actions = slices.Insert(revised.Actions, index+1, batcher)
			break
		}
	}
	revised.PriorPlanHashes = append(slices.Clone(prior.PriorPlanHashes), prior.PlanHash)
	revised.PlanHash, err = revised.hash()
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePlanBudget(revised); err != nil {
		t.Fatalf("historical forward-batcher prerequisite is invalid: %v", err)
	}
	if err := applyVoluntaryConvictionDuplicateRecovery(cfg, stateDir, revised, prior, entries, recovery); err != nil {
		t.Fatal(err)
	}
	reconciliation := actionByID(t, revised, voluntaryConvictionReconciliationActionID)
	repairId := reconciliation.Parameters[voluntaryRecoveryRepairActionParameter]
	if !slices.Contains(actionByID(t, revised, batcher.ID).DependsOn, repairId) || actionByID(t, revised, activation.ID).IntentHash != activation.IntentHash {
		t.Fatal("historical recovery did not retain its first forward batcher barrier")
	}
	gasAfter, err := voluntaryConvictionRecoveryGasAfter(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err := applySupersededSpend(revised, Spend{EVMGasWei: gasAfter}); err != nil {
		t.Fatal(err)
	}
	if err := trimLiveCampaignEVMReserveToLimit(revised); err != nil {
		t.Fatal(err)
	}
	revised.PlanHash, err = revised.hash()
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePlanBudget(revised); err != nil {
		t.Fatalf("historical recovery graph or accounting is invalid: %v", err)
	}
}

// An earlier batcher is not a fallback when no later fleet transition exists.
func TestVoluntaryConvictionRecoveryRejectsOnlyAncestorBarrier(t *testing.T) {
	cfg, stateDir, prior, current, entries, recovery := testVoluntaryConvictionDuplicateRecovery(t)
	roles, err := derivePublicRoles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	revised, err := buildPlan(cfg, current, roles, time.Unix(3, 0))
	if err != nil {
		t.Fatal(err)
	}
	batcher := actionByID(t, revised, "fleet.refresh.deploy-batcher")
	for index, action := range revised.Actions {
		if action.ID == voluntaryConvictionActionID {
			revised.Actions = revised.Actions[:index+1]
			break
		}
	}
	if err := applyVoluntaryConvictionDuplicateRecovery(cfg, stateDir, revised, prior, entries, recovery); err == nil || !strings.Contains(err.Error(), "no forward fleet-setup recovery barrier") {
		t.Fatalf("ancestor-only recovery must fail closed: %v", err)
	}
	if actionByID(t, revised, batcher.ID).IntentHash != batcher.IntentHash {
		t.Fatal("rejected recovery rewrote an earlier deployment")
	}
}

// A retained action may share spare dependency capacity with another owner.
// Extending its barrier must not overwrite that owner's next element.
func TestVoluntaryConvictionRecoveryOwnsForwardBarrierDependencies(t *testing.T) {
	cfg, stateDir, prior, current, entries, recovery := testVoluntaryConvictionDuplicateRecovery(t)
	roles, err := derivePublicRoles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	revised, err := buildPlan(cfg, current, roles, time.Unix(3, 0))
	if err != nil {
		t.Fatal(err)
	}
	activation := findMutableAction(revised, "fleet.refresh.oracle-activate")
	if activation == nil {
		t.Fatal("fixture has no forward activation")
	}
	activationId := activation.ID
	shared := append(slices.Clone(activation.DependsOn), "untouched-synthetic-owner")
	activation.DependsOn = shared[:len(shared)-1]
	before := slices.Clone(shared)
	if err := applyVoluntaryConvictionDuplicateRecovery(cfg, stateDir, revised, prior, entries, recovery); err != nil {
		t.Fatal(err)
	}
	reconciliation := actionByID(t, revised, voluntaryConvictionReconciliationActionID)
	if !slices.Contains(actionByID(t, revised, activationId).DependsOn, reconciliation.Parameters[voluntaryRecoveryRepairActionParameter]) {
		t.Fatal("forward activation does not depend on the repair")
	}
	if !slices.Equal(shared, before) {
		t.Fatalf("recovery overwrote a borrowed dependency owner: %v", shared)
	}
}
