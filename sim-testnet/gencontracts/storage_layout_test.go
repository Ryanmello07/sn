// Preserve the complete real evidence layout while removing compiler-only IDs.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The fixture is the storageLayout projection of the preserved full compiler
// artifact, not a hand-built pair which can omit the affected reference graph.
func evidenceStorageLayoutTest(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "validator-evidence-storage-layout.json"))
	if err != nil {
		t.Fatal(err)
	}
	var layout map[string]any
	if err := json.Unmarshal(data, &layout); err != nil {
		t.Fatal(err)
	}
	return layout
}

// JSON copying prevents a negative case from changing the next case's input.
func cloneStorageLayoutTest(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var copy map[string]any
	if err := json.Unmarshal(data, &copy); err != nil {
		t.Fatal(err)
	}
	return copy
}

// Add the nested container edges which use the same nominal declarations.
func nestedStorageLayoutTest(t *testing.T) map[string]any {
	t.Helper()
	layout := evidenceStorageLayoutTest(t)
	types := layout["types"].(map[string]any)
	for index, id := range []string{"58897", "59634"} {
		domain := "t_struct(Domain)" + id + "_storage"
		array := "t_array(" + domain + ")2_storage"
		mapping := "t_mapping(t_bytes32," + array + ")"
		types[array] = map[string]any{"base": domain, "encoding": "inplace", "label": fmt.Sprintf("domain array %d", index), "numberOfBytes": "512"}
		types[mapping] = map[string]any{"key": "t_bytes32", "value": array, "encoding": "mapping", "label": fmt.Sprintf("domain mapping %d", index), "numberOfBytes": "32"}
		layout["storage"] = append(layout["storage"].([]any), map[string]any{"astId": float64(index + 1), "contract": "fixture", "label": fmt.Sprintf("nested%d", index), "offset": float64(0), "slot": strconv.Itoa(index + 2), "type": mapping})
	}
	return layout
}

// Renumber every declaration and reference, including IDs inside containers.
func renumberStorageLayoutTest(t *testing.T, value any) any {
	t.Helper()
	rename := func(text string) string {
		return foundryTypeASTID.ReplaceAllStringFunc(text, func(raw string) string {
			match := foundryTypeASTID.FindStringSubmatch(raw)
			id, err := strconv.Atoi(strings.TrimPrefix(raw, match[1]))
			if err != nil {
				t.Fatal(err)
			}
			return match[1] + strconv.Itoa(id+100000)
		})
	}
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range typed {
			if key == "astId" {
				out[key] = float64(900000)
				continue
			}
			if key == "contract" {
				out[key] = "unrelated-test-graph:Fixture"
				continue
			}
			out[rename(key)] = renumberStorageLayoutTest(t, child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			out[index] = renumberStorageLayoutTest(t, child)
		}
		return out
	case string:
		return rename(typed)
	default:
		return value
	}
}

// Fail deterministically on the old collision even if its random overwrite
// happens to choose the same Domain in two consecutive hash computations.
func TestNormalizeFoundryStorageLayoutPreservesBothEvidenceDomains(t *testing.T) {
	layout := evidenceStorageLayoutTest(t)
	before, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := normalizeFoundryStorageLayout(layout)
	if err != nil {
		t.Fatal(err)
	}
	types := normalized.(map[string]any)["types"].(map[string]any)
	if len(types) != 14 {
		t.Fatalf("evidence layout lost a declared type: got %d want 14", len(types))
	}
	evidenceName := "t_struct(ValidatorEvidence.Domain)_storage"
	activationName := "t_struct(ValidatorEvidenceActivation.Domain)_storage"
	evidence, evidenceOK := types[evidenceName].(map[string]any)
	activation, activationOK := types[activationName].(map[string]any)
	if !evidenceOK || !activationOK || evidence["numberOfBytes"] != "256" || activation["numberOfBytes"] != "224" {
		t.Fatal("distinct evidence and activation domains did not retain their full identity and shape")
	}
	if len(evidence["members"].([]any)) != 9 || len(activation["members"].([]any)) != 8 {
		t.Fatal("domain fields were merged or dropped")
	}
	for _, binding := range []struct{ holder, domain string }{
		{holder: "t_struct(Header)_storage", domain: evidenceName},
		{holder: "t_struct(Record)_storage", domain: activationName},
	} {
		member := types[binding.holder].(map[string]any)["members"].([]any)[0].(map[string]any)
		if member["type"] != binding.domain {
			t.Fatalf("%s references %v, want %s", binding.holder, member["type"], binding.domain)
		}
	}
	after, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("normalization mutated the authenticated input")
	}
}

// Compiler renumbering changes neither complete type identity nor artifact hash.
func TestCanonicalArtifactHashStableWithQualifiedTypeRenumbering(t *testing.T) {
	layout := nestedStorageLayoutTest(t)
	first, err := normalizeFoundryStorageLayout(layout)
	if err != nil {
		t.Fatal(err)
	}
	second, err := normalizeFoundryStorageLayout(renumberStorageLayoutTest(t, layout))
	if err != nil {
		t.Fatal(err)
	}
	left, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatal("compiler renumbering changed the complete normalized type graph")
	}
	if len(first.(map[string]any)["types"].(map[string]any)) != 18 {
		t.Fatal("nested array or mapping type was lost")
	}
	candidate := testContractItem(t, "Evidence", "6001", strings.Repeat("11", 32))
	candidate.Artifact.StorageLayout, err = json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	hash := canonicalArtifactHash(candidate.Artifact, candidate.References)
	candidate.Artifact.StorageLayout, err = json.Marshal(renumberStorageLayoutTest(t, layout))
	if err != nil {
		t.Fatal(err)
	}
	if canonicalArtifactHash(candidate.Artifact, candidate.References) != hash {
		t.Fatal("AST-only graph drift changed the artifact hash")
	}
}

// Both formerly colliding types, and every container/member edge, remain hashed.
func TestCanonicalArtifactHashPinsBothDomainsAndNestedReferences(t *testing.T) {
	layout := nestedStorageLayoutTest(t)
	candidate := testContractItem(t, "Evidence", "6002", strings.Repeat("22", 32))
	raw, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Artifact.StorageLayout = raw
	baseline := canonicalArtifactHash(candidate.Artifact, candidate.References)
	for _, change := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "evidence domain member type", mutate: func(types map[string]any) {
			types["t_struct(Domain)58897_storage"].(map[string]any)["members"].([]any)[8].(map[string]any)["type"] = "t_uint64"
		}},
		{name: "activation domain member slot", mutate: func(types map[string]any) {
			types["t_struct(Domain)59634_storage"].(map[string]any)["members"].([]any)[7].(map[string]any)["slot"] = "7"
		}},
		{name: "nested member offset", mutate: func(types map[string]any) {
			types["t_struct(Domain)59634_storage"].(map[string]any)["members"].([]any)[3].(map[string]any)["offset"] = float64(3)
		}},
		{name: "header domain reference", mutate: func(types map[string]any) {
			types["t_struct(Header)58931_storage"].(map[string]any)["members"].([]any)[0].(map[string]any)["type"] = "t_struct(Domain)59634_storage"
		}},
		{name: "array base", mutate: func(types map[string]any) {
			types["t_array(t_struct(Domain)58897_storage)2_storage"].(map[string]any)["base"] = "t_struct(Domain)59634_storage"
		}},
		{name: "mapping value", mutate: func(types map[string]any) {
			types["t_mapping(t_bytes32,t_array(t_struct(Domain)58897_storage)2_storage)"].(map[string]any)["value"] = "t_array(t_struct(Domain)59634_storage)2_storage"
		}},
		{name: "type width", mutate: func(types map[string]any) {
			types["t_struct(Domain)58897_storage"].(map[string]any)["numberOfBytes"] = "288"
		}},
		{name: "type encoding", mutate: func(types map[string]any) {
			types["t_struct(Domain)59634_storage"].(map[string]any)["encoding"] = "bytes"
		}},
	} {
		copy := cloneStorageLayoutTest(t, layout)
		change.mutate(copy["types"].(map[string]any))
		candidate.Artifact.StorageLayout, err = json.Marshal(copy)
		if err != nil {
			t.Fatal(err)
		}
		if canonicalArtifactHash(candidate.Artifact, candidate.References) == baseline {
			t.Errorf("%s escaped the artifact hash", change.name)
		}
	}
}

// A type label is disambiguation evidence, not authority to merge declarations.
func TestNormalizeFoundryStorageLayoutRejectsUnresolvableTypeIdentities(t *testing.T) {
	for _, change := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "same qualified label", mutate: func(types map[string]any) {
			types["t_struct(Domain)59634_storage"].(map[string]any)["label"] = "struct ValidatorEvidence.Domain"
		}},
		{name: "missing qualified label", mutate: func(types map[string]any) { delete(types["t_struct(Domain)59634_storage"].(map[string]any), "label") }},
		{name: "wrong label kind", mutate: func(types map[string]any) {
			types["t_struct(Domain)59634_storage"].(map[string]any)["label"] = "contract ValidatorEvidenceActivation.Domain"
		}},
		{name: "undeclared nominal reference", mutate: func(types map[string]any) {
			types["t_struct(Header)58931_storage"].(map[string]any)["members"].([]any)[0].(map[string]any)["type"] = "t_struct(Domain)77777_storage"
		}},
		{name: "qualified identity aliases another declaration", mutate: func(types map[string]any) {
			types["t_struct(ValidatorEvidence.Domain)88888_storage"] = map[string]any{"label": "struct Third.Domain", "encoding": "inplace", "numberOfBytes": "32"}
		}},
	} {
		layout := evidenceStorageLayoutTest(t)
		change.mutate(layout["types"].(map[string]any))
		if normalized, err := normalizeFoundryStorageLayout(layout); err == nil || normalized != nil {
			t.Errorf("%s was not refused without partial output: %v", change.name, err)
		}
	}
}

// The generic nominal-ID grammar also covers contracts, enums and value types.
func TestNormalizeFoundryStorageLayoutSeparatesAllNominalTypeKinds(t *testing.T) {
	for _, kind := range []string{"struct", "contract", "enum", "userDefinedValueType"} {
		prefix := kind + " "
		if kind == "userDefinedValueType" {
			prefix = ""
		}
		first, second := "t_"+kind+"(Value)12", "t_"+kind+"(Value)34"
		layout := map[string]any{
			"storage": []any{map[string]any{"type": first}, map[string]any{"type": second}},
			"types":   map[string]any{first: map[string]any{"label": prefix + "First.Value"}, second: map[string]any{"label": prefix + "Second.Value"}},
		}
		normalized, err := normalizeFoundryStorageLayout(layout)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		fields := normalized.(map[string]any)
		if len(fields["types"].(map[string]any)) != 2 || fields["storage"].([]any)[0].(map[string]any)["type"] == fields["storage"].([]any)[1].(map[string]any)["type"] {
			t.Errorf("%s declarations or references collapsed", kind)
		}
	}
}

// Metadata-only preservation must authenticate a complete, collision-free
// layout; it must not silently retain bytes against only one domain's shape.
func TestPreserveReviewedBytecodePinsCompleteEvidenceTypeGraph(t *testing.T) {
	candidate := testContractItem(t, "Evidence", "6003", strings.Repeat("44", 32))
	var err error
	candidate.Artifact.StorageLayout, err = json.Marshal(evidenceStorageLayoutTest(t))
	if err != nil {
		t.Fatal(err)
	}
	candidate.ArtifactHash = canonicalArtifactHash(candidate.Artifact, candidate.References)
	reviewed := reviewedContractItem(t, candidate, metadataBytecode("600301", strings.Repeat("33", 32)), metadataBytecode("600302", strings.Repeat("33", 32)))
	existing, err := renderContractArtifacts([]item{reviewed})
	if err != nil {
		t.Fatal(err)
	}
	preserved, err := preserveReviewedBytecode("contracts_gen.go", existing, []item{candidate})
	if err != nil {
		t.Fatal(err)
	}
	if preserved[0].Creation != reviewed.Creation || preserved[0].Runtime != reviewed.Runtime || preserved[0].ArtifactHash != reviewed.ArtifactHash {
		t.Fatal("complete metadata-equivalent evidence graph was not preserved")
	}
	for _, domain := range []string{"t_struct(Domain)58897_storage", "t_struct(Domain)59634_storage"} {
		layout := evidenceStorageLayoutTest(t)
		layout["types"].(map[string]any)[domain].(map[string]any)["numberOfBytes"] = "999"
		changed := candidate
		changed.Artifact.StorageLayout, err = json.Marshal(layout)
		if err != nil {
			t.Fatal(err)
		}
		changed.ArtifactHash = canonicalArtifactHash(changed.Artifact, changed.References)
		result, err := preserveReviewedBytecode("contracts_gen.go", existing, []item{changed})
		if err != nil {
			t.Fatal(err)
		}
		if result[0].Creation != changed.Creation || result[0].Runtime != changed.Runtime || result[0].ArtifactHash != changed.ArtifactHash {
			t.Errorf("%s semantic drift retained old deployment bytes", domain)
		}
	}
}
