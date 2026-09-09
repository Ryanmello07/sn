package main

// The independent validator evidence journal is a deployed contract, not a
// transitive dependency of coordinator analysis. Static coverage is a literal
// executable root census, not a count of names in comments.
import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

const solidityStaticDeployedRoots = "src/STCoordinator.sol\nsrc/STFleetBatcher.sol\nsrc/STValidatorEvidence.sol\nsrc/probe/STSubnetProbe.sol\nsrc/testnet/STCoordinatorAdversary.sol"

// Admit the reviewed literal array only. A comment, duplicate, nondeployed
// library or dynamic expansion cannot stand in for one actual analysis root.
func verifySolidityStaticRootCensus(script string) error {
	required := map[string]bool{}
	for _, root := range strings.Split(solidityStaticDeployedRoots, "\n") {
		required[root] = true
	}
	counts := map[string]int{}
	arrays, loops := 0, 0
	inside, closed := false, false
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "contracts=(" {
			arrays++
			if inside || arrays != 1 {
				return errors.New("static root array is duplicated")
			}
			inside = true
			continue
		}
		if inside {
			if line == ")" {
				inside, closed = false, true
				continue
			}
			if !required[line] {
				return fmt.Errorf("static array includes an unreviewed or nonliteral root %q", line)
			}
			counts[line]++
			if counts[line] != 1 {
				return fmt.Errorf("static root %s is duplicated", line)
			}
			continue
		}
		if line == `for contract in "${contracts[@]}"; do` {
			loops++
		}
	}
	if arrays != 1 || !closed || inside || loops != 1 {
		return errors.New("static analysis lacks one closed root array and executable census loop")
	}
	for root := range required {
		if counts[root] != 1 {
			return fmt.Errorf("static analysis omits deployed root %s", root)
		}
	}
	if !strings.Contains(script, "all ${#contracts[@]} deployable roots") {
		return errors.New("static analysis does not report its complete root count")
	}
	return nil
}

// The pre-fix four-root array and a comment-only journal name fail without
// invoking Solidity tools. Adjacent duplicate, missing and loop omissions fail.
func TestSolidityStaticGateRejectsOmittedOrCommentOnlyEvidenceRoot(t *testing.T) {
	t.Parallel()
	encoded, err := os.ReadFile("../scripts/test-solidity-static.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(encoded)
	if err := verifySolidityStaticRootCensus(script); err != nil {
		t.Fatal(err)
	}
	for _, root := range strings.Split(solidityStaticDeployedRoots, "\n") {
		line := "  " + root + "\n"
		for _, replacement := range []string{"", "  # " + root + "\n", line + line} {
			changed := strings.Replace(script, line, replacement, 1)
			if changed == script || verifySolidityStaticRootCensus(changed) == nil {
				t.Fatalf("missing, commented or duplicate root %s passed static coverage", root)
			}
		}
	}
	for _, changed := range []string{
		strings.Replace(script, "  src/STValidatorEvidence.sol\n", "  src/lib/ValidatorEvidence.sol\n", 1),
		strings.Replace(script, `for contract in "${contracts[@]}"; do`, `# for contract in "${contracts[@]}"; do`, 1),
		strings.Replace(script, "contracts=(", "# contracts=(", 1),
	} {
		if changed == script || verifySolidityStaticRootCensus(changed) == nil {
			t.Fatal("a library, comment or absent executable census authorized static coverage")
		}
	}
}
