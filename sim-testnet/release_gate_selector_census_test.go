package main

// Every -run selector of both release gates selects tests: each of its
// alternatives, nested ones included, matches a test that a package of its
// command declares, read from the package's test sources without a build, the
// way Go matches it, so ^Test(…)$ is an exact name and an alternative without
// $ a prefix. A renamed or never-declared test fails here instead of leaving
// its alternative selecting nothing, as the server-unit phase's
// StHeadBoundCkeysFromEvents$ did: no gate ran either HF-1 double-pay
// regression test.

import (
	"errors"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// The gates whose selectors the census reads, beside sim-testnet.
var releaseGateSelectorScripts = []string{"../scripts/test-release-1.0-local.sh", "../scripts/test-release-1.0-producer-gate.sh"}

// The -run values, as written, that the census leaves out, each with its
// reason; one held only by the command given must be that command.
var releaseGateUncensusedRuns = []struct{ run, command, reason string }{
	{run: `'^$'`, reason: "compile only: it builds the packages' tests and selects none"},
	{run: `"$selector"`, command: releaseGateShardCommand, reason: "computed at run time: each shard's CRC-32 share of its package's go test -list, which release_gate_shard_selector refuses when it is empty or skips a name that is no test"},
}

// The flags of the gates' -run commands that take a value, and those that
// take none; the census refuses any other.
var (
	releaseGateGoTestValueFlags = []string{"-run", "-skip", "-timeout", "-tags", "-p", "-parallel", "-count"}
	releaseGateGoTestBoolFlags  = []string{"-race", "-v"}
)

// One go test command of a gate that selects its tests with -run, as written:
// its line, the directory it runs in, beside sim-testnet, its packages, the
// build tags of its mode and -tags, and its -run value.
type releaseGateRunCommand struct {
	line                 int
	text, directory, run string
	packages, tags       []string
}

// The names a package in the directory, beside sim-testnet, declares for -run
// when built with the tags.
type releaseGateTestCensus func(directory string, tags []string) ([]string, error)

// The census of the checked-out sources, which every test shares: it reads
// each package once per set of tags.
var releaseGateCheckoutTestCensus = releaseGateSourceTestCensus()

func releaseGateSourceTestCensus() releaseGateTestCensus {
	var lock sync.Mutex
	read := map[string][]string{}
	return func(directory string, tags []string) ([]string, error) {
		lock.Lock()
		defer lock.Unlock()
		key := directory + " " + strings.Join(tags, ",")
		if names, ok := read[key]; ok {
			return names, nil
		}
		context := build.Default
		context.BuildTags = append(slices.Clone(context.BuildTags), tags...)
		names, err := releaseGateDeclaredTests(directory, context)
		if err != nil {
			return nil, err
		}
		read[key] = names
		return names, nil
	}
}

// The directory, beside sim-testnet, that a gate's cd argument enters: sn's
// checkout for $sn_repo and the one that holds its siblings for $workspace.
// Any other argument leaves it unknown.
func releaseGateCdDirectory(argument string) string {
	target, quoted := strings.CutPrefix(argument, `"`)
	target, closed := strings.CutSuffix(target, `"`)
	for variable, directory := range map[string]string{"$sn_repo": "..", "$workspace": "../.."} {
		if rest, ok := strings.CutPrefix(target, variable); quoted && closed && ok && (rest == "" || strings.HasPrefix(rest, "/")) && !strings.ContainsAny(rest, "$\"' ") {
			return filepath.Join(directory, rest)
		}
	}
	return ""
}

// One go test line's packages, build tags and -run value as written, up to
// its first control operator.
func releaseGateParseRunCommand(line string) (releaseGateRunCommand, error) {
	command := releaseGateRunCommand{text: line}
	fields := strings.Fields(line)
	for index, field := range fields {
		if field == "||" || field == "&&" || field == "|" || field == ";" {
			fields = fields[:index]
			break
		}
	}
	if len(fields) < 2 || fields[0] != "go" || fields[1] != "test" || strings.HasSuffix(line, `\`) {
		return command, errors.New("the census cannot read this -run command")
	}
	for index := 2; index < len(fields); index++ {
		name, value, inline := strings.Cut(fields[index], "=")
		switch {
		case !strings.HasPrefix(name, "-"):
			command.packages = append(command.packages, fields[index])
		case slices.Contains(releaseGateGoTestBoolFlags, name) && !inline:
			if name == "-race" {
				command.tags = append(command.tags, "race")
			}
		case slices.Contains(releaseGateGoTestValueFlags, name):
			if !inline {
				if index++; index == len(fields) {
					return command, fmt.Errorf("its %s has no value", name)
				}
				value = fields[index]
			}
			if name == "-run" && command.run != "" {
				return command, errors.New("it repeats -run")
			}
			if name == "-run" {
				command.run = value
			} else if name == "-tags" {
				command.tags = append(command.tags, strings.Split(value, ",")...)
			}
		default:
			return command, fmt.Errorf("it has the unreviewed go test flag %s", fields[index])
		}
	}
	if command.run == "" {
		return command, errors.New("it has no -run value")
	}
	slices.Sort(command.tags)
	return command, nil
}

// The go test commands of a gate that select their tests with -run, each with
// the directory that the last cd of its own function, group or subshell
// enters; a function's is its caller's, so it starts unknown. A line the
// census cannot read is refused rather than left out.
func releaseGateRunCommands(script string) ([]releaseGateRunCommand, error) {
	function := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\(\) \{$`)
	directories := []string{""}
	var commands []releaseGateRunCommand
	for index, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
			continue
		case function.MatchString(line):
			directories = append(directories, "")
			continue
		case line == "(" || line == "{":
			directories = append(directories, directories[len(directories)-1])
			continue
		case strings.HasPrefix(line, ")") || strings.HasPrefix(line, "}"):
			if len(directories) == 1 {
				return nil, fmt.Errorf("line %d closes no function, group or subshell", index+1)
			}
			directories = directories[:len(directories)-1]
			continue
		case strings.HasPrefix(line, "cd "):
			directories[len(directories)-1] = releaseGateCdDirectory(strings.TrimPrefix(line, "cd "))
			continue
		case !strings.Contains(line, "go test") || !strings.Contains(line, "-run"):
			continue
		}
		command, err := releaseGateParseRunCommand(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", index+1, line, err)
		}
		command.line, command.directory = index+1, directories[len(directories)-1]
		commands = append(commands, command)
	}
	if len(directories) != 1 {
		return nil, errors.New("the gate leaves a function, group or subshell open")
	}
	return commands, nil
}

// The values a command's -run takes in the gate's two dependency modes: its
// quoted literal, or its variable's one assignment or reviewed mode pair.
func releaseGateRunSelectorValues(script, run string) ([]string, error) {
	if literal, ok := strings.CutPrefix(run, "'"); ok {
		if value, ok := strings.CutSuffix(literal, "'"); ok && !strings.Contains(value, "'") {
			return []string{value}, nil
		}
	}
	variable, quoted := strings.CutPrefix(run, `"$`)
	variable, closed := strings.CutSuffix(variable, `"`)
	if !quoted || !closed || !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(variable) {
		return nil, fmt.Errorf("the census cannot read -run %s", run)
	}
	var values []string
	for _, mode := range []string{"pinned", "local"} {
		value, err := releaseGateModeSelector(script, variable, mode)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(values, value) {
			values = append(values, value)
		}
	}
	return values, nil
}

// The leaves of a -run selector: one regular expression for each way of
// taking one branch of every alternation in it, nested ones included, so each
// alternative as written selects tests on its own. A group with flags or a
// quantifier, whose branches cannot stand alone, is refused.
func releaseGateSelectorLeaves(selector string) ([]string, error) {
	leaves, rest, err := releaseGateSelectorAlternation(selector)
	if err == nil && rest != "" {
		err = errors.New("an unmatched )")
	}
	if err != nil {
		return nil, fmt.Errorf("selector %q has %w", selector, err)
	}
	return leaves, nil
}

// The leaves of the branches from the start of text to its first unmatched )
// or its end, and the text from there.
func releaseGateSelectorAlternation(text string) ([]string, string, error) {
	var leaves []string
	for {
		branch, rest, err := releaseGateSelectorBranch(text)
		if err != nil {
			return nil, "", err
		}
		leaves = append(leaves, branch...)
		if !strings.HasPrefix(rest, "|") {
			return leaves, rest, nil
		}
		text = rest[1:]
	}
}

// The leaves of one branch, up to its | or ), and the text from there.
func releaseGateSelectorBranch(text string) ([]string, string, error) {
	leaves := []string{""}
	for text != "" && text[0] != '|' && text[0] != ')' {
		atoms, rest, err := releaseGateSelectorAtom(text)
		if err != nil {
			return nil, "", err
		}
		var joined []string
		for _, leaf := range leaves {
			for _, atom := range atoms {
				joined = append(joined, leaf+atom)
			}
		}
		leaves, text = joined, rest
	}
	return leaves, text, nil
}

// The leaves of the atom that starts text, an escape, a class, a group's
// branches or one byte, and the text after it.
func releaseGateSelectorAtom(text string) ([]string, string, error) {
	switch text[0] {
	case '\\':
		if len(text) < 2 {
			return nil, "", errors.New("a trailing \\")
		}
		return []string{text[:2]}, text[2:], nil
	case '[':
		end := 1
		if strings.HasPrefix(text[end:], "^") {
			end++
		}
		if strings.HasPrefix(text[end:], "]") {
			end++
		}
		for end < len(text) && text[end] != ']' {
			if text[end] == '\\' {
				end++
			} else if strings.HasPrefix(text[end:], "[:") {
				if named := strings.Index(text[end:], ":]"); named > 0 {
					end += named + 1
				}
			}
			end++
		}
		if end >= len(text) {
			return nil, "", errors.New("an unmatched [")
		}
		return []string{text[:end+1]}, text[end+1:], nil
	case '(':
		inner := strings.TrimPrefix(text[1:], "?:")
		if strings.HasPrefix(inner, "?") {
			return nil, "", errors.New("a group with flags")
		}
		branches, rest, err := releaseGateSelectorAlternation(inner)
		if err != nil {
			return nil, "", err
		}
		if !strings.HasPrefix(rest, ")") {
			return nil, "", errors.New("an unmatched (")
		}
		if rest = rest[1:]; rest != "" && strings.ContainsRune("*+?{", rune(rest[0])) {
			return nil, "", errors.New("a quantified group")
		}
		return branches, rest, nil
	}
	return []string{text[:1]}, text[1:], nil
}

// The tests that the packages of a command declare, built with its tags.
func releaseGateRunCommandTests(command releaseGateRunCommand, census releaseGateTestCensus) ([]string, []string, error) {
	if command.directory == "" {
		return nil, nil, errors.New("no cd of its own function or subshell enters a checkout it names")
	}
	if len(command.packages) == 0 {
		return nil, nil, errors.New("it names no package")
	}
	var names, directories []string
	for _, packagePath := range command.packages {
		if packagePath != "." && (!strings.HasPrefix(packagePath, "./") || strings.Contains(packagePath, "...") || strings.ContainsAny(packagePath, "$\"'")) {
			return nil, nil, fmt.Errorf("its package %s is not one directory of its module", packagePath)
		}
		directory := filepath.Join(command.directory, packagePath)
		declared, err := census(directory, command.tags)
		if err != nil {
			return nil, nil, err
		}
		names, directories = append(names, declared...), append(directories, directory)
	}
	return names, directories, nil
}

// One -run value's census: each leaf of each value it takes selects a test
// that a package of one of its commands declares. A selector that several
// commands share, each on its own packages, as evidence_source_tests is
// across the server's, selects from all of them.
func verifyReleaseGateRunSelector(script, run string, commands []releaseGateRunCommand, census releaseGateTestCensus) error {
	var names, directories, lines []string
	for _, command := range commands {
		declared, packages, err := releaseGateRunCommandTests(command, census)
		if err != nil {
			return fmt.Errorf("line %d: %s: %w", command.line, command.text, err)
		}
		names, lines = append(names, declared...), append(lines, strconv.Itoa(command.line))
		for _, directory := range packages {
			if !slices.Contains(directories, directory) {
				directories = append(directories, directory)
			}
		}
	}
	at := "line " + lines[0]
	if len(lines) > 1 {
		at = "lines " + strings.Join(lines, ", ")
	}
	values, err := releaseGateRunSelectorValues(script, run)
	if err != nil {
		return fmt.Errorf("%s: %w", at, err)
	}
	var stale []error
	for _, value := range values {
		leaves, err := releaseGateSelectorLeaves(value)
		if err != nil {
			return fmt.Errorf("%s: %w", at, err)
		}
		for _, leaf := range leaves {
			compiled, err := regexp.Compile(leaf)
			if err != nil {
				return fmt.Errorf("%s: %w", at, err)
			}
			if !slices.ContainsFunc(names, compiled.MatchString) {
				stale = append(stale, fmt.Errorf("%s: -run %s: its alternative %s selects no test of %s", at, run, leaf, strings.Join(directories, " ")))
			}
		}
	}
	return errors.Join(stale...)
}

// The census of a gate's -run commands: the number it read, the number it
// left out by their -run value, and an error for each alternative that
// selects no test and each command it cannot read.
func verifyReleaseGateRunSelectors(script string, census releaseGateTestCensus) (int, map[string]int, error) {
	commands, err := releaseGateRunCommands(script)
	if err != nil {
		return 0, nil, err
	}
	var runs []string
	selecting, left := map[string][]releaseGateRunCommand{}, map[string]int{}
	var failures []error
commands:
	for _, command := range commands {
		for _, uncensused := range releaseGateUncensusedRuns {
			if command.run != uncensused.run {
				continue
			}
			if uncensused.command != "" && command.text != uncensused.command {
				failures = append(failures, fmt.Errorf("line %d: %s: only %s has -run %s", command.line, command.text, uncensused.command, command.run))
			}
			left[command.run]++
			continue commands
		}
		if selecting[command.run] == nil {
			runs = append(runs, command.run)
		}
		selecting[command.run] = append(selecting[command.run], command)
	}
	read := 0
	for _, run := range runs {
		read += len(selecting[run])
		if err := verifyReleaseGateRunSelector(script, run, selecting[run], census); err != nil {
			failures = append(failures, err)
		}
	}
	return read, left, errors.Join(failures...)
}

// HF-1's drift cannot recur: at the checked-out sn and siblings, every
// alternative of every -run selector of both gates, written in its command or
// in its variable, selects a test that its command's packages declare. The
// census reads every go test line with -run, and leaves out only the reviewed
// selectors that no script states, each used and for its reason.
func TestReleaseGateSelectorsSelectDeclaredTests(t *testing.T) {
	t.Parallel()
	census := releaseGateCheckoutTestCensus
	left := map[string]int{}
	for _, path := range releaseGateSelectorScripts {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		lines := 0
		for _, line := range strings.Split(string(raw), "\n") {
			if fields := strings.Fields(line); len(fields) > 2 && fields[0] == "go" && fields[1] == "test" && slices.ContainsFunc(fields, func(field string) bool { return field == "-run" || strings.HasPrefix(field, "-run=") }) {
				lines++
			}
		}
		read, uncensused, err := verifyReleaseGateRunSelectors(string(raw), census)
		if err != nil {
			t.Errorf("%s:\n%v", path, err)
		}
		count := read
		for run, commands := range uncensused {
			left[run] += commands
			count += commands
		}
		if read == 0 || count != lines {
			t.Errorf("%s: the census read %d and left out %d of %d go test commands with -run", path, read, count-read, lines)
		}
		t.Logf("%s: %d -run commands read", path, read)
	}
	for _, uncensused := range releaseGateUncensusedRuns {
		if left[uncensused.run] == 0 {
			t.Errorf("no gate command has the uncensused -run %s", uncensused.run)
		}
		t.Logf("%d commands with -run %s left out: %s", left[uncensused.run], uncensused.run, uncensused.reason)
	}
}

// The original HF-1 selector fails the census, as do a renamed regression of
// the group that replaced it and a renamed alternative of a selector
// variable in the producer gate.
func TestReleaseGateSelectorCensusRefusesTheHeadBoundDrift(t *testing.T) {
	t.Parallel()
	census := releaseGateCheckoutTestCensus
	local := readReleaseGateLocalScript(t)
	producer, err := os.ReadFile("../scripts/test-release-1.0-producer-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ script, old, new, stale string }{
		{local, "StHeadBoundCkeysFromEvents(WindowOverlap|UnbindDodge)", "StHeadBoundCkeysFromEvents", "^TestStHeadBoundCkeysFromEvents$ selects no test of ../../server/model"},
		{local, "StHeadBoundCkeysFromEvents(WindowOverlap|UnbindDodge)", "StHeadBoundCkeysFromEvents(WindowOverlap|UnbindDodgeRenamed)", "^TestStHeadBoundCkeysFromEventsUnbindDodgeRenamed$ selects no test of ../../server/model"},
		{string(producer), "|ProductionLoader)'", "|ProductionLoaderRenamed)'", "^TestProductionLoaderRenamed selects no test of ../validator"},
	} {
		if strings.Count(change.script, change.old) != 1 {
			t.Fatalf("the mutation names no unique text: %q", change.old)
		}
		if _, _, err := verifyReleaseGateRunSelectors(change.script, census); err != nil {
			t.Fatalf("the unchanged gate fails the census: %v", err)
		}
		_, _, err := verifyReleaseGateRunSelectors(strings.Replace(change.script, change.old, change.new, 1), census)
		if err == nil || !strings.Contains(err.Error(), "its alternative "+change.stale) {
			t.Fatalf("the census accepted %s: %v", change.new, err)
		}
	}
}

// A census of synthetic declarations, by directory, beside sim-testnet, and
// build tags, each written after its directory.
func releaseGateFixtureTestCensus(declared map[string][]string) releaseGateTestCensus {
	return func(directory string, tags []string) ([]string, error) {
		names, ok := declared[strings.Join(append([]string{directory}, tags...), " ")]
		if !ok {
			return nil, fmt.Errorf("test census: 0 files in %s %q", directory, tags)
		}
		return names, nil
	}
}

// A synthetic gate whose one phase enters the server's checkout.
func releaseGateCensusFixtureScript(lines ...string) string {
	return "release_phase_fixture() {\n  cd \"$workspace/server\"\n  " + strings.Join(lines, "\n  ") + "\n}\nrelease_gate_start fixture release_phase_fixture\n"
}

// ^Test(…)$ selects only a test of that exact name, an alternative without $
// any test it begins, and an unanchored one any test containing it.
func TestReleaseGateSelectorCensusHonoursAnchoring(t *testing.T) {
	t.Parallel()
	census := releaseGateFixtureTestCensus(map[string][]string{"../../server/model": {"TestFooBar", "TestQux"}})
	for _, testCase := range []struct{ run, stale string }{
		{run: `'^Test(FooBar|Qux)$'`},
		{run: `'^Test(Foo|Qux)'`},
		{run: `'Test.*(Bar|Qux)'`},
		{run: `'^Test(Foo|Qux)$'`, stale: "^TestFoo$"},
		{run: `'^Test(FooBar|Quux)'`, stale: "^TestQuux"},
		{run: `'^TestFoo(Bar|Baz)$'`, stale: "^TestFooBaz$"},
		{run: `'Test.*(Bar|Baz)'`, stale: "Test.*Baz"},
	} {
		_, _, err := verifyReleaseGateRunSelectors(releaseGateCensusFixtureScript("go test ./model -run "+testCase.run+" -count=1"), census)
		if testCase.stale == "" && err != nil || testCase.stale != "" && (err == nil || !strings.Contains(err.Error(), "its alternative "+testCase.stale+" selects no test")) {
			t.Fatalf("-run %s: %v, want stale %q", testCase.run, err, testCase.stale)
		}
	}
}

// Each alternative as written stands alone, nested ones included, and a
// selector whose alternatives cannot stand alone is refused.
func TestReleaseGateSelectorCensusReadsEveryAlternative(t *testing.T) {
	t.Parallel()
	for selector, want := range map[string][]string{
		"^Test(A(B|C)|D)$":            {"^TestAB$", "^TestAC$", "^TestD$"},
		"^TestX(Y|Z)":                 {"^TestXY", "^TestXZ"},
		"^(TestA|TestB)$|^(TestC)$":   {"^TestA$", "^TestB$", "^TestC$"},
		`^Test(V[12].*|H[13]X|Y\(Z)$`: {"^TestV[12].*$", "^TestH[13]X$", `^TestY\(Z$`},
		"^Test(?:A|B)[]|(:alpha:]$":   {"^TestA[]|(:alpha:]$", "^TestB[]|(:alpha:]$"},
		"^Test([[:alpha:]|]|C)":       {"^Test[[:alpha:]|]", "^TestC"},
		"TestTrie":                    {"TestTrie"},
	} {
		leaves, err := releaseGateSelectorLeaves(selector)
		if err != nil || !slices.Equal(leaves, want) {
			t.Fatalf("the leaves of %q are %q, want %q: %v", selector, leaves, want, err)
		}
	}
	for _, selector := range []string{"^Test(A|B)*", "^Test(A|B)?", "^Test(A|B)+", "^Test(A|B){2}", "^Test(A|B", "^Test(A|B))", "^Test(?i:A|B)", "^Test[AB", `^Test\`} {
		if leaves, err := releaseGateSelectorLeaves(selector); err == nil {
			t.Fatalf("the census read %q as %q", selector, leaves)
		}
	}
}

// Each command reads its own phase's checkout and the union of its packages,
// built with the tags of its mode and -tags, and only the reviewed runtime
// selectors are left out; a command the census cannot place or read is
// refused rather than passed.
func TestReleaseGateSelectorCensusResolvesEachCommand(t *testing.T) {
	t.Parallel()
	census := releaseGateFixtureTestCensus(map[string][]string{
		"../ledger":                  {"TestLedgerRoot"},
		"../../server/model":         {"TestModelRoot"},
		"../../server/model race":    {"TestModelRoot", "TestModelRaceRoot"},
		"../../server/model scale":   {"TestModelRoot", "TestStV2ScaleRoot"},
		"../../server/controller":    {"TestControllerRoot"},
		"../../sdk/build/cmd/export": {"TestExportRoot"},
	})
	other := "release_phase_other() {\n  cd \"$workspace/sdk/build\"\n  go test ./cmd/export -run '^TestExportRoot$' -count=1\n}\n"
	read, left, err := verifyReleaseGateRunSelectors(releaseGateCensusFixtureScript(
		`go test ./model ./controller -run '^Test(ModelRoot|ControllerRoot)$' -count=1`,
		`shared_tests='^Test(ModelRoot|ControllerRoot)$'`,
		`go test ./model -run "$shared_tests" -count=1`,
		`go test ./controller -run "$shared_tests" -count=1`,
		`fixture_tests='^Test(ModelRaceRoot)$'`,
		`go test -race ./model -run "$fixture_tests" -count=1 -parallel=4 -timeout 10m`,
		`go test -tags scale ./model -run '^TestStV2Scale' -count=1 -p 1 -v`,
		"(",
		`  cd "$sn_repo"`,
		`  go test ./ledger -run='^TestLedgerRoot$' -count=1 || status=1`,
		") || status=1",
		`go test ./model -run '^TestModelRoot$' -count=1`,
		`go test ./... -run '^$'`,
	)+other, census)
	if err != nil || read != 8 || left[`'^$'`] != 1 || len(left) != 1 {
		t.Fatalf("the census read %d and left out %v: %v", read, left, err)
	}
	for _, testCase := range []struct{ script, refusal string }{
		{releaseGateCensusFixtureScript(`go test ./model -run '^Test(ModelRoot|ControllerRoot)$' -count=1`), "its alternative ^TestControllerRoot$ selects no test of ../../server/model"},
		{releaseGateCensusFixtureScript(`shared_tests='^Test(ModelRoot|ControllerRoot|LedgerRoot)$'`, `go test ./model -run "$shared_tests" -count=1`, `go test ./controller -run "$shared_tests" -count=1`), "its alternative ^TestLedgerRoot$ selects no test of ../../server/model ../../server/controller"},
		{releaseGateCensusFixtureScript(`go test ./model -run '^TestModelRaceRoot$' -count=1`), "its alternative ^TestModelRaceRoot$ selects no test"},
		{releaseGateCensusFixtureScript(`go test ./model -run '^TestStV2Scale' -count=1`), "its alternative ^TestStV2Scale selects no test"},
		{releaseGateCensusFixtureScript("(", `  cd "$sn_repo"`, ")", `go test ./ledger -run '^TestLedgerRoot$' -count=1`), "0 files in ../../server/ledger"},
		{"release_phase_fixture() {\n  go test ./model -run '^TestModelRoot$' -count=1\n}\n", "no cd of its own function or subshell enters a checkout"},
		{other + "release_phase_fixture() {\n  go test ./cmd/export -run '^TestExportRoot$' -count=1\n}\n", "no cd of its own function or subshell enters a checkout"},
		{"release_phase_fixture() {\n  cd \"$HOME/server\"\n  go test ./model -run '^TestModelRoot$' -count=1\n}\n", "no cd of its own function or subshell enters a checkout"},
		{releaseGateCensusFixtureScript(`go test ./... -run '^TestModelRoot$' -count=1`), "its package ./... is not one directory of its module"},
		{releaseGateCensusFixtureScript(`go test "$package" -run '^TestModelRoot$' -count=1`), `its package "$package" is not one directory of its module`},
		{releaseGateCensusFixtureScript(`go test ./missing -run '^TestModelRoot$' -count=1`), "0 files in ../../server/missing"},
		{releaseGateCensusFixtureScript(`go test -run '^TestModelRoot$' -count=1`), "it names no package"},
		{releaseGateCensusFixtureScript(`go test -cover ./model -run '^TestModelRoot$' -count=1`), "the unreviewed go test flag -cover"},
		{releaseGateCensusFixtureScript(`go test ./model -run '^TestModelRoot$' -count=1 \`, "  -timeout 10m"), "the census cannot read this -run command"},
		{releaseGateCensusFixtureScript(`env GOFLAGS=-v go test ./model -run '^TestModelRoot$' -count=1`), "the census cannot read this -run command"},
		{releaseGateCensusFixtureScript(`go test ./model -run '^TestModelRoot$' -run '^TestModelRoot$' -count=1`), "it repeats -run"},
		{releaseGateCensusFixtureScript(`go test ./model -count=1 -run`), "its -run has no value"},
		{releaseGateCensusFixtureScript(`go test ./model -runx '^TestModelRoot$' -count=1`), "the unreviewed go test flag -runx"},
		{releaseGateCensusFixtureScript(`go test ./model -run TestModelRoot -count=1`), "the census cannot read -run TestModelRoot"},
		{releaseGateCensusFixtureScript(`go test ./model -run "${fixture_tests}" -count=1`), `the census cannot read -run "${fixture_tests}"`},
		{releaseGateCensusFixtureScript(`go test ./model -run "$missing_tests" -count=1`), "missing_tests has 0 assignments"},
		{releaseGateCensusFixtureScript(`go test ./model -run '^Test(ModelRoot|Other)*$' -count=1`), "a quantified group"},
		{releaseGateCensusFixtureScript(`go test ./model -run "$selector" -count=1`), "only " + releaseGateShardCommand + ` has -run "$selector"`},
		{releaseGateCensusFixtureScript("}", `go test ./model -run '^TestModelRoot$' -count=1`), "closes no function, group or subshell"},
		{"release_phase_fixture() {\n  cd \"$workspace/server\"\n", "leaves a function, group or subshell open"},
	} {
		if _, _, err := verifyReleaseGateRunSelectors(testCase.script, census); err == nil || !strings.Contains(err.Error(), testCase.refusal) {
			t.Fatalf("the census accepted %q: %v, want %q", testCase.script, err, testCase.refusal)
		}
	}
}
