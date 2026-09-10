// Evidence commitments have production deployment identity and an append-only
// coordinator anchor. Generated artifacts must preserve both facts.
package main

import (
	"slices"
	"testing"
)

// Pin semantic immutable names in declaration order, not unstable AST ids.
func TestValidatorEvidenceDefinitionIsProductionAndBindsEveryImmutable(t *testing.T) {
	count := 0
	for _, definition := range artifactDefinitions {
		if definition.name != "ValidatorEvidence" {
			continue
		}
		count++
		if !definition.release || definition.variable != "" || definition.path != "STValidatorEvidence.sol/STValidatorEvidence.json" ||
			!slices.Equal(definition.immutableNames, []string{"coordinator", "settlementVault", "chainId", "netuid", "genesisHash", "deploymentIdHash"}) ||
			!slices.Equal(definition.immutableSources, []string{"src/STValidatorEvidence.sol"}) {
			t.Fatal("evidence companion is not an exact production artifact")
		}
	}
	if count != 1 {
		t.Fatalf("evidence artifact count = %d, want exactly one", count)
	}
}

// A generator must reject the anchor moving into an old economic slot even
// when its ABI and function names are unchanged.
func TestValidatorEvidenceDefinitionPinsAppendOnlyAnchor(t *testing.T) {
	count := 0
	for _, definition := range artifactDefinitions {
		if definition.name != "Coordinator" {
			continue
		}
		count++
		for name, slot := range map[string]string{"netuid": "0", "settlementVault": "2", "reserveSink": "3", "validatorEvidence": "23"} {
			if definition.requiredStorageSlots[name] != slot {
				t.Fatalf("coordinator %s slot = %q, want %q", name, definition.requiredStorageSlots[name], slot)
			}
		}
	}
	if count != 1 {
		t.Fatalf("coordinator artifact count = %d, want exactly one", count)
	}
}
