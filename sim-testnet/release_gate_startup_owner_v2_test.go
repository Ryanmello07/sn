package main

// Startup owner controls must run inside the real settlement phase in both
// modes; compiling the package does not exercise the ownership boundary.
import (
	"os"
	"strings"
	"testing"
)

// Read every declaration in the actual fixture source, including the public
// RunRelease paths, and reject the original missing-owner selector.
func TestProducerGateStateSelectionCoversActualStartupOwnersV2(t *testing.T) {
	t.Parallel()
	encoded, err := os.ReadFile("../scripts/test-release-1.0-producer-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(encoded)
	group := releaseEvidenceV2GateGroup{
		phase: "settlement", variable: "producer_tests", alternatives: "ReleaseStartupOwnerV2|RunRelease",
		packages: []string{"./validator"},
		sources:  map[string][]string{"./validator": releaseEvidenceV2GateSources(t, []string{"../validator/release_startup_owner_v2_test.go"})},
		commands: []string{`go test ./validator -run "$producer_tests" -count=1 -parallel=4 -timeout 90m`, `go test -race ./validator -run "$producer_tests" -count=1 -parallel=4 -timeout 90m`},
	}
	if err := verifyReleaseEvidenceV2GateGroup(script, group); err != nil {
		t.Fatal(err)
	}
	prior := strings.Replace(script, "|ReleaseStartupOwnerV2|", "|", 1)
	if prior == script || verifyReleaseEvidenceV2GateGroup(prior, group) == nil {
		t.Fatal("original producer omission acquired startup owner coverage")
	}
	for _, command := range group.commands {
		changed := strings.Replace(script, command, "# omitted: "+command, 1)
		if changed == script || verifyReleaseEvidenceV2GateGroup(changed, group) == nil {
			t.Fatalf("startup owner coverage survived removed invocation %s", command)
		}
	}
}
