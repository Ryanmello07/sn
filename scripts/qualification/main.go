// Build-once, dependency-driven offline qualification with compact status.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// The already-qualified Linux owner remains a temporary migration dependency.
// Only the direct Bash child is signaled; its pidfd/subreaper owner joins all
// descendants before its completion/ACK/wait protocol releases the stage.
const ownerScript = `set -euo pipefail
sn_repo=$1
workspace=$2
export QUALIFICATION_RUNNER=$3 QUALIFICATION_REQUEST=$4
source "$5"
release_gate_jobs_init
qualification_phase() {
  "$QUALIFICATION_RUNNER" worker "$QUALIFICATION_REQUEST"
}
release_gate_start qualification qualification_phase
qualification_status=0
release_gate_complete || qualification_status=$?
if (( release_gate_active == 0 )) && [[ "$release_gate_services_cleaned" == 1 ]]; then
  (set -o noclobber; printf 'joined\n' > "$6") || exit 125
else
  exit 125
fi
exit "$qualification_status"
`

type commandRequest struct {
	Argv        []string          `json:"argv"`
	Directory   string            `json:"directory"`
	Seconds     int               `json:"seconds"`
	Environment map[string]string `json:"environment"`
	Stdin       string            `json:"stdin"`
	Stdout      string            `json:"stdout"`
	Stderr      string            `json:"stderr"`
	Result      string            `json:"result"`
}
type commandResult struct {
	Exit      *int    `json:"exit"`
	Joined    bool    `json:"joined"`
	OwnerExit int     `json:"owner_exit"`
	Seconds   float64 `json:"seconds"`
	Stdout    string  `json:"stdout"`
	Stderr    string  `json:"stderr"`
	Request   string  `json:"request"`
	OwnerLog  string  `json:"owner_log"`
	Error     string  `json:"error,omitempty"`
}
type stageResult struct {
	Status       string         `json:"status"`
	Error        string         `json:"error,omitempty"`
	Command      *commandResult `json:"command,omitempty"`
	BinarySHA256 string         `json:"binary_sha256,omitempty"`
	Seconds      float64        `json:"seconds,omitempty"`
	Verification *eventSummary  `json:"verification,omitempty"`
	Events       string         `json:"events,omitempty"`
}
type stage struct {
	Dependencies []string
	Build        *suiteSpec
	Suite        *suiteSpec
	Binary       string
	BinarySHA256 string
}
type matrixStatus struct {
	State           string   `json:"state"`
	Finished        int      `json:"finished"`
	Total           int      `json:"total"`
	Running         []string `json:"running,omitempty"`
	Pending         int      `json:"pending,omitempty"`
	Failed          []string `json:"failed,omitempty"`
	SourceUnchanged bool     `json:"source_unchanged"`
	Error           string   `json:"error,omitempty"`
	Report          string   `json:"report"`
}
type matrixReport struct {
	Status  matrixStatus           `json:"status"`
	Results map[string]stageResult `json:"results"`
}

// A scheduler failure still cancels and joins every admitted worker before
// returning. No global lock is held across an external command or callback.
func runDAG(ctx context.Context, nodes map[string]stage, jobs int, execute func(context.Context, string, stage) stageResult, publish func(map[string]stageResult, []string, int) error) (map[string]stageResult, error) {
	if jobs < 1 || jobs > 256 || len(nodes) > 2048 || execute == nil || publish == nil {
		return nil, errors.New("bounded jobs, nodes and callbacks required")
	}
	// Validate the entire graph before any independent stage can mutate capture.
	visited := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] == 1 {
			return errors.New("cyclic dependency")
		}
		if visited[name] == 2 {
			return nil
		}
		node, ok := nodes[name]
		if !ok {
			return errors.New("missing dependency")
		}
		visited[name] = 1
		for _, dependency := range node.Dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visited[name] = 2
		return nil
	}
	for name := range nodes {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type completion struct {
		Name   string
		Result stageResult
		Err    error
	}
	complete := make(chan completion, jobs)
	pending := map[string]stage{}
	for name, node := range nodes {
		pending[name] = node
	}
	results := map[string]stageResult{}
	running := map[string]bool{}
	var scheduleErr error
	for len(pending)+len(running) > 0 {
		for _, name := range sortedKeys(pending) {
			node := pending[name]
			blocked, ready := false, true
			for _, dependency := range node.Dependencies {
				result, done := results[dependency]
				ready = ready && done
				blocked = blocked || (done && result.Status != "passed")
			}
			if ctx.Err() != nil || blocked {
				status := "blocked"
				if ctx.Err() != nil {
					status = "canceled"
				}
				results[name] = stageResult{Status: status}
				delete(pending, name)
			} else if ready && len(running) < jobs {
				running[name] = true
				delete(pending, name)
				if node.Binary != "" {
					node.BinarySHA256 = results[node.Binary].BinarySHA256
				}
				go func() {
					finished := completion{Name: name}
					returned := false
					defer func() {
						recovered := recover()
						if !returned {
							finished.Err = errors.New("stage exited without returning")
							if recovered != nil {
								finished.Err = errors.New("stage panicked")
							}
							finished.Result = stageResult{Status: "failed", Error: finished.Err.Error()}
						}
						complete <- finished
					}()
					finished.Result = execute(ctx, name, node)
					returned = true
					if finished.Result.Status != "passed" && finished.Result.Status != "failed" && finished.Result.Status != "canceled" {
						finished.Result = stageResult{Status: "failed", Error: "stage returned an invalid terminal status"}
					}
				}()
			}
		}
		if scheduleErr == nil {
			if err := publishSnapshot(publish, results, sortedKeys(running), len(pending)); err != nil {
				scheduleErr = err
				cancel()
			}
		}
		if len(running) == 0 {
			if len(pending) != 0 {
				scheduleErr = errors.Join(scheduleErr, errors.New("cycle or missing dependency"))
			}
			break
		}
		var finished completion
		if ctx.Err() != nil {
			finished = <-complete
		} else {
			select {
			case finished = <-complete:
			case <-ctx.Done():
				continue
			}
		}
		delete(running, finished.Name)
		results[finished.Name] = finished.Result
		if finished.Err != nil {
			scheduleErr = errors.Join(scheduleErr, finished.Err)
			cancel()
		}
	}
	return results, scheduleErr
}

// One joined publisher invocation also covers runtime.Goexit, whose defers run
// without a panic. The callback receives no mutable scheduler-owned storage.
func publishSnapshot(publish func(map[string]stageResult, []string, int) error, results map[string]stageResult, running []string, pending int) error {
	snapshot := map[string]stageResult{}
	for name, result := range results {
		if result.Command != nil {
			command := *result.Command
			if command.Exit != nil {
				exit := *command.Exit
				command.Exit = &exit
			}
			result.Command = &command
		}
		if result.Verification != nil {
			verification := *result.Verification
			result.Verification = &verification
		}
		snapshot[name] = result
	}
	finished := make(chan error, 1)
	go func() {
		var err error
		returned := false
		defer func() {
			recovered := recover()
			if !returned {
				err = errors.New("publisher exited without returning")
				if recovered != nil {
					err = errors.New("publisher panicked")
				}
			}
			finished <- err
		}()
		err = publish(snapshot, append([]string(nil), running...), pending)
		returned = true
	}()
	return <-finished
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		return exited.ExitCode()
	}
	return 125
}

// Child execution is deliberately outside CommandContext: the timeout's result
// is recorded independently and the enclosing owner proves descendant joins.
func runWorker(path string) int {
	var request commandRequest
	if err := readJSON(path, &request); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 125
	}
	if err := validateRequest(path, request); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 125
	}
	output, err := os.OpenFile(request.Stdout, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return 125
	}
	errorsFile, err := os.OpenFile(request.Stderr, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		output.Close()
		return 125
	}
	inputPath := request.Stdin
	if inputPath == "" {
		inputPath = os.DevNull
	}
	input, err := os.Open(inputPath)
	if err != nil {
		output.Close()
		errorsFile.Close()
		return 125
	}
	argv := append([]string{"--signal=TERM", "--kill-after=10s", strconv.Itoa(request.Seconds)}, request.Argv...)
	command := exec.Command("timeout", argv...)
	command.Dir = request.Directory
	environment := map[string]string{}
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			environment[key] = value
		}
	}
	for key, value := range request.Environment {
		environment[key] = value
	}
	for _, key := range sortedKeys(environment) {
		command.Env = append(command.Env, key+"="+environment[key])
	}
	capturedOutput, capturedErrors := &boundedCapture{File: output, Remaining: capturedJSONLimit}, &boundedCapture{File: errorsFile, Remaining: capturedJSONLimit}
	command.Stdin, command.Stdout, command.Stderr = input, capturedOutput, capturedErrors
	command.WaitDelay = 10 * time.Second
	started := time.Now()
	runErr := command.Run()
	code := exitCode(runErr)
	result := commandResult{Exit: &code, Seconds: time.Since(started).Seconds()}
	captureErr := errors.Join(capturedOutput.Err, capturedErrors.Err, input.Close(), output.Close(), errorsFile.Close())
	if captureErr != nil {
		result.Error = captureErr.Error()
	}
	if err := writeJSON(request.Result, result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 125
	}
	return code
}

// Requests are private capture-local values, not a general arbitrary-file API.
func validateRequest(path string, request commandRequest) error {
	if len(request.Argv) == 0 || request.Argv[0] == "" || request.Seconds < 1 || request.Seconds > 86400 || request.Environment == nil {
		return errors.New("invalid bounded command request")
	}
	if err := physicalPath(request.Directory, true); err != nil {
		return err
	}
	root := filepath.Dir(path)
	if err := physicalPath(root, true); err != nil {
		return err
	}
	paths := map[string]bool{path: true}
	for _, output := range []string{request.Stdout, request.Stderr, request.Result} {
		if !filepath.IsAbs(output) || filepath.Clean(output) != output || filepath.Dir(output) != root || paths[output] || strings.ContainsAny(output, "\x00\r\n") {
			return errors.New("command output is not an independent capture-local path")
		}
		paths[output] = true
		if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
			return errors.New("command output already exists or cannot be inspected")
		}
	}
	if request.Stdin != "" {
		if paths[request.Stdin] {
			return errors.New("command input aliases an output")
		}
		if err := physicalPath(request.Stdin, false); err != nil {
			return err
		}
	}
	for _, argument := range request.Argv {
		if strings.ContainsRune(argument, '\x00') {
			return errors.New("invalid command argument")
		}
	}
	for key, value := range request.Environment {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return errors.New("invalid command environment")
		}
	}
	return nil
}

// Capture each command stream within the existing 64 MiB event-stream ceiling.
// Separate stdout/stderr owners are each written by one exec copier goroutine.
type boundedCapture struct {
	File      io.Writer
	Remaining int
	Err       error
}

func (self *boundedCapture) Write(data []byte) (int, error) {
	if self.Err != nil {
		return 0, self.Err
	}
	if len(data) > self.Remaining {
		self.Err = errors.New("command output exceeds byte bound")
		return 0, self.Err
	}
	count, err := self.File.Write(data)
	self.Remaining -= count
	if count != len(data) && err == nil {
		err = io.ErrShortWrite
	}
	self.Err = err
	return count, err
}

// Immutable maps support concurrent stage commands. Unproven cleanup is sticky
// and forbids all final source-fence claims, including after cancellation.
type stageOwner struct {
	Source      string
	Capture     string
	Environment map[string]string
	Go          string
	Bash        string
	Inputs      map[string]fileProof
	Unproven    atomic.Bool
}

func (self *stageOwner) command(ctx context.Context, name string, argv []string, directory string, seconds int, stdin string) (result commandResult, resultErr error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := checkProofs(self.Inputs); err != nil {
		return result, err
	}
	base := filepath.Join(self.Capture, name)
	request := commandRequest{Argv: argv, Directory: directory, Seconds: seconds, Environment: self.Environment,
		Stdin: stdin, Stdout: base + ".stdout", Stderr: base + ".stderr", Result: base + ".command.json"}
	path := base + ".request.json"
	if err := writeJSON(path, request); err != nil {
		return result, err
	}
	requestProof, err := regularProof(path)
	if err != nil {
		return result, err
	}
	log, err := os.OpenFile(base+".owner.log", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result, err
	}
	// The Bash child inherits only this fixed environment plus the private values
	// established by the qualified helper; plan limits are reapplied by the worker.
	bash := self.Bash
	if bash == "" {
		bash = "bash"
	}
	command := exec.Command(bash, "-c", ownerScript, "qualification-owner", self.Source, filepath.Dir(self.Source), filepath.Join(self.Capture, "runner"), path, filepath.Join(self.Capture, "release-gate-jobs.sh"), base+".joined")
	command.Env = os.Environ()
	for _, key := range sortedKeys(self.Environment) {
		command.Env = append(command.Env, key+"="+self.Environment[key])
	}
	command.Env = append(command.Env, "RELEASE_GATE_JOBS=1")
	command.Stdout, command.Stderr = log, log
	started, proven := false, false
	defer func() {
		if started && !proven {
			self.Unproven.Store(true)
		}
		resultErr = errors.Join(resultErr, log.Close())
	}()
	if err := ctx.Err(); err != nil {
		return commandResult{}, err
	}
	if err := command.Start(); err != nil {
		return commandResult{}, err
	}
	started = true
	joined := make(chan error, 1)
	go func() { joined <- command.Wait() }()
	var waitErr error
	select {
	case waitErr = <-joined:
	case <-ctx.Done():
		signalErr := command.Process.Signal(syscall.SIGTERM)
		waitErr = <-joined
		if signalErr != nil && !errors.Is(signalErr, os.ErrProcessDone) {
			waitErr = errors.Join(waitErr, signalErr)
		}
	}
	readErr := readJSON(request.Result, &result)
	joinedProof, joinedErr := readBounded(base+".joined", int64(len("joined\n")))
	proven = joinedErr == nil && string(joinedProof) == "joined\n"
	if !proven {
		joinedErr = errors.Join(joinedErr, errors.New("owner did not prove every descendant joined"))
	}
	result.Joined = proven
	result.OwnerExit, result.Stdout, result.Stderr = exitCode(waitErr), request.Stdout, request.Stderr
	result.Request, result.OwnerLog = path, base+".owner.log"
	custodyErr := checkProofs(map[string]fileProof{path: requestProof})
	if result.Error != "" {
		readErr = errors.Join(readErr, errors.New(result.Error))
	}
	return result, errors.Join(readErr, joinedErr, custodyErr, ctx.Err(), writeJSON(base+".result.json", result))
}

func commandMatches(result commandResult, expected int) bool {
	return result.Joined && result.Error == "" && result.Exit != nil && *result.Exit == expected && result.OwnerExit == expected
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, info, err := openRegular(source)
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return errors.Join(err, input.Close())
	}
	count, writeErr := io.Copy(output, io.LimitReader(input, info.Size()+1))
	if count != info.Size() {
		writeErr = errors.Join(writeErr, errors.New("copy source changed size"))
	}
	return errors.Join(writeErr, sameRegular(source, input, info), input.Close(), output.Close())
}

// Compare both mode and content, retaining an explicit error for every mismatch.
func checkProofs(expected map[string]fileProof) error {
	for _, path := range sortedKeys(expected) {
		actual, err := regularProof(path)
		if err != nil {
			return fmt.Errorf("input fence %s: %w", path, err)
		}
		if actual != expected[path] {
			return fmt.Errorf("input fence changed: %s", path)
		}
	}
	return nil
}

func executeStage(ctx context.Context, name string, node stage, owner *stageOwner, plan planSpec, packages map[string]packageSpec) stageResult {
	failed := func(err error, result *commandResult) stageResult {
		text := "command outcome differs"
		if err != nil {
			text = err.Error()
		}
		return stageResult{Status: "failed", Error: text, Command: result}
	}
	if node.Build != nil {
		item := node.Build
		pkg := packages[item.Package]
		identity, err := owner.command(ctx, name+"-identity", []string{"go", "list", "-f", "{{.ImportPath}}", "."}, pkg.Directory, plan.Limits.BuildSeconds, "")
		if err != nil || !commandMatches(identity, 0) {
			return failed(err, &identity)
		}
		actual, err := readBounded(identity.Stdout, metadataLimit)
		if err != nil || strings.TrimSpace(string(actual)) != pkg.ImportPath {
			return failed(errors.New("actual Go import path differs"), &identity)
		}
		graph, err := owner.command(ctx, name+"-modules", []string{"go", "list", "-m", "-json", "all"}, pkg.Directory, plan.Limits.BuildSeconds, "")
		if err != nil || !commandMatches(graph, 0) {
			return failed(err, &graph)
		}
		binary := filepath.Join(owner.Capture, name+".testbin")
		argv := []string{"go", "test", "-c", "-p=" + strconv.Itoa(plan.Limits.Parallel)}
		if item.Mode == "race" {
			argv = append(argv, "-race")
		}
		argv = append(argv, "-o", binary, ".")
		result, err := owner.command(ctx, name, argv, pkg.Directory, plan.Limits.BuildSeconds, "")
		if err != nil || !commandMatches(result, 0) {
			return failed(err, &result)
		}
		hash, err := fileHash(binary)
		if err != nil {
			return failed(err, &result)
		}
		if err := writeJSON(binary+".json", fileProof{SHA256: hash}); err != nil {
			return failed(err, &result)
		}
		return stageResult{Status: "passed", BinarySHA256: hash, Seconds: result.Seconds, Command: &result}
	}
	item := node.Suite
	pkg := packages[item.Package]
	expected, err := expectedInputs(item.Outcomes, item.FailureLiterals)
	if err != nil {
		return failed(err, nil)
	}
	selector := "^(" + strings.Join(expected.Roots, "|") + ")$"
	binary := filepath.Join(owner.Capture, node.Binary+".testbin")
	var proof fileProof
	if err := readJSON(binary+".json", &proof); err != nil {
		return failed(err, nil)
	}
	hash, err := fileHash(binary)
	if err != nil || hash != proof.SHA256 {
		return failed(errors.New("test binary changed before execution"), nil)
	}
	listed, err := owner.command(ctx, name+"-list", []string{binary, "-test.list=" + selector}, pkg.Directory, plan.Limits.BuildSeconds, "")
	if err != nil || !commandMatches(listed, 0) {
		return failed(err, &listed)
	}
	actual, err := metadataRows(listed.Stdout, false)
	sort.Strings(actual)
	if err != nil || !reflect.DeepEqual(actual, expected.Roots) {
		return failed(errors.New("compiled root census differs"), &listed)
	}
	result, err := owner.command(ctx, name, []string{binary, "-test.run=" + selector, "-test.v=test2json", "-test.count=1", "-test.parallel=" + strconv.Itoa(plan.Limits.Parallel), "-test.timeout=" + strconv.Itoa(plan.Limits.TestSeconds) + "s"}, pkg.Directory, plan.Limits.OuterSeconds, "")
	wanted := 0
	if len(expected.Markers) != 0 {
		wanted = 1
	}
	if err != nil || !commandMatches(result, wanted) {
		return failed(err, &result)
	}
	converted, err := owner.command(ctx, name+"-events", []string{"go", "tool", "test2json", "-t", "-p", pkg.ImportPath}, pkg.Directory, plan.Limits.BuildSeconds, result.Stdout)
	if err != nil || !commandMatches(converted, 0) {
		return failed(err, &converted)
	}
	events, err := os.Open(converted.Stdout)
	if err != nil {
		return failed(err, &result)
	}
	checked, checkErr := verifyEvents(events, expected, pkg.ImportPath, *result.Exit)
	closeErr := events.Close()
	if err := errors.Join(checkErr, closeErr); err != nil {
		return failed(err, &result)
	}
	hash, err = fileHash(binary)
	if err != nil || hash != proof.SHA256 {
		return failed(errors.New("test binary changed during execution"), &result)
	}
	return stageResult{Status: "passed", Seconds: result.Seconds, BinarySHA256: hash, Verification: &checked, Events: converted.Stdout, Command: &result}
}

func runMatrix(ctx context.Context, planPath, capture string) (matrixStatus, error) {
	var plan planSpec
	if err := readJSON(planPath, &plan); err != nil {
		return matrixStatus{}, err
	}
	if err := validatePlan(plan); err != nil {
		return matrixStatus{}, err
	}
	before, err := sourceFence(plan.Sources)
	if err != nil {
		return matrixStatus{}, err
	}
	if !filepath.IsAbs(capture) || filepath.Clean(capture) != capture {
		return matrixStatus{}, errors.New("capture path must be absolute and canonical")
	}
	if err := physicalPath(filepath.Dir(capture), true); err != nil {
		return matrixStatus{}, err
	}
	for _, source := range plan.Sources {
		if inside(capture, source.Root) {
			return matrixStatus{}, errors.New("capture must not mutate a source tree")
		}
	}
	helperPaths := []string{}
	for _, name := range []string{"release-gate-jobs.sh", "release-gate-child.py"} {
		path := filepath.Join(plan.SourceRoot, "scripts", name)
		if err := physicalPath(path, false); err != nil {
			return matrixStatus{}, err
		}
		helperPaths = append(helperPaths, path)
	}
	if err := os.Mkdir(capture, 0700); err != nil {
		return matrixStatus{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return matrixStatus{}, err
	}
	if err := copyFile(executable, filepath.Join(capture, "runner"), 0700); err != nil {
		return matrixStatus{}, err
	}
	for _, path := range helperPaths {
		if err := copyFile(path, filepath.Join(capture, filepath.Base(path)), 0600); err != nil {
			return matrixStatus{}, err
		}
	}
	if err := writeJSON(filepath.Join(capture, "source.before.json"), before); err != nil {
		return matrixStatus{}, err
	}
	for index := range plan.Suites {
		suite := &plan.Suites[index]
		for suffix, source := range map[string]*string{"outcomes": &suite.Outcomes, "literals": &suite.FailureLiterals} {
			destination := filepath.Join(capture, suite.Id+"."+suffix+".tsv")
			if err := copyFile(*source, destination, 0600); err != nil {
				return matrixStatus{}, err
			}
			*source = destination
		}
	}
	if err := writeJSON(filepath.Join(capture, "plan.json"), plan); err != nil {
		return matrixStatus{}, err
	}
	inputs := map[string]string{}
	entries, err := os.ReadDir(capture)
	if err != nil {
		return matrixStatus{}, err
	}
	for _, entry := range entries {
		path := filepath.Join(capture, entry.Name())
		hash, err := fileHash(path)
		if err != nil {
			return matrixStatus{}, err
		}
		inputs[entry.Name()] = hash
	}
	if err := writeJSON(filepath.Join(capture, "inputs.json"), inputs); err != nil {
		return matrixStatus{}, err
	}
	owner := &stageOwner{Source: plan.SourceRoot, Capture: capture, Environment: map[string]string{"GOMAXPROCS": strconv.Itoa(plan.Limits.GOMAXPROCS), "GOFLAGS": "-mod=readonly", "GOPROXY": "off", "GOSUMDB": "off", "GOWORK": "off"}}
	packages := map[string]packageSpec{}
	for _, pkg := range plan.Packages {
		packages[pkg.Id] = pkg
	}
	nodes := map[string]stage{}
	for index := range plan.Suites {
		suite := &plan.Suites[index]
		build := "build-" + suite.Package + "-" + suite.Mode
		nodes[build] = stage{Build: suite}
		nodes["suite-"+suite.Id] = stage{Dependencies: []string{build}, Suite: suite, Binary: build}
	}
	status := matrixStatus{State: "running", Total: len(nodes), Report: filepath.Join(capture, "report.json")}
	publish := func(results map[string]stageResult, running []string, pending int) error {
		status.Finished, status.Running, status.Pending = len(results), running, pending
		status.Failed = nil
		for _, name := range sortedKeys(results) {
			if results[name].Status != "passed" {
				status.Failed = append(status.Failed, name)
			}
		}
		return writeJSON(filepath.Join(capture, "status.json"), status)
	}
	results, runErr := runDAG(ctx, nodes, plan.Limits.Jobs, func(ctx context.Context, name string, node stage) stageResult {
		return executeStage(ctx, name, node, owner, plan, packages)
	}, publish)
	after, fenceErr := sourceFence(plan.Sources)
	afterErr := writeJSON(filepath.Join(capture, "source.after.json"), after)
	unchanged := fenceErr == nil && reflect.DeepEqual(before, after)
	for name, expected := range inputs {
		hash, err := fileHash(filepath.Join(capture, name))
		unchanged = unchanged && err == nil && hash == expected
	}
	status.State, status.SourceUnchanged, status.Finished, status.Running, status.Pending = "failed", unchanged, len(results), nil, 0
	status.Failed = nil
	for _, name := range sortedKeys(results) {
		if results[name].Status != "passed" {
			status.Failed = append(status.Failed, name)
		}
	}
	finalErr := errors.Join(runErr, fenceErr, afterErr, ctx.Err())
	if finalErr != nil {
		status.Error = finalErr.Error()
	}
	if finalErr == nil && unchanged && len(status.Failed) == 0 {
		status.State = "passed"
	}
	report := matrixReport{Status: status, Results: results}
	reportErr := writeJSON(status.Report, report)
	failureStages := map[string]stageResult{}
	for _, name := range status.Failed {
		failureStages[name] = results[name]
	}
	failureErr := writeJSON(filepath.Join(capture, "failures.json"), matrixReport{Status: status, Results: failureStages})
	statusErr := writeJSON(filepath.Join(capture, "status.json"), status)
	return status, errors.Join(finalErr, reportErr, failureErr, statusErr)
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "worker" {
		os.Exit(runWorker(os.Args[2]))
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var result any
	var err error
	if len(os.Args) == 3 && os.Args[1] == "status" {
		var status matrixStatus
		err = readJSON(filepath.Join(os.Args[2], "status.json"), &status)
		result = status
	} else if len(os.Args) == 4 && os.Args[1] == "run" {
		var status matrixStatus
		status, err = runMatrix(ctx, os.Args[2], os.Args[3])
		result = status
		if err == nil && status.State != "passed" {
			err = errors.New("qualification did not pass")
		}
	} else {
		err = errors.New("usage: qualification run PLAN.json NEW_CAPTURE | status CAPTURE")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if status, ok := result.(matrixStatus); !ok || status.State == "" {
			result = matrixStatus{State: "refused", Error: err.Error()}
		}
	}
	encoded, encodeErr := json.Marshal(result)
	if encodeErr == nil {
		fmt.Println(string(bytes.TrimSpace(encoded)))
	}
	if err != nil || encodeErr != nil {
		os.Exit(1)
	}
}
