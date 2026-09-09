// Release-lock hashing retains distinct compiler declarations and refuses
// ambiguous normalization rather than authenticating a partial storage graph.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Use the same real compiler layout as gencontracts without importing a child.
func releaseEvidenceStorageLayoutTest(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "validator-evidence-storage-layout.json"))
	if err != nil {
		t.Fatal(err)
	}
	var layout map[string]any
	if err := json.Unmarshal(data, &layout); err != nil {
		t.Fatal(err)
	}
	return layout
}

// Exercise the existing file-to-release-digest boundary using private fixtures.
func releaseStorageLayoutHashTest(t *testing.T, layout map[string]any) (string, error) {
	t.Helper()
	root := t.TempDir()
	data, err := json.Marshal(map[string]any{"storageLayout": layout})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return foundryStorageLayoutHash(root, "artifact.json")
}

// This cardinality proof fails for every iteration order of the old map copy.
func TestReleaseStorageLayoutPreservesBothEvidenceDomains(t *testing.T) {
	layout := releaseEvidenceStorageLayoutTest(t)
	before, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := normalizeReleaseStorageLayout(layout)
	if err != nil {
		t.Fatal(err)
	}
	types := normalized.(map[string]any)["types"].(map[string]any)
	if len(types) != 14 {
		t.Fatalf("release layout lost a declared type: got %d want 14", len(types))
	}
	for _, expected := range []struct {
		name, holder, width string
		members             int
	}{
		{name: "t_struct(ValidatorEvidence.Domain)_storage", holder: "t_struct(Header)_storage", width: "256", members: 9},
		{name: "t_struct(ValidatorEvidenceActivation.Domain)_storage", holder: "t_struct(Record)_storage", width: "224", members: 8},
	} {
		fields, exists := types[expected.name].(map[string]any)
		if !exists || fields["numberOfBytes"] != expected.width || len(fields["members"].([]any)) != expected.members {
			t.Fatalf("%s was dropped, merged or changed", expected.name)
		}
		reference := types[expected.holder].(map[string]any)["members"].([]any)[0].(map[string]any)["type"]
		if reference != expected.name {
			t.Fatalf("%s reference = %v, want %s", expected.holder, reference, expected.name)
		}
	}
	after, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("release normalization changed its source layout")
	}
}

// AST renumbering is irrelevant, while either domain's physical shape is not.
func TestReleaseStorageLayoutHashPinsEveryDomainAcrossCompilerGraphs(t *testing.T) {
	layout := releaseEvidenceStorageLayoutTest(t)
	baseline, err := releaseStorageLayoutHashTest(t, layout)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	renumbered := strings.NewReplacer("57292", "157292", "57298", "157298", "58897", "158897", "59634", "159634", "58931", "158931", "59657", "159657", "58903", "158903").Replace(string(raw))
	var other map[string]any
	if err := json.Unmarshal([]byte(renumbered), &other); err != nil {
		t.Fatal(err)
	}
	stable, err := releaseStorageLayoutHashTest(t, other)
	if err != nil {
		t.Fatal(err)
	}
	if baseline != stable {
		t.Fatal("compiler graph renumbering changed the release layout digest")
	}
	for _, change := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "evidence domain shape", mutate: func(types map[string]any) {
			types["t_struct(Domain)58897_storage"].(map[string]any)["numberOfBytes"] = "288"
		}},
		{name: "activation domain shape", mutate: func(types map[string]any) {
			types["t_struct(Domain)59634_storage"].(map[string]any)["members"].([]any)[7].(map[string]any)["slot"] = "7"
		}},
		{name: "member reference", mutate: func(types map[string]any) {
			types["t_struct(Header)58931_storage"].(map[string]any)["members"].([]any)[0].(map[string]any)["type"] = "t_struct(Domain)59634_storage"
		}},
		{name: "mapping reference", mutate: func(types map[string]any) {
			types["t_mapping(t_bytes32,t_struct(Activation)57292_storage)"].(map[string]any)["value"] = "t_struct(Commitment)57298_storage"
		}},
	} {
		input := releaseEvidenceStorageLayoutTest(t)
		change.mutate(input["types"].(map[string]any))
		hash, err := releaseStorageLayoutHashTest(t, input)
		if err != nil {
			t.Fatalf("%s: %v", change.name, err)
		}
		if hash == baseline {
			t.Errorf("%s escaped the release digest", change.name)
		}
	}
}

// No digest or partial normalized layout may escape an ambiguous type graph.
func TestReleaseStorageLayoutHashRejectsAmbiguousTypeGraphs(t *testing.T) {
	for _, change := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "colliding qualified labels", mutate: func(types map[string]any) {
			types["t_struct(Domain)59634_storage"].(map[string]any)["label"] = "struct ValidatorEvidence.Domain"
		}},
		{name: "missing declaration label", mutate: func(types map[string]any) { delete(types["t_struct(Domain)59634_storage"].(map[string]any), "label") }},
		{name: "unresolved declaration", mutate: func(types map[string]any) {
			types["t_struct(Header)58931_storage"].(map[string]any)["members"].([]any)[0].(map[string]any)["type"] = "t_struct(Domain)77777_storage"
		}},
	} {
		layout := releaseEvidenceStorageLayoutTest(t)
		change.mutate(layout["types"].(map[string]any))
		if normalized, err := normalizeReleaseStorageLayout(layout); err == nil || normalized != nil {
			t.Errorf("%s produced partial normalized authority: %v", change.name, err)
		}
		if hash, err := releaseStorageLayoutHashTest(t, layout); err == nil || hash != "" {
			t.Errorf("%s produced a release digest: hash=%q error=%v", change.name, hash, err)
		}
	}
}
