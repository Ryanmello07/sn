// Both complete gates must exercise the actual resource generator in their
// existing private-service isolation phase before its downstream controls.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Match executable whole lines inside the one existing admitted phase. A
// comment, narrowed command, alternate working directory or orphan function
// cannot supply the required generator normal/race checks.
func verifyReleaseGateSuiteFixtureChecks(script string) error {
	pattern := regexp.MustCompile(`(?ms)^release_phase_isolation_regressions\(\) \{\n(.*?)^\}\n`)
	phases := pattern.FindAllStringSubmatch(script, -1)
	if len(phases) != 1 {
		return fmt.Errorf("expected one isolation regression phase, got %d", len(phases))
	}
	body := phases[0][1]
	prefix := "  cd \"$sn_repo\"\n  go test ./scripts/server-fixture -count=1\n  go test -race ./scripts/server-fixture -count=1\n"
	if !strings.HasPrefix(body, prefix) {
		return fmt.Errorf("isolation phase omits exact generator checks in the source repository")
	}
	for _, command := range []string{"go test ./scripts/server-fixture -count=1", "go test -race ./scripts/server-fixture -count=1"} {
		invocation := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(command) + `$`)
		if len(invocation.FindAllString(body, -1)) != 1 {
			return fmt.Errorf("generator mode duplicated or omitted: %s", command)
		}
	}
	start := regexp.MustCompile(`(?m)^release_gate_start isolation-regressions release_phase_isolation_regressions$`)
	if len(start.FindAllString(script, -1)) != 1 {
		return fmt.Errorf("generator checks have no unique phase owner")
	}
	if !strings.Contains(strings.TrimPrefix(body, prefix), "  go test ./sim-testnet -run '^TestReleaseGate(Child|Jobs|Isolation)' -count=1 -parallel=4 -timeout 3m\n") {
		return fmt.Errorf("existing isolation controls lost their command or deadline")
	}
	return nil
}

// Both actual checked-in callers include the tool; merely running it once by
// hand cannot cover the recurring complete qualification payload.
func TestReleaseGateIsolationIncludesCompletePrivateFixtureTool(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"../scripts/test-release-1.0-local.sh", "../scripts/test-release-1.0-producer-gate.sh"} {
		encoded, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := verifyReleaseGateSuiteFixtureChecks(string(encoded)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
}

// Each omission reconstructs the pre-fix coverage defect deterministically;
// the original selectors, phase owner and deadlines must remain executable.
func TestReleaseGateIsolationRejectsPrivateFixtureCoverageOmissions(t *testing.T) {
	t.Parallel()
	encoded, err := os.ReadFile("../scripts/test-release-1.0-producer-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(encoded)
	if err := verifyReleaseGateSuiteFixtureChecks(script); err != nil {
		t.Fatal(err)
	}
	for _, omitted := range []string{"  cd \"$sn_repo\"\n", "  go test ./scripts/server-fixture -count=1\n", "  go test -race ./scripts/server-fixture -count=1\n", "release_gate_start isolation-regressions release_phase_isolation_regressions\n"} {
		phase := strings.Index(script, "release_phase_isolation_regressions() {\n")
		changed := script[:phase] + strings.Replace(script[phase:], omitted, "# omitted: "+omitted, 1)
		if changed == script || verifyReleaseGateSuiteFixtureChecks(changed) == nil {
			t.Fatalf("fixture mode/owner omission admitted: %q", omitted)
		}
	}
}
