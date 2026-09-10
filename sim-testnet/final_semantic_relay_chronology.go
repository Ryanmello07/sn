//go:build linux || darwin

// Original relay requests follow their journal into the existing foundation
// and fleet-lineage artifacts. Archived replay never trusts an action map.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
)

// Only a journal-selected canonical slot may name a foundation source file.
func finalRelayRequestFoundationPath(key evidenceRelayRequestKey) (string, error) {
	slot := strings.TrimPrefix(key.actionId, evidenceRelayActionPrefix)
	if !validCanonicalHashHex(key.planHash) || !strings.HasPrefix(key.actionId, evidenceRelayActionPrefix) || !validCanonicalHashHex("0x"+slot) {
		return "", errors.New("historical relay request has a noncanonical source owner")
	}
	return "launch-foundation/evidence-relay/" + slot + ".json", nil
}

// Reads exact original bytes, including failed and predecessor admissions,
// and refuses extra or aliased files in the closed request namespace.
func finalRelayRequestsFromFiles(plans map[string]*SetupPlan, entries []JournalEntry, files map[string][]byte) (map[evidenceRelayRequestKey][]byte, error) {
	expected := make(map[string]bool)
	requests, err := evidenceRelayRequestsForJournal(plans, entries, func(key evidenceRelayRequestKey) ([]byte, error) {
		name, err := finalRelayRequestFoundationPath(key)
		if err != nil {
			return nil, err
		}
		if expected[name] {
			return nil, errors.New("historical relay request path has multiple plan owners")
		}
		expected[name] = true
		raw, found := files[name]
		if !found {
			return nil, fmt.Errorf("historical relay original request %s is absent", name)
		}
		return raw, nil
	})
	if err != nil {
		return nil, err
	}
	for name := range files {
		if name != "launch-foundation/evidence-relay" && !strings.HasPrefix(name, "launch-foundation/evidence-relay/") {
			continue
		}
		if !expected[name] {
			return nil, fmt.Errorf("historical relay request namespace contains an unapproved source %s", name)
		}
	}
	return requests, nil
}

// The independent replay gets requests from the same content-addressed fleet
// lineage that supplies its plans and journal, never a mutable state path.
func finalRelayRequestsFromArtifact(evidence *FinalSemanticEvidence, plans map[string]*SetupPlan, entries []JournalEntry, cache map[string][]byte) (map[evidenceRelayRequestKey][]byte, error) {
	if evidence == nil || evidence.FleetGeneration == nil {
		return nil, errors.New("historical relay fleet-lineage artifact is absent")
	}
	raw, found := cache[evidence.FleetGeneration.Artifact.URI]
	if !found {
		return nil, errors.New("historical relay fleet-lineage bytes are not loaded")
	}
	files, err := finalFleetGenerationArtifactFiles(evidence, raw)
	if err != nil {
		return nil, err
	}
	return finalRelayRequestsFromFiles(plans, entries, files)
}

// Extends the already-read foundation using its exact plan and journal bytes.
// The original bounded request reader returns bytes once; capture retains those
// bytes directly instead of opening a mutable path for a second read.
func captureFinalRelayFoundationEntries(ctx context.Context, stateRoot string, foundation []FinalCollectedFileBundleEntry) ([]FinalCollectedFileBundleEntry, error) {
	if ctx == nil || stateRoot == "" {
		return nil, errors.New("relay foundation capture owner is absent")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var planBytes, journalBytes []byte
	for _, item := range foundation {
		switch item.Path {
		case "plan.json":
			if planBytes != nil {
				return nil, errors.New("relay foundation current plan is duplicated")
			}
			planBytes = item.Data
		case "journal.jsonl":
			if journalBytes != nil {
				return nil, errors.New("relay foundation journal is duplicated")
			}
			journalBytes = item.Data
		}
	}
	current, err := decodePersistedPlanBytes(planBytes)
	if err != nil || current == nil {
		return nil, errors.Join(errors.New("relay foundation current plan is absent or invalid"), err)
	}
	entries, err := decodeFinalSemanticJournalBytes(journalBytes)
	if err != nil {
		return nil, err
	}
	plans := map[string]*SetupPlan{current.PlanHash: current}
	for _, hash := range current.PriorPlanHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		plan, err := readValidatorEvidenceHistoricalPlan(stateRoot, hash)
		if err != nil || plan == nil || plan.PlanHash != hash || plan.DeploymentID != current.DeploymentID || plan.ChainID != current.ChainID || plan.Netuid != current.Netuid || plan.GenesisHash != current.GenesisHash {
			return nil, stateMismatchError(err, "relay foundation predecessor plan %s differs", hash)
		}
		if plans[hash] != nil {
			return nil, errors.New("relay foundation predecessor plan is duplicated")
		}
		plans[hash] = plan
	}
	requests, err := evidenceRelayRequestsFromState(ctx, stateRoot, plans, entries)
	if err != nil {
		return nil, err
	}
	result := make([]FinalCollectedFileBundleEntry, 0, len(requests))
	for key, raw := range requests {
		name, err := finalRelayRequestFoundationPath(key)
		if err != nil {
			return nil, err
		}
		owned := bytes.Clone(raw)
		result = append(result, FinalCollectedFileBundleEntry{Path: strings.TrimPrefix(name, "launch-foundation/"), ContentHash: bytesSHA256(owned), SizeBytes: uint64(len(owned)), Data: owned})
	}
	return result, ctx.Err()
}
