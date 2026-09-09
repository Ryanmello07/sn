// Heavy original metadata and ordinary capture own explicit scoped budgets.
// Excluding the exact metadata root is valid only with its admitted successor.
package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Inspect the gates' bounded line-oriented registry grammar, after removing
// declarations. Exact calls inside an extra branch are not admitted jobs.
func releaseGateRegistrationConditions(script string, registration string) ([]string, error) {
	definitions := regexp.MustCompile(`(?ms)^[\t ]*[a-zA-Z_][a-zA-Z0-9_]*\(\) \{\n.*?^[\t ]*\}[\t ]*$`)
	registry := definitions.ReplaceAllString(script, "")
	var conditions []string
	var registrationConditions []string
	count := 0
	for _, line := range strings.Split(registry, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == registration {
			count++
			registrationConditions = slices.Clone(conditions)
		}
		// Nothing after the first registration can change its ancestry. Count
		// later duplicates without parsing unrelated final-fence shell syntax.
		if count != 0 {
			continue
		}
		switch {
		case strings.HasPrefix(line, "if "):
			if !strings.HasSuffix(line, "; then") || len(conditions) >= 16 {
				return nil, fmt.Errorf("release registration has unsupported conditional syntax")
			}
			conditions = append(conditions, line)
		case line == "else", strings.HasPrefix(line, "elif "):
			if len(conditions) == 0 {
				return nil, fmt.Errorf("release registration has an unowned conditional branch")
			}
			conditions[len(conditions)-1] = line
		case line == "fi":
			if len(conditions) == 0 {
				return nil, fmt.Errorf("release registration has an unowned conditional end")
			}
			conditions = conditions[:len(conditions)-1]
		case line == "(", line == "{", strings.HasPrefix(line, "for "), strings.HasPrefix(line, "while "), strings.HasPrefix(line, "until "), strings.HasPrefix(line, "select "), strings.HasPrefix(line, "case "):
			return nil, fmt.Errorf("release registration has unsupported enclosing control flow")
		}
	}
	if count != 1 {
		return nil, fmt.Errorf("release registration needs exactly one executable call")
	}
	return registrationConditions, nil
}

// Bind actual phase commands and registry entries, not comments or dead text.
// The ordinary selector remains protected by the complete source-family guards.
func verifyReleaseGateCaptureMetadataIsolation(script string) error {
	const fullRoot = "TestCampaignEvidenceCapacityV2MetadataFullCensusMaterializesFlatWireAndCarrier"
	const fullSelector = "^" + fullRoot + "$"
	definitions := regexp.MustCompile(`(?ms)^[\t ]*release_phase_[a-z0-9_]+\(\) \{\n.*?^[\t ]*\}[\t ]*$`)
	registry := definitions.ReplaceAllString(script, "")
	for _, group := range []struct {
		phase       string
		job         string
		variable    string
		skip        string
		raceTimeout string
	}{
		{phase: "capture", job: "capture", variable: "capture_tests", skip: " -skip '" + fullSelector + "'", raceTimeout: "10m"},
		{phase: "capture_metadata", job: "capture-metadata", variable: "capture_metadata_tests", raceTimeout: "45m"},
	} {
		function := "release_phase_" + group.phase
		pattern := regexp.MustCompile("(?ms)^[\\t ]*" + function + "\\(\\) \\{\\n(.*?)^[\\t ]*\\}[\\t ]*$")
		phases := pattern.FindAllStringSubmatch(script, -1)
		if len(phases) != 1 {
			return fmt.Errorf("capture metadata needs exactly one %s phase", group.phase)
		}
		selector, err := releaseConnectPolicySelectorAssignment(phases[0][1], group.variable)
		if err != nil {
			return err
		}
		selected, err := regexp.MatchString(selector, fullRoot)
		if err != nil || !selected || group.phase == "capture_metadata" && selector != fullSelector {
			return fmt.Errorf("capture metadata changed its exact partition: %v", err)
		}
		var commands []string
		for _, line := range strings.Split(phases[0][1], "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				commands = append(commands, line)
			}
		}
		expected := []string{
			`cd "$sn_repo"`,
			group.variable + "='" + selector + "'",
			`go test ./sim-testnet -run "$` + group.variable + `" -count=1` + group.skip + " -timeout 5m",
			`go test -race ./sim-testnet -run "$` + group.variable + `" -count=1` + group.skip + " -timeout " + group.raceTimeout,
		}
		if !slices.Equal(commands, expected) {
			return fmt.Errorf("capture metadata changed %s execution or its scoped budgets", group.phase)
		}
		start := "release_gate_start " + group.job + " " + function
		invocation := regexp.MustCompile("(?m)^" + regexp.QuoteMeta(start) + "[\\t ]*$")
		calls := invocation.FindAllStringIndex(script, -1)
		definition := pattern.FindStringIndex(script)
		if len(calls) != 1 || len(invocation.FindAllString(registry, -1)) != 1 || calls[0][0] < definition[1] {
			return fmt.Errorf("capture metadata does not independently admit %s", group.phase)
		}
		conditions, err := releaseGateRegistrationConditions(script, start)
		if err != nil || len(conditions) != 0 {
			return fmt.Errorf("capture metadata has conditional job admission: %v %v", conditions, err)
		}
	}
	return nil
}

// The full census is neither duplicated inside ordinary capture nor omitted
// from the gate; each owner retains its independently reviewed mode budgets.
func TestProducerGateCaptureSelectionRequiresIndependentMetadata(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../scripts/test-release-1.0-producer-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseGateCaptureMetadataIsolation(string(raw)); err != nil {
		t.Fatal(err)
	}
	metadataSource, err := os.ReadFile("evidence_metadata_census_v2_test.go")
	if err != nil {
		t.Fatal(err)
	}
	const selector = "^TestCampaignEvidenceCapacityV2MetadataFullCensusMaterializesFlatWireAndCarrier$"
	declarations, err := releaseSelectedTestDeclarations(selector, []string{string(metadataSource)})
	if err != nil || len(declarations) != 1 {
		t.Fatalf("independent metadata phase lost its exact source root: %v", err)
	}
}

// Recreate the old combined aggregate and adjacent ways an exclusion could
// become an omitted root, broadened deadline, hidden failure or unowned job.
func TestProducerGateCaptureSelectionRejectsMetadataPartitionDrift(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../scripts/test-release-1.0-producer-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	if err := verifyReleaseGateCaptureMetadataIsolation(script); err != nil {
		t.Fatal(err)
	}
	const skip = " -skip '^TestCampaignEvidenceCapacityV2MetadataFullCensusMaterializesFlatWireAndCarrier$'"
	if err := verifyReleaseGateCaptureMetadataIsolation(strings.ReplaceAll(script, skip, "")); err == nil {
		t.Fatal("capture accepted the old combined serial stress owner")
	}
	for _, pair := range []struct {
		old     string
		changed string
	}{
		{old: skip, changed: " -skip '^TestCampaignEvidence'"},
		{old: skip, changed: skip + " -skip '^Test'"},
		{old: "capture_metadata_tests='^TestCampaignEvidenceCapacityV2MetadataFullCensusMaterializesFlatWireAndCarrier$'", changed: "capture_metadata_tests='^TestCampaignEvidenceCapacityV2MetadataFullCensusMaterializesFlatWireAndCarrier'"},
		{old: "release_gate_start capture-metadata release_phase_capture_metadata", changed: "# release_gate_start capture-metadata release_phase_capture_metadata"},
		{old: "release_gate_start capture-metadata release_phase_capture_metadata", changed: "release_gate_start capture-metadata release_phase_capture"},
		{old: "release_gate_start capture-metadata release_phase_capture_metadata", changed: "release_gate_start capture-metadata release_phase_capture_metadata\nrelease_gate_start capture-metadata release_phase_capture_metadata"},
		{old: "release_gate_start capture-metadata release_phase_capture_metadata", changed: "release_phase_unused() {\nrelease_gate_start capture-metadata release_phase_capture_metadata\n}"},
		{old: "release_gate_start capture-metadata release_phase_capture_metadata", changed: "if false; then\nrelease_gate_start capture-metadata release_phase_capture_metadata\nfi"},
		{old: "release_gate_start capture-metadata release_phase_capture_metadata", changed: "for omitted in; do\nrelease_gate_start capture-metadata release_phase_capture_metadata\ndone"},
		{old: "release_gate_start capture-metadata release_phase_capture_metadata", changed: "{\nrelease_gate_start capture-metadata release_phase_capture_metadata\n}"},
	} {
		if !strings.Contains(script, pair.old) {
			t.Fatal("mutation lost its actual source prerequisite", pair.old)
		}
		if err := verifyReleaseGateCaptureMetadataIsolation(strings.Replace(script, pair.old, pair.changed, 1)); err == nil {
			t.Fatal("capture accepted altered metadata partition", pair.changed)
		}
	}
	for _, variable := range []string{"capture_tests", "capture_metadata_tests"} {
		for _, race := range []bool{false, true} {
			command := "go test"
			timeout := "5m"
			if race {
				command += " -race"
				timeout = "10m"
				if variable == "capture_metadata_tests" {
					timeout = "45m"
				}
			}
			command += ` ./sim-testnet -run "$` + variable + `" -count=1`
			if variable == "capture_tests" {
				command += skip
			}
			command += " -timeout " + timeout
			for _, replacement := range []string{
				"# " + command, command + " || true", command + " -run '^$'",
				strings.Replace(command, "-timeout "+timeout, "-timeout 0", 1),
				strings.Replace(command, "-count=1", "-count=0", 1),
				strings.Replace(command, "./sim-testnet", "./protocol", 1),
				"if false; then\n  " + command + "\n  fi",
			} {
				if strings.Count(script, command) != 1 {
					t.Fatal("mutation does not identify one original command", command)
				}
				if err := verifyReleaseGateCaptureMetadataIsolation(strings.Replace(script, command, replacement, 1)); err == nil {
					t.Fatal("capture accepted altered metadata execution", replacement)
				}
			}
		}
	}
}

// The actual complete metadata command receives only its measured race owner.
// Restoring the failed ten-minute allowance must fail before any gate body.
func TestProducerGateCaptureSelectionPinsMeasuredMetadataRaceBudget(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../scripts/test-release-1.0-producer-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	if err := verifyReleaseGateCaptureMetadataIsolation(script); err != nil {
		t.Fatalf("full metadata race lacks its measured 45-minute owner: %v", err)
	}
	const command = `go test -race ./sim-testnet -run "$capture_metadata_tests" -count=1 -timeout 45m`
	if strings.Count(script, command) != 1 {
		t.Fatal("full metadata race command is not uniquely executable")
	}
	old := strings.Replace(script, command, strings.Replace(command, "45m", "10m", 1), 1)
	if err := verifyReleaseGateCaptureMetadataIsolation(old); err == nil {
		t.Fatal("full metadata race accepted the previously failed ten-minute owner")
	}
}

// A larger full-metadata race owner must not leak to normal, ordinary capture,
// selector breadth, ignored failures, or the separately admitted full packages.
func TestProducerGateCaptureSelectionRejectsMetadataRaceBudgetLeak(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../scripts/test-release-1.0-producer-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	if err := verifyReleaseGateCaptureMetadataIsolation(script); err != nil {
		t.Fatal(err)
	}
	const metadataRace = `go test -race ./sim-testnet -run "$capture_metadata_tests" -count=1 -timeout 45m`
	const metadataNormal = `go test ./sim-testnet -run "$capture_metadata_tests" -count=1 -timeout 5m`
	const captureNormal = `go test ./sim-testnet -run "$capture_tests" -count=1 -skip '^TestCampaignEvidenceCapacityV2MetadataFullCensusMaterializesFlatWireAndCarrier$' -timeout 5m`
	const captureRace = `go test -race ./sim-testnet -run "$capture_tests" -count=1 -skip '^TestCampaignEvidenceCapacityV2MetadataFullCensusMaterializesFlatWireAndCarrier$' -timeout 10m`
	for _, change := range []struct {
		name        string
		original    string
		replacement string
	}{
		{name: "metadata implicit timeout", original: metadataRace, replacement: strings.Replace(metadataRace, " -timeout 45m", "", 1)},
		{name: "metadata unbounded timeout", original: metadataRace, replacement: strings.Replace(metadataRace, "45m", "0", 1)},
		{name: "metadata diagnostic timeout", original: metadataRace, replacement: strings.Replace(metadataRace, "45m", "90m", 1)},
		{name: "metadata normal budget leak", original: metadataNormal, replacement: strings.Replace(metadataNormal, "5m", "45m", 1)},
		{name: "ordinary normal budget leak", original: captureNormal, replacement: strings.Replace(captureNormal, "5m", "45m", 1)},
		{name: "ordinary race budget leak", original: captureRace, replacement: strings.Replace(captureRace, "10m", "45m", 1)},
		{name: "broader metadata selector", original: metadataRace, replacement: strings.Replace(metadataRace, `"$capture_metadata_tests"`, "'^TestCampaignEvidence'", 1)},
		{name: "hidden metadata failure", original: metadataRace, replacement: metadataRace + " || true"},
	} {
		if strings.Count(script, change.original) != 1 {
			t.Fatalf("%s lost its unique command", change.name)
		}
		mutated := strings.Replace(script, change.original, change.replacement, 1)
		if err := verifyReleaseGateCaptureMetadataIsolation(mutated); err == nil {
			t.Fatalf("%s escaped the exact phase budget", change.name)
		}
	}
	localRaw, err := os.ReadFile("../scripts/test-release-1.0-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	localScript := string(localRaw)
	if err := verifyReleaseGateFullValidatorRace(localScript); err != nil {
		t.Fatalf("metadata budget changed an independent full-package owner: %v", err)
	}
	const fullRace = "go test -race -parallel=4 -timeout 90m ./sim-testnet"
	if strings.Count(localScript, fullRace) != 1 {
		t.Fatal("full simulator race command is not uniquely executable")
	}
	changedFull := strings.Replace(localScript, fullRace, strings.Replace(fullRace, "90m", "45m", 1), 1)
	if err := verifyReleaseGateFullValidatorRace(changedFull); err == nil {
		t.Fatal("metadata race budget replaced the independent full-simulator allowance")
	}
}
