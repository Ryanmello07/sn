// The producer census has nearly the complete validator population. Its
// aggregate deadline must cover that work without changing per-root limits.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Pin actual executable commands inside one admitted owner. Separate source
// census guards retain every family; this guard cannot certify dead shell text.
func verifyReleaseGateProducerSettlementBudget(script string) error {
	const function = "release_phase_settlement"
	pattern := regexp.MustCompile("(?ms)^[\\t ]*" + function + "\\(\\) \\{\\n(.*?)^[\\t ]*\\}[\\t ]*$")
	definitions := pattern.FindAllStringSubmatch(script, -1)
	if len(definitions) != 1 {
		return fmt.Errorf("producer settlement needs exactly one phase definition")
	}
	body := definitions[0][1]
	selector, err := releaseConnectPolicySelectorAssignment(body, "producer_tests")
	if err != nil {
		return err
	}
	var commands []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			commands = append(commands, line)
		}
	}
	expected := []string{
		"cd \"$sn_repo\"",
		"producer_tests='" + selector + "'",
		"go test ./validator -run \"$producer_tests\" -count=1 -parallel=4 -timeout 90m",
		"go test -race ./validator -run \"$producer_tests\" -count=1 -parallel=4 -timeout 90m",
	}
	if len(commands) != len(expected) {
		return fmt.Errorf("producer settlement changed its executable command sequence")
	}
	for index, command := range expected {
		if commands[index] != command {
			return fmt.Errorf("producer settlement changed command %d or its scoped budget", index)
		}
	}
	invocation := regexp.MustCompile("(?m)^release_gate_start settlement " + function + "[\\t ]*$")
	calls := invocation.FindAllStringIndex(script, -1)
	allFunctions := regexp.MustCompile("(?ms)^[\\t ]*release_phase_[a-z0-9_]+\\(\\) \\{\\n.*?^[\\t ]*\\}[\\t ]*$")
	registry := allFunctions.ReplaceAllString(script, "")
	definition := pattern.FindStringIndex(script)
	if len(calls) != 1 || len(invocation.FindAllString(registry, -1)) != 1 || calls[0][0] < definition[1] {
		return fmt.Errorf("producer settlement is not admitted exactly once after its definition")
	}
	return nil
}

// The old ten-minute default is shorter than the recorded passing serial work
// alone in both modes. Require the explicit budget on the unchanged census.
func TestReleaseGateJobsRequireProducerSettlementAggregateBudget(t *testing.T) {
	t.Parallel()
	encoded, err := os.ReadFile("../scripts/test-release-1.0-producer-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseGateProducerSettlementBudget(string(encoded)); err != nil {
		t.Fatal(err)
	}
}

// Reproduce the old missing deadline independently in both modes, then cover
// adjacent omission, override, hidden failure and disconnected-owner mistakes.
func TestReleaseGateJobsRejectProducerSettlementBudgetOmissions(t *testing.T) {
	t.Parallel()
	encoded, err := os.ReadFile("../scripts/test-release-1.0-producer-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(encoded)
	if err := verifyReleaseGateProducerSettlementBudget(script); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"go test ./validator -run \"$producer_tests\" -count=1 -parallel=4 -timeout 90m",
		"go test -race ./validator -run \"$producer_tests\" -count=1 -parallel=4 -timeout 90m",
	} {
		for _, replacement := range []string{
			strings.Replace(command, " -timeout 90m", "", 1),
			strings.Replace(command, " -timeout 90m", " -timeout 10m", 1),
			strings.Replace(command, " -parallel=4", " -parallel=1", 1),
			strings.Replace(command, " -count=1", "", 1),
			strings.Replace(command, "./validator", "./protocol", 1),
			strings.Replace(command, "$producer_tests", "TestAttemptOnly", 1),
			"# " + command,
			command + " || true",
			command + " -timeout 0",
			command + " -run '^$'",
			"if false; then\n  " + command + "\n  fi",
		} {
			if strings.Count(script, command) != 1 {
				t.Fatalf("mutation does not identify one command: %s", command)
			}
			if err := verifyReleaseGateProducerSettlementBudget(strings.Replace(script, command, replacement, 1)); err == nil {
				t.Fatalf("producer budget accepted altered execution: %s", replacement)
			}
		}
	}
	const start = "release_gate_start settlement release_phase_settlement"
	for _, replacement := range []string{
		"# " + start,
		"release_gate_start settlement release_phase_framing",
		start + "\n" + start,
		"release_phase_unused() {\n" + start + "\n}",
	} {
		if strings.Count(script, start) != 1 {
			t.Fatal("mutation does not identify one settlement admission")
		}
		if err := verifyReleaseGateProducerSettlementBudget(strings.Replace(script, start, replacement, 1)); err == nil {
			t.Fatalf("producer budget accepted altered admission: %s", replacement)
		}
	}
	early := strings.Replace(script, start, "# admission moved before definition", 1)
	early = strings.Replace(early, "release_phase_settlement() {", start+"\nrelease_phase_settlement() {", 1)
	if err := verifyReleaseGateProducerSettlementBudget(early); err == nil {
		t.Fatal("producer budget admitted an undefined phase")
	}
}
