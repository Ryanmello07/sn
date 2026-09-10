// Historical fixtures own complete source approvals and remove only fields
// that did not exist in their schema. Production admission remains unchanged.
package main

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Build through the ordinary shared config, so a partial lock in that helper
// fails at the same historical reader as the retained fleet/recovery failures.
func planCompatibilityCurrentFixtureTest(t *testing.T) (*ResolvedConfig, *SetupPlan) {
	t.Helper()
	cfg := testResolvedConfig(t)
	roles, err := derivePublicRoles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildPlan(cfg, testSetupFacts(), roles, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	return cfg, plan
}

// Recomputed outer hashes never make an incomplete or substituted original
// release lock valid. The public release approval is not rewritten by fixtures.
func TestPlanCompatibilityFixtureAuthenticatesCompleteOriginalReleaseSource(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "deploy", "testnet", "release.lock.yml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, plan := planCompatibilityCurrentFixtureTest(t)
	wire, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := decodePersistedPlanBytesForHistory(wire, true)
	if err != nil || opened == nil || opened.PlanHash != plan.PlanHash || !finalJSONEqual(opened.ValidatorEvidenceSource, plan.ValidatorEvidenceSource) {
		t.Fatalf("ordinary fixture cannot reopen its exact original source: %v", err)
	}
	if err := validateReleaseLockStatic(cfg.Release); err != nil {
		t.Fatalf("ordinary plan fixture has incomplete release authority: %v", err)
	}
	for _, change := range []struct {
		name   string
		mutate func(*ReleaseLock)
	}{
		{name: "absent image", mutate: func(lock *ReleaseLock) { lock.Runtime.Image = "" }},
		{name: "unpinned image", mutate: func(lock *ReleaseLock) { lock.Runtime.Image = "synthetic-runtime:latest" }},
		{name: "absent generated artifact", mutate: func(lock *ReleaseLock) { delete(lock.EVMBuild, "validator_evidence_artifact_hash") }},
		{name: "different generated runtime", mutate: func(lock *ReleaseLock) {
			lock.EVMBuild["validator_evidence_runtime_hash"] = "0x" + strings.Repeat("81", 32)
		}},
	} {
		changed := *plan
		lock := *cfg.Release
		lock.EVMBuild = maps.Clone(cfg.Release.EVMBuild)
		change.mutate(&lock)
		rebindValidatorEvidenceReleaseLockTest(t, &changed, &lock)
		changed.PlanHash, err = changed.hash()
		if err != nil {
			t.Fatal(err)
		}
		altered, err := json.Marshal(&changed)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := decodePersistedPlanBytesForHistory(altered, true); err == nil || got != nil || !strings.Contains(err.Error(), "validator evidence original release") {
			t.Fatalf("rehashed %s acquired historical source authority: %v", change.name, err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("fixture construction rewrote the actual release approval: %v", err)
	}
	retained, err := json.Marshal(plan)
	if err != nil || !bytes.Equal(wire, retained) {
		t.Fatalf("independent lock mutation changed its original plan owner: %v", err)
	}
}

// Every historical approval reopens through the actual wire/hash/budget
// reader. Companion fields/actions cannot be reintroduced by rehashing it.
func TestPlanCompatibilityFixtureProjectsExactHistoricalOwnership(t *testing.T) {
	t.Parallel()
	_, original := planCompatibilityCurrentFixtureTest(t)
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{"urnetwork-sim-plan-v1", "urnetwork-sim-plan-v2", setupPlanSchemaV8, setupPlanSchemaV9, setupPlanSchemaV10, setupPlanSchemaV11} {
		var projected *SetupPlan
		switch schema {
		case "urnetwork-sim-plan-v1", "urnetwork-sim-plan-v2":
			value := *original
			downgradePlanForCompatibilityTest(t, &value, schema)
			projected = &value
		case setupPlanSchemaV8:
			value := *original
			downgradeReserveEnvelopeToV8(t, &value)
			projected = &value
		default:
			projected = validatorEvidenceLegacyPlanSchemaTest(t, original, schema)
		}
		wire, err := json.Marshal(projected)
		if err != nil {
			t.Fatal(err)
		}
		opened, err := decodePersistedPlanBytesForHistory(wire, true)
		if err != nil || opened == nil || opened.PlanHash != projected.PlanHash || opened.Schema != schema {
			t.Fatalf("%s exact historical projection cannot reopen: %v", schema, err)
		}
		if projected.Schema != schema || projected.ValidatorEvidence != nil || projected.ValidatorEvidenceSource != nil || projected.ValidatorEvidenceCarry != nil {
			t.Fatalf("%s did not project an evidence-free historical owner", schema)
		}
		for _, action := range projected.Actions {
			if action.ID == validatorEvidenceDeployActionID || action.ID == validatorEvidenceAnchorActionID || slices.Contains(action.DependsOn, validatorEvidenceDeployActionID) || slices.Contains(action.DependsOn, validatorEvidenceAnchorActionID) {
				t.Fatalf("%s retained a future companion action/dependency in %s", schema, action.ID)
			}
		}
		spend, err := maximumActionSpend(projected.Actions)
		if err != nil || spend != projected.MaximumSpend {
			t.Fatalf("%s retained removed companion spend: %v", schema, err)
		}
		for _, field := range []string{"manifest", "source", "carry", "install action", "anchor action"} {
			var changed SetupPlan
			if err := json.Unmarshal(wire, &changed); err != nil {
				t.Fatal(err)
			}
			switch field {
			case "manifest":
				changed.ValidatorEvidence = original.ValidatorEvidence
			case "source":
				changed.ValidatorEvidenceSource = original.ValidatorEvidenceSource
			case "carry":
				changed.ValidatorEvidenceCarry = &ValidatorEvidenceCarry{Schema: "urnetwork-validator-evidence-carry-v1"}
			case "install action":
				changed.Actions = append(changed.Actions, actionByID(t, original, validatorEvidenceDeployActionID))
			case "anchor action":
				changed.Actions = append(changed.Actions, actionByID(t, original, validatorEvidenceAnchorActionID))
			}
			changed.MaximumSpend, err = maximumActionSpend(changed.Actions)
			if err != nil {
				t.Fatal(err)
			}
			changed.PlanHash, err = changed.hash()
			if err != nil {
				t.Fatal(err)
			}
			altered, err := json.Marshal(&changed)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := decodePersistedPlanBytesForHistory(altered, true); err == nil || got != nil || !strings.Contains(err.Error(), "legacy plan unexpectedly carries validator evidence") {
				t.Fatalf("%s rehashed foreign %s acquired legacy authority: %v", schema, field, err)
			}
		}
	}
	after, err := json.Marshal(original)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("historical projection mutated the complete current approval: %v", err)
	}
}

// Pre-companion revision is distinct from relocating an installed current
// companion. Matching surrounding deployment fields cannot cross that boundary.
func TestPlanCompatibilityFixtureKeepsCurrentCompanionRelocationClosed(t *testing.T) {
	t.Parallel()
	_, current := planCompatibilityCurrentFixtureTest(t)
	if current.ValidatorEvidence == nil {
		t.Fatal("current fixture lost its companion prerequisite")
	}
	if err := validateValidatorEvidenceRevision(current, current.ValidatorEvidence); err != nil {
		t.Fatalf("exact current companion was not carryable: %v", err)
	}
	next := *current.ValidatorEvidence
	next.DeployerNonce++
	next.Address = crypto.CreateAddress(next.Deployer, next.DeployerNonce)
	if err := validateValidatorEvidenceRevision(current, &next); err == nil || !strings.Contains(err.Error(), "authenticated existing-companion carry") {
		t.Fatalf("current approval borrowed pre-companion relocation authority: %v", err)
	}
	legacy := validatorEvidenceLegacyPlanTest(t, current)
	if err := validateValidatorEvidenceRevision(legacy, &next); err != nil {
		t.Fatalf("real pre-companion approval cannot enter its first install: %v", err)
	}
	legacy.ValidatorEvidence = current.ValidatorEvidence
	if err := validateValidatorEvidencePlan(legacy); err == nil || !strings.Contains(err.Error(), "legacy plan unexpectedly carries") {
		t.Fatalf("schema relabeling laundered an existing companion: %v", err)
	}
}
