package main

// Settlement v2 in the aggregate gate (VERSION2-FINAL.md §14 gates 1 and 3,
// §15): the scale shard, the Solidity coverage bar, the deep invariant
// campaign and the operator's v2 suites, each executed by an admitted phase.

import (
	"errors"
	"fmt"
	"go/build"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const releaseGateSolidityCoverageCommand = `FOUNDRY_OUT="$TMPDIR/coverage-out" FOUNDRY_CACHE_PATH="$TMPDIR/coverage-cache" forge coverage --ir-minimum --report lcov --skip STValidatorEvidence.sol --skip ValidatorEvidenceStorage.t.sol --skip ValidatorEvidenceActivation.t.sol --report-file "$TMPDIR/lcov.info" || coverage_status=$?`

const releaseGateV2InvariantCommand = "FOUNDRY_PROFILE=v2_invariant forge test --match-path test/V2Invariant.t.sol"

func readReleaseGateLocalScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../scripts/test-release-1.0-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The executable lines of one phase definition, without comments.
func releaseGatePhaseCommands(script, phase string) ([]string, []int, error) {
	pattern := regexp.MustCompile("(?ms)^[\\t ]*" + regexp.QuoteMeta("release_phase_"+phase) + "\\(\\) \\{\\n(.*?)^[\\t ]*\\}[\\t ]*$")
	definitions := pattern.FindAllStringSubmatchIndex(script, -1)
	if len(definitions) != 1 {
		return nil, nil, fmt.Errorf("gate has %d release_phase_%s definitions", len(definitions), phase)
	}
	var commands []string
	for _, line := range strings.Split(script[definitions[0][2]:definitions[0][3]], "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			commands = append(commands, line)
		}
	}
	return commands, definitions[0], nil
}

// One unconditional admission of the phase, after its definition.
func verifyReleaseGateAdmission(script, job, phase string, definition []int) error {
	start := "release_gate_start " + job + " release_phase_" + phase
	invocations := regexp.MustCompile("(?m)^" + regexp.QuoteMeta(start) + "[\\t ]*$")
	registry := regexp.MustCompile(`(?ms)^[\t ]*release_phase_[a-z0-9_]+\(\) \{\n.*?^[\t ]*\}[\t ]*$`).ReplaceAllString(script, "")
	calls := invocations.FindAllStringIndex(script, -1)
	if len(calls) != 1 || len(invocations.FindAllString(registry, -1)) != 1 || calls[0][0] < definition[1] {
		return fmt.Errorf("gate does not admit %s exactly once after its definition", job)
	}
	conditions, err := releaseGateRegistrationConditions(script, start)
	if err != nil || len(conditions) != 0 {
		return fmt.Errorf("gate admits %s conditionally: %v %v", job, conditions, err)
	}
	return nil
}

// A phase whose whole body is exactly these commands, admitted once.
func verifyReleaseGateExactPhase(script, job, phase string, want []string) error {
	commands, definition, err := releaseGatePhaseCommands(script, phase)
	if err != nil {
		return err
	}
	if !slices.Equal(commands, want) {
		return fmt.Errorf("release_phase_%s runs %q, want %q", phase, commands, want)
	}
	return verifyReleaseGateAdmission(script, job, phase, definition)
}

// releaseGateScalePackages lists, as ./dir, the packages under root with a Go
// file that only the scale build tag builds. Like go, it skips testdata,
// hidden and underscore directories and nested modules.
func releaseGateScalePackages(root string) ([]string, error) {
	scale := build.Default
	scale.BuildTags = append(slices.Clone(scale.BuildTags), "scale")
	packages := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if path == root {
				return nil
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		plain, err := build.Default.MatchFile(filepath.Dir(path), name)
		if err != nil {
			return err
		}
		tagged, err := scale.MatchFile(filepath.Dir(path), name)
		if err != nil {
			return err
		}
		if tagged && !plain {
			relative, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			packages["./"+filepath.ToSlash(relative)] = true
		}
		return nil
	})
	return slices.Sorted(maps.Keys(packages)), err
}

// GL-10 (§15 scale tests): one shard runs every scale-tagged package, verbose
// so that each root's time and memory are in its log, under one explicit
// deadline. No other gate command builds with a tag: the runtime rehearsal
// stays beside the gate.
func verifyReleaseGateScaleShard(script string, packages []string) error {
	if len(packages) == 0 {
		return errors.New("no package has a scale-tagged file")
	}
	command := "go test -tags scale -v -timeout 90m " + strings.Join(packages, " ") + " -count=1"
	if err := verifyReleaseGateExactPhase(script, "sn-scale", "sn_scale", []string{`cd "$sn_repo"`, command}); err != nil {
		return err
	}
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") && strings.Contains(line, "-tags") && line != command {
			return fmt.Errorf("gate builds with a tag outside the scale shard: %s", line)
		}
	}
	return nil
}

// Gate 1 (§15 coverage): the pinned command in a private build, then the bar,
// checked even when a test fails, whose status the phase still returns.
func verifyReleaseGateSolidityCoverage(script string) error {
	return verifyReleaseGateExactPhase(script, "solidity-coverage", "solidity_coverage", []string{
		`cd "$sn_repo/evm"`, "coverage_status=0", releaseGateSolidityCoverageCommand,
		`cd "$sn_repo"`, `go run ./scripts/solidity-coverage evm "$TMPDIR/lcov.info"`, `return "$coverage_status"`,
	})
}

// I-1 to I-6 (§15.5): the deep profile exists, the test file sizes its
// campaign, and the Solidity phase runs it last, after the plain suite and the
// release build's checks.
func verifyReleaseGateV2InvariantCampaign(script, foundry, invariant string) error {
	if !regexp.MustCompile(`(?m)^\[profile\.v2_invariant\.invariant\]$`).MatchString(foundry) {
		return errors.New("foundry.toml has no v2_invariant invariant profile")
	}
	for _, key := range []string{"runs", "depth"} {
		if !regexp.MustCompile(`(?m)^/// forge-config: v2_invariant\.invariant\.` + key + ` = [1-9][0-9]*$`).MatchString(invariant) {
			return fmt.Errorf("V2Invariant.t.sol sizes no deep campaign %s", key)
		}
	}
	commands, definition, err := releaseGatePhaseCommands(script, "solidity")
	if err != nil {
		return err
	}
	last := len(commands) - 1
	plain := slices.Index(commands, "forge test --summary")
	binding := slices.Index(commands, `"$sn_repo/stabi/generate.sh" --check --artifacts "$FOUNDRY_OUT"`)
	if last < 1 || commands[last] != releaseGateV2InvariantCommand || commands[last-1] != `cd "$sn_repo/evm"` || plain < 0 || binding < plain || binding >= last-1 {
		return errors.New("the Solidity phase does not end with the deep v2 invariant campaign")
	}
	count := 0
	for _, line := range strings.Split(script, "\n") {
		if line = strings.TrimSpace(line); !strings.HasPrefix(line, "#") && strings.Contains(line, "v2_invariant") {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("gate runs the v2_invariant profile %d times", count)
	}
	return verifyReleaseGateAdmission(script, "solidity", "solidity", definition)
}

// §15.7 (SV-1 to SV-8): the operator's v2 suites, normal and race, on the
// gate's private services; every root of a v2 test file is selected.
func releaseGateLedgerV2ServerGroup(t *testing.T) releaseEvidenceV2GateGroup {
	t.Helper()
	packages := ". ./controller ./model ./startifact ./taskworker ./taskworker/work"
	return releaseEvidenceV2GateGroup{
		phase: "server_db", job: "server-db", variable: "ledger_v2_tests",
		alternatives: "StLedgerV2|LedgerV2|LedgerObject|StConfigSelectsLedgerV2|StConfigRefusesMalformedLedgerV2|StSyncEventsMirrorsLedgerV2|StV1DriverStopsAtFirstV2Epoch|StSyncChainKeepsV1BelowFirstV2Epoch|WorkloadsRunLedgerV2",
		packages:     strings.Fields(packages),
		sources: map[string][]string{
			".":                 releaseEvidenceV2GateSources(t, []string{"../../server/st_ledger_v2_*_test.go"}),
			"./controller":      releaseEvidenceV2GateSources(t, []string{"../../server/controller/st_ledger_v2_*_test.go"}),
			"./model":           releaseEvidenceV2GateSources(t, []string{"../../server/model/st_ledger_v2_*_test.go"}),
			"./startifact":      releaseEvidenceV2GateSources(t, []string{"../../server/startifact/ledger_v2*_test.go"}),
			"./taskworker":      releaseEvidenceV2GateSources(t, []string{"../../server/taskworker/st_ledger_v2_*_test.go"}),
			"./taskworker/work": releaseEvidenceV2GateSources(t, []string{"../../server/taskworker/work/st_ledger_v2_*_test.go"}),
		},
		commands: []string{
			"go test " + packages + ` -run "$ledger_v2_tests" -count=1`,
			"go test -race " + packages + ` -run "$ledger_v2_tests" -count=1`,
		},
	}
}

func TestReleaseGateSettlementV2RunsEveryScalePackage(t *testing.T) {
	t.Parallel()
	packages, err := releaseGateScalePackages("..")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(packages, "./ledger") {
		t.Fatalf("scale census %q lacks GL-10's ./ledger", packages)
	}
	if err := verifyReleaseGateScaleShard(readReleaseGateLocalScript(t), packages); err != nil {
		t.Fatal(err)
	}
}

// The census reads build constraints as go does: a file every build includes
// or excludes is not scale-tagged, and testdata, hidden and underscore
// directories and nested modules are not packages of this module.
func TestReleaseGateSettlementV2ScaleCensusFollowsBuildConstraints(t *testing.T) {
	t.Parallel()
	foreign := "windows"
	if runtime.GOOS == foreign {
		foreign = "linux"
	}
	root := t.TempDir()
	for path, source := range map[string]string{
		"go.mod":                      "module example\n",
		"plain/plain_test.go":         "package plain\n",
		"tagged/tagged.go":            "package tagged\n",
		"tagged/scale_test.go":        "//go:build scale\n\npackage tagged\n",
		"either/either_test.go":       "//go:build scale || !scale\n\npackage either\n",
		"negated/negated_test.go":     "//go:build !scale\n\npackage negated\n",
		"rehearsal/node_test.go":      "//go:build runtime_rehearsal\n\npackage rehearsal\n",
		"host/deeper/scale_test.go":   "//go:build scale && " + runtime.GOOS + "\n\npackage deeper\n",
		"foreign/scale_test.go":       "//go:build scale && " + foreign + "\n\npackage foreign\n",
		"tagged/testdata/scale.go":    "//go:build scale\n\npackage testdata\n",
		"_ignored/scale_test.go":      "//go:build scale\n\npackage ignored\n",
		".hidden/scale_test.go":       "//go:build scale\n\npackage hidden\n",
		"module/go.mod":               "module other\n",
		"module/scale_test.go":        "//go:build scale\n\npackage module\n",
		"tagged/notes_scale_test.txt": "//go:build scale\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	packages, err := releaseGateScalePackages(root)
	if err != nil || !slices.Equal(packages, []string{"./host/deeper", "./tagged"}) {
		t.Fatalf("scale census %q: %v", packages, err)
	}
	if _, err := releaseGateScalePackages(filepath.Join(root, "absent")); err == nil {
		t.Fatal("an unreadable tree gave a census")
	}
}

// Each loss of the tag, the log, the deadline, an uncached run, a package or
// the one unconditional admission fails, as does a tagged command elsewhere.
func TestReleaseGateSettlementV2ScaleShardRejectsDrift(t *testing.T) {
	t.Parallel()
	script := readReleaseGateLocalScript(t)
	packages := []string{"./ledger"}
	if err := verifyReleaseGateScaleShard(script, packages); err != nil {
		t.Fatal(err)
	}
	const command = "go test -tags scale -v -timeout 90m ./ledger -count=1"
	const start = "release_gate_start sn-scale release_phase_sn_scale"
	for _, change := range []struct{ name, old, new string }{
		{"untagged", command, "go test -v -timeout 90m ./ledger -count=1"},
		{"another tag", command, strings.Replace(command, "scale", "runtime_rehearsal", 1)},
		{"unlogged measurements", command, strings.Replace(command, " -v", "", 1)},
		{"implicit deadline", command, strings.Replace(command, " -timeout 90m", "", 1)},
		{"cached run", command, strings.Replace(command, " -count=1", "", 1)},
		{"no test bodies", command, command + " -run '^$'"},
		{"hidden failure", command, command + " || true"},
		{"commented run", command, "# " + command},
		{"extra package", command, strings.Replace(command, "./ledger", "./ledger ./merkle", 1)},
		{"commented admission", start, "# " + start},
		{"duplicate admission", start, start + "\n" + start},
		{"conditional admission", start, "if false; then\n" + start + "\nfi"},
		{"nested admission", start, "release_phase_unused() {\n" + start + "\n}"},
		{"rehearsal in the gate", start, start + "\nrelease_phase_rehearsal() {\n  go test -tags runtime_rehearsal ./rehearsal/ -count=1\n}\nrelease_gate_start rehearsal release_phase_rehearsal"},
	} {
		if strings.Count(script, change.old) != 1 {
			t.Fatalf("%s: the mutation names no unique text", change.name)
		}
		if err := verifyReleaseGateScaleShard(strings.Replace(script, change.old, change.new, 1), packages); err == nil {
			t.Fatalf("scale shard accepted %s", change.name)
		}
	}
	early := strings.Replace(script, start, "# admitted before its definition", 1)
	early = strings.Replace(early, "release_phase_sn_scale() {", start+"\nrelease_phase_sn_scale() {", 1)
	if err := verifyReleaseGateScaleShard(early, packages); err == nil {
		t.Fatal("scale shard admitted an undefined phase")
	}
	for _, census := range [][]string{nil, {"./ledger", "./validator"}} {
		if err := verifyReleaseGateScaleShard(script, census); err == nil {
			t.Fatalf("scale shard accepted the census %q", census)
		}
	}
}

func TestReleaseGateSettlementV2ChecksTheSolidityCoverageBar(t *testing.T) {
	t.Parallel()
	if err := verifyReleaseGateSolidityCoverage(readReleaseGateLocalScript(t)); err != nil {
		t.Fatal(err)
	}
}

// Each change to the pinned command, its private build, the bar's input, the
// returned status or the admission fails.
func TestReleaseGateSettlementV2CoverageRejectsDrift(t *testing.T) {
	t.Parallel()
	script := readReleaseGateLocalScript(t)
	const check = `go run ./scripts/solidity-coverage evm "$TMPDIR/lcov.info"`
	const start = "release_gate_start solidity-coverage release_phase_solidity_coverage"
	command := releaseGateSolidityCoverageCommand
	for _, change := range []struct{ name, old, new string }{
		{"default mode", command, strings.Replace(command, " --ir-minimum", "", 1)},
		{"summary report", command, strings.Replace(command, "--report lcov", "--report summary", 1)},
		{"unskipped evidence library", command, strings.Replace(command, " --skip STValidatorEvidence.sol", "", 1)},
		{"unskipped storage test", command, strings.Replace(command, " --skip ValidatorEvidenceStorage.t.sol", "", 1)},
		{"unskipped activation test", command, strings.Replace(command, " --skip ValidatorEvidenceActivation.t.sol", "", 1)},
		{"release build directory", command, strings.Replace(command, `FOUNDRY_OUT="$TMPDIR/coverage-out" `, "", 1)},
		{"report in the checkout", command, strings.Replace(command, ` --report-file "$TMPDIR/lcov.info"`, "", 1)},
		{"masked test failure", command, strings.Replace(command, "|| coverage_status=$?", "|| true", 1)},
		{"unchecked bar", check, "# " + check},
		{"another report", check, strings.Replace(check, "$TMPDIR/lcov.info", "lcov.info", 1)},
		{"dropped status", `return "$coverage_status"`, "return 0"},
		{"commented admission", start, "# " + start},
		{"duplicate admission", start, start + "\n" + start},
		{"conditional admission", start, "if false; then\n" + start + "\nfi"},
	} {
		if strings.Count(script, change.old) != 1 {
			t.Fatalf("%s: the mutation names no unique text", change.name)
		}
		if err := verifyReleaseGateSolidityCoverage(strings.Replace(script, change.old, change.new, 1)); err == nil {
			t.Fatalf("coverage phase accepted %s", change.name)
		}
	}
}

func readReleaseGateInvariantSources(t *testing.T) (string, string) {
	t.Helper()
	foundry, err := os.ReadFile("../evm/foundry.toml")
	if err != nil {
		t.Fatal(err)
	}
	invariant, err := os.ReadFile("../evm/test/V2Invariant.t.sol")
	if err != nil {
		t.Fatal(err)
	}
	return string(foundry), string(invariant)
}

func TestReleaseGateSettlementV2RunsTheDeepInvariantCampaign(t *testing.T) {
	t.Parallel()
	foundry, invariant := readReleaseGateInvariantSources(t)
	if err := verifyReleaseGateV2InvariantCampaign(readReleaseGateLocalScript(t), foundry, invariant); err != nil {
		t.Fatal(err)
	}
}

// A missing profile or campaign size, a profile name Foundry's inline config
// ignores, a wider or masked run, a second or misplaced run, or a commented
// one fails.
func TestReleaseGateSettlementV2InvariantCampaignRejectsDrift(t *testing.T) {
	t.Parallel()
	script := readReleaseGateLocalScript(t)
	foundry, invariant := readReleaseGateInvariantSources(t)
	command := releaseGateV2InvariantCommand
	for _, change := range []struct{ name, old, new string }{
		{"hyphenated profile", command, strings.Replace(command, "v2_invariant", "v2-invariant", 1)},
		{"default profile", command, strings.Replace(command, "FOUNDRY_PROFILE=v2_invariant ", "", 1)},
		{"whole suite", command, strings.Replace(command, " --match-path test/V2Invariant.t.sol", "", 1)},
		{"masked failure", command, command + " || true"},
		{"commented campaign", command, "# " + command},
		{"second campaign", command, command + "\n  " + command},
	} {
		if strings.Count(script, change.old) != 1 {
			t.Fatalf("%s: the mutation names no unique text", change.name)
		}
		if err := verifyReleaseGateV2InvariantCampaign(strings.Replace(script, change.old, change.new, 1), foundry, invariant); err == nil {
			t.Fatalf("invariant campaign accepted %s", change.name)
		}
	}
	const campaign = "\n  cd \"$sn_repo/evm\"\n  " + releaseGateV2InvariantCommand + "\n"
	moved := strings.Replace(script, campaign, "\n", 1)
	moved = strings.Replace(moved, "  forge test --summary\n", "  "+releaseGateV2InvariantCommand+"\n  forge test --summary\n", 1)
	if strings.Count(script, campaign) != 1 || strings.Count(moved, releaseGateV2InvariantCommand) != 1 {
		t.Fatal("the moved campaign names no unique text")
	}
	if err := verifyReleaseGateV2InvariantCampaign(moved, foundry, invariant); err == nil {
		t.Fatal("invariant campaign accepted a run before the plain suite and the release build's checks")
	}
	for _, change := range []struct{ name, foundry, invariant string }{
		{"no profile", strings.Replace(foundry, "[profile.v2_invariant.invariant]", "[profile.default.invariant.v2]", 1), invariant},
		{"no deep runs", foundry, regexp.MustCompile(`(?m)^/// forge-config: v2_invariant\.invariant\.runs = .*\n`).ReplaceAllString(invariant, "")},
		{"no deep depth", foundry, regexp.MustCompile(`(?m)^/// forge-config: v2_invariant\.invariant\.depth = .*\n`).ReplaceAllString(invariant, "")},
	} {
		if change.foundry == foundry && change.invariant == invariant {
			t.Fatalf("%s: the mutation changes nothing", change.name)
		}
		if err := verifyReleaseGateV2InvariantCampaign(script, change.foundry, change.invariant); err == nil {
			t.Fatalf("invariant campaign accepted %s", change.name)
		}
	}
}

func TestReleaseGateSettlementV2RunsTheOperatorSuites(t *testing.T) {
	t.Parallel()
	if err := verifyReleaseEvidenceV2GateGroup(readReleaseGateLocalScript(t), releaseGateLedgerV2ServerGroup(t)); err != nil {
		t.Fatal(err)
	}
}

// Removing a family, a mode or the admitted phase fails, as does a v2 root
// that escapes the selector.
func TestReleaseGateSettlementV2OperatorSuitesRejectOmissions(t *testing.T) {
	t.Parallel()
	script := readReleaseGateLocalScript(t)
	group := releaseGateLedgerV2ServerGroup(t)
	selector, err := releaseConnectPolicySelectorAssignment(script, group.variable)
	if err != nil {
		t.Fatal(err)
	}
	for _, omitted := range strings.Split(group.alternatives, "|") {
		var retained []string
		for _, value := range strings.Split(group.alternatives, "|") {
			if value != omitted {
				retained = append(retained, value)
			}
		}
		changed := strings.Replace(script, group.variable+"='"+selector+"'", group.variable+"='^Test("+strings.Join(retained, "|")+")'", 1)
		if err := verifyReleaseEvidenceV2GateGroup(changed, group); err == nil {
			t.Fatalf("operator v2 suites accepted the removed family %s", omitted)
		}
	}
	for _, command := range append(slices.Clone(group.commands), "release_gate_start server-db release_phase_server_db") {
		if err := verifyReleaseEvidenceV2GateGroup(strings.Replace(script, command, "# omitted: "+command, 1), group); err == nil {
			t.Fatalf("operator v2 suites accepted the removed command %s", command)
		}
	}
	escaped := group
	escaped.sources = maps.Clone(group.sources)
	escaped.sources["./controller"] = append(slices.Clone(group.sources["./controller"]), "package controller\nfunc TestOmittedSettlementV2Root(t *testing.T) {}\n")
	if err := verifyReleaseEvidenceV2GateGroup(script, escaped); err == nil {
		t.Fatal("operator v2 suites accepted a root outside the selector")
	}
}
