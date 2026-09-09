// Full history populations retain independent process deadlines and exact
// source coverage, with private database state under the gate's service owner.
package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Inspect executable phase bodies and their service-gated admission, not
// comments or a broad selector that silently skips its largest members.
func verifyReleaseGateHistoryPopulationIsolation(script string) error {
	const skip = "^TestStClientKeyHistoryBatchActualFullPopulation(SharedBoundary|DistinctBoundaries)$"
	phasePattern := func(name string) *regexp.Regexp {
		return regexp.MustCompile("(?ms)^[\\t ]*release_phase_" + regexp.QuoteMeta(name) + "\\(\\) \\{\\n(.*?)^[\\t ]*\\}[\\t ]*$")
	}
	ordinary := phasePattern("server_db").FindAllStringSubmatch(script, -1)
	if len(ordinary) != 1 {
		return fmt.Errorf("history partition needs one ordinary database owner")
	}
	selector, err := releaseConnectPolicySelectorAssignment(ordinary[0][1], "evidence_source_tests")
	if err != nil {
		return err
	}
	var domainCommand string
	for _, line := range strings.Split(ordinary[0][1], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "export WARP_DOMAIN=") {
			if domainCommand != "" {
				return fmt.Errorf("history partition has ambiguous application ownership")
			}
			domainCommand = line
		}
	}
	if domainCommand == "" {
		return fmt.Errorf("history partition lacks the ordinary application owner")
	}
	var actualCommands []string
	for _, line := range strings.Split(ordinary[0][1], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `go test ./controller -run "$evidence_source_tests"`) || strings.HasPrefix(line, `go test -race ./controller -run "$evidence_source_tests"`) {
			actualCommands = append(actualCommands, line)
		}
	}
	expectedCommands := []string{
		`go test ./controller -run "$evidence_source_tests" -count=1 -skip '` + skip + `' -timeout 10m`,
		`go test -race ./controller -run "$evidence_source_tests" -count=1 -skip '` + skip + `' -timeout 10m`,
	}
	if !slices.Equal(actualCommands, expectedCommands) {
		return fmt.Errorf("history partition changed its exact ordinary exclusion or budgets")
	}
	for _, population := range []struct {
		phase string
		root  string
	}{
		{phase: "shared", root: "TestStClientKeyHistoryBatchActualFullPopulationSharedBoundary"},
		{phase: "distinct", root: "TestStClientKeyHistoryBatchActualFullPopulationDistinctBoundaries"},
	} {
		selected, err := regexp.MatchString(selector, population.root)
		if err != nil || !selected {
			return fmt.Errorf("history partition lost its ordinary source family: %v", err)
		}
		phase := "server_history_" + population.phase
		variable := phase + "_tests"
		functions := phasePattern(phase).FindAllStringSubmatch(script, -1)
		if len(functions) != 1 {
			return fmt.Errorf("history partition needs exactly one %s owner", phase)
		}
		var commands []string
		for _, line := range strings.Split(functions[0][1], "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				commands = append(commands, line)
			}
		}
		expected := []string{
			`cd "$workspace/server"`,
			`export WARP_ENV=local`, `export WARP_SERVICE=test`,
			domainCommand, `export WARP_BLOCK=test`, `export WARP_VERSION=0.0.0`,
			`source "$RELEASE_GATE_SERVICE_ENV"`, `source "$workspace/server/test-env.sh"`,
			`test_env_validate_suite_resource_manifest "$TEST_ENV_SUITE_RESOURCE_MANIFEST" "$WARP_VAULT_HOME" "$WARP_CONFIG_HOME"`,
			variable + "='^" + population.root + "$'",
			`go test ./controller -run "$` + variable + `" -count=1 -timeout 10m`,
			`go test -race ./controller -run "$` + variable + `" -count=1 -timeout 10m`,
		}
		if !slices.Equal(commands, expected) {
			return fmt.Errorf("history partition changed %s services, selection or executable modes", phase)
		}
	}
	// No population starts outside the same checked service-ready conditional.
	const registration = `if [[ "$release_gate_services_ready" == 1 ]]; then
  release_gate_start server-db release_phase_server_db
  release_gate_start server-history-shared release_phase_server_history_shared
  release_gate_start server-history-distinct release_phase_server_history_distinct
else
  release_gate_start server-db release_gate_unavailable_services
  release_gate_start server-history-shared release_gate_unavailable_services
  release_gate_start server-history-distinct release_gate_unavailable_services
fi`
	definitions := regexp.MustCompile(`(?ms)^[\t ]*release_phase_[a-z0-9_]+\(\) \{\n.*?^[\t ]*\}[\t ]*$`)
	outside := definitions.ReplaceAllString(script, "")
	var lines []string
	for _, line := range strings.Split(outside, "\n") {
		lines = append(lines, strings.TrimSpace(line))
	}
	normalized := strings.Join(lines, "\n")
	var registry []string
	for _, line := range strings.Split(registration, "\n") {
		registry = append(registry, strings.TrimSpace(line))
	}
	if strings.Count(normalized, strings.Join(registry, "\n")) != 1 {
		return fmt.Errorf("history populations lack exact service-gated admission")
	}
	const optionalDatabase = `if [[ "${RUN_SERVER_DB_TESTS:-0}" == "1" ]]; then`
	var expectedConditions []string
	if strings.Contains(normalized, optionalDatabase+"\n") {
		expectedConditions = []string{optionalDatabase}
	}
	conditions, err := releaseGateRegistrationConditions(script, `if [[ "$release_gate_services_ready" == 1 ]]; then`)
	if err != nil || !slices.Equal(conditions, expectedConditions) {
		return fmt.Errorf("history populations have hidden service admission: %v %v", conditions, err)
	}
	for _, name := range []string{"shared", "distinct"} {
		for _, function := range []string{"release_phase_server_history_" + name, "release_gate_unavailable_services"} {
			line := "release_gate_start server-history-" + name + " " + function
			if strings.Count(normalized, line+"\n") != 1 {
				return fmt.Errorf("history population has duplicated or foreign admission")
			}
		}
	}
	return nil
}

// Both strict gates retain every source-declared full-population root, each
// with its own real normal/race process and original ten-minute allowance.
func TestProducerGateStateSelectionRequiresIsolatedHistoryPopulations(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("../../server/controller/st_client_key_history_batch_test.go")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := releaseSelectedTestDeclarations("^Test", []string{string(source)})
	expected := []string{"TestStClientKeyHistoryBatchActualFullPopulationDistinctBoundaries", "TestStClientKeyHistoryBatchActualFullPopulationSharedBoundary"}
	if err != nil || !slices.Equal(actual, expected) {
		t.Fatalf("history population source census changed: %v %v", actual, err)
	}
	for _, path := range []string{"../scripts/test-release-1.0-producer-gate.sh", "../scripts/test-release-1.0-local.sh"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := verifyReleaseGateHistoryPopulationIsolation(string(raw)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
}

// The old combined serial owner and adjacent exclusion, failure masking,
// deadline and service-admission errors are rejected deterministically.
func TestProducerGateStateSelectionRejectsHistoryPopulationPartitionDrift(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"../scripts/test-release-1.0-producer-gate.sh", "../scripts/test-release-1.0-local.sh"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		script := string(raw)
		if err := verifyReleaseGateHistoryPopulationIsolation(script); err != nil {
			t.Fatal(err)
		}
		const skip = " -skip '^TestStClientKeyHistoryBatchActualFullPopulation(SharedBoundary|DistinctBoundaries)$'"
		if err := verifyReleaseGateHistoryPopulationIsolation(strings.ReplaceAll(script, skip, "")); err == nil {
			t.Fatal("history partition accepted the original combined serial owner")
		}
		for _, name := range []string{"shared", "distinct"} {
			start := "release_gate_start server-history-" + name + " release_phase_server_history_" + name
			for _, replacement := range []string{"# " + start, start + "\n" + start, strings.Replace(start, "release_phase_server_history_"+name, "release_phase_server_db", 1)} {
				if err := verifyReleaseGateHistoryPopulationIsolation(strings.Replace(script, start, replacement, 1)); err == nil {
					t.Fatal("history partition accepted changed job admission", name, replacement)
				}
			}
			for _, prefix := range []string{"go test ", "go test -race "} {
				command := prefix + `./controller -run "$server_history_` + name + `_tests" -count=1 -timeout 10m`
				if strings.Count(script, command) != 1 {
					t.Fatal("mutation lost its original population command", command)
				}
				for _, replacement := range []string{"# " + command, command + " || true", command + " -run '^$'", strings.Replace(command, "-timeout 10m", "-timeout 0", 1), strings.Replace(command, "-count=1", "-count=0", 1), strings.Replace(command, "./controller", "./model", 1), "if false; then\n" + command + "\nfi"} {
					if err := verifyReleaseGateHistoryPopulationIsolation(strings.Replace(script, command, replacement, 1)); err == nil {
						t.Fatal("history partition accepted altered execution", replacement)
					}
				}
			}
		}
		for _, replacement := range []string{" -skip '^Test'", skip + " -skip '^Test'"} {
			if err := verifyReleaseGateHistoryPopulationIsolation(strings.Replace(script, skip, replacement, 1)); err == nil {
				t.Fatal("history partition accepted broadened exclusion")
			}
		}
		if err := verifyReleaseGateHistoryPopulationIsolation(strings.Replace(script, `if [[ "$release_gate_services_ready" == 1 ]]; then`, "if true; then", 1)); err == nil {
			t.Fatal("history partition accepted unverified services")
		}
		const start = `if [[ "$release_gate_services_ready" == 1 ]]; then`
		pattern := regexp.MustCompile("(?ms)^[\\t ]*" + regexp.QuoteMeta(start) + "\\n.*?^[\\t ]*fi[\\t ]*$")
		blocks := pattern.FindAllString(script, -1)
		if len(blocks) != 1 {
			t.Fatal("mutation lost its exact service-ready block")
		}
		for _, wrapper := range []struct {
			start string
			end   string
		}{
			{start: "if false; then", end: "fi"},
			{start: "if true; then", end: "fi"},
			{start: "for omitted in; do", end: "done"},
			{start: "{", end: "}"},
		} {
			changed := wrapper.start + "\n" + blocks[0] + "\n" + wrapper.end
			if err := verifyReleaseGateHistoryPopulationIsolation(strings.Replace(script, blocks[0], changed, 1)); err == nil {
				t.Fatal("history partition accepted an enclosing registry branch", wrapper.start)
			}
		}
	}
}
