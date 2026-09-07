package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type releaseGateJobsFixture struct {
	root       string
	command    *exec.Cmd
	output     bytes.Buffer
	files      map[string]*os.File
	lines      map[string]chan string
	readers    sync.WaitGroup
	readerStop chan struct{}
	finished   chan struct{}
	err        error
}

// Uses the actual queue and subreaper with only machine capacity and the
// external service cleanup callback supplied as deterministic boundaries.
func newReleaseGateJobsFixture(t *testing.T, body string) *releaseGateJobsFixture {
	t.Helper()
	self := &releaseGateJobsFixture{root: t.TempDir(), files: map[string]*os.File{}, lines: map[string]chan string{}, readerStop: make(chan struct{}), finished: make(chan struct{})}
	constructed := false
	defer func() {
		if !constructed {
			close(self.readerStop)
			for _, file := range self.files {
				_ = file.Close()
			}
			self.readers.Wait()
		}
	}()
	for _, name := range []string{"events", "control-a", "control-b", "control-c", "control", "leaf-control", "ack-relay"} {
		path := filepath.Join(self.root, name)
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		self.files[name] = file
		if name == "events" || name == "ack-relay" {
			lines := make(chan string, 8)
			self.lines[name] = lines
			self.readers.Add(1)
			go func() {
				defer self.readers.Done()
				defer close(lines)
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					select {
					case lines <- scanner.Text():
					case <-self.readerStop:
						return
					}
				}
			}()
		}
	}
	workspace := filepath.Join(self.root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "server", "local"), 0o700); err != nil {
		t.Fatal(err)
	}
	cleanup := `release_gate_services_cleanup() {
  if [[ -f "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/descendant-pid" ]]; then
    local pid
    pid="$(< "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/descendant-pid")"
    [[ ! -e "/proc/$pid" ]] || return 95
  fi
  printf 'cleaned\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/cleaned"
}
`
	if err := os.WriteFile(filepath.Join(workspace, "server", "local", "release-gate-services.sh"), []byte(cleanup), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(self.root, "descendant.py"), []byte(releaseGateDescendantFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	helper, err := filepath.Abs("../scripts/release-gate-jobs.sh")
	if err != nil {
		t.Fatal(err)
	}
	sn, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	self.command = exec.CommandContext(ctx, "bash", "-c", `set -euo pipefail
source "$1"
sn_repo="$2"; workspace="$3"
release_gate_effective_resources() { RELEASE_GATE_CPUS=24; RELEASE_GATE_MEMORY=34359738368; }
release_gate_jobs_init
`+body, "release-jobs-test", helper, sn, workspace)
	self.command.Env = append(os.Environ(), "TMPDIR="+self.root, "RELEASE_GATE_QUEUE_FIXTURE_ROOT="+self.root, "RELEASE_GATE_JOBS=2", "RELEASE_GATE_CONCURRENT_GATES=2", "SIM_TESTNET_LIVE_DEPENDENCIES=0")
	self.command.Cancel = func() error { return self.command.Process.Signal(syscall.SIGTERM) }
	self.command.WaitDelay = 20 * time.Second
	self.command.Stdout, self.command.Stderr = &self.output, &self.output
	if err := self.command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	go func() { self.err = self.command.Wait(); close(self.finished) }()
	t.Cleanup(func() {
		cancel()
		for _, name := range []string{"control-a", "control-b", "control-c", "control", "leaf-control"} {
			_, _ = self.files[name].WriteString("release\n")
		}
		select {
		case <-self.finished:
		case <-time.After(25 * time.Second):
			t.Error("foreground queue did not join after cancellation")
			_ = self.command.Process.Kill()
			<-self.finished
		}
		close(self.readerStop)
		for _, file := range self.files {
			_ = file.Close()
		}
		self.readers.Wait()
	})
	constructed = true
	return self
}

func (self *releaseGateJobsFixture) line(t *testing.T, name string) string {
	t.Helper()
	select {
	case line, ok := <-self.lines[name]:
		if !ok {
			t.Fatalf("queue %s reader closed", name)
		}
		return line
	case <-self.finished:
		t.Fatalf("queue exited before %s barrier: %v\n%s", name, self.err, self.output.String())
	case <-time.After(20 * time.Second):
		t.Fatalf("queue did not reach %s barrier", name)
	}
	return ""
}

func (self *releaseGateJobsFixture) write(t *testing.T, name, value string) {
	t.Helper()
	if _, err := self.files[name].WriteString(value + "\n"); err != nil {
		t.Fatal(err)
	}
}

func (self *releaseGateJobsFixture) join(t *testing.T, status int) {
	t.Helper()
	select {
	case <-self.finished:
	case <-time.After(20 * time.Second):
		t.Fatal("queue did not join")
	}
	if self.command.ProcessState.ExitCode() != status {
		t.Fatalf("queue exit=%d want=%d error=%v\n%s", self.command.ProcessState.ExitCode(), status, self.err, self.output.String())
	}
}

func TestReleaseGateJobsRunReadyPhasesConcurrentlyAndJoinFailures(t *testing.T) {
	self := newReleaseGateJobsFixture(t, `
phase_a() { printf 'a\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"; return 23; }
phase_b() { printf 'b\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-b"; }
release_gate_start a phase_a
release_gate_start b phase_b
release_gate_wait_one
[[ "$release_gate_result" == 23 && "$release_gate_active" == 1 ]]
printf 'first-joined\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
status=0; release_gate_complete || status=$?
[[ "$release_gate_active" == 0 && "$release_gate_services_cleaned" == 1 && "$status" == 23 ]]
exit "$status"
`)
	first, second := self.line(t, "events"), self.line(t, "events")
	if first+second != "ab" && first+second != "ba" {
		t.Fatalf("two ready workers = %q %q", first, second)
	}
	self.write(t, "control-a", "release")
	if got := self.line(t, "events"); got != "first-joined" {
		t.Fatalf("failure join boundary = %q", got)
	}
	self.write(t, "control-b", "release")
	self.join(t, 23)
	if _, err := os.Stat(filepath.Join(self.root, "cleaned")); err != nil {
		t.Fatalf("failed but joined phases omitted resource cleanup: %v", err)
	}
}

func TestReleaseGateJobsBoundReadyAdmissionByEffectiveCapacity(t *testing.T) {
	self := newReleaseGateJobsFixture(t, `
phase_a() { printf 'a\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"; }
phase_b() { printf 'b\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-b"; }
phase_c() { printf 'c\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-c"; }
release_gate_start a phase_a
release_gate_start b phase_b
original_wait="$(declare -f release_gate_wait_one)"
eval "${original_wait/release_gate_wait_one/release_gate_wait_one_real}"
release_gate_wait_one() {
  [[ "$release_gate_active" == 2 ]]
  printf 'capacity-wait\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
  release_gate_wait_one_real
}
release_gate_start c phase_c
eval "$original_wait"
release_gate_complete
`)
	seen := map[string]bool{}
	for range 3 {
		seen[self.line(t, "events")] = true
	}
	if !seen["a"] || !seen["b"] || !seen["capacity-wait"] {
		t.Fatalf("capacity barrier entries=%v", seen)
	}
	self.write(t, "control-a", "release")
	if got := self.line(t, "events"); got != "c" {
		t.Fatalf("next admitted worker=%q", got)
	}
	self.write(t, "control-b", "release")
	self.write(t, "control-c", "release")
	self.join(t, 0)
}

func TestReleaseGateJobsCancellationReapsNewSessionBeforeServiceCleanup(t *testing.T) {
	self := newReleaseGateJobsFixture(t, `
phase_tree() { python3 "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/descendant.py" "$RELEASE_GATE_QUEUE_FIXTURE_ROOT" session; }
release_gate_start tree phase_tree
read -r hold < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"
`)
	pid := self.line(t, "events")
	if err := os.WriteFile(filepath.Join(self.root, "descendant-pid"), []byte(pid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := self.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	self.join(t, 143)
	assertReleaseGateDescendantReaped(t, self.root, pid)
	if _, err := os.Stat(filepath.Join(self.root, "cleaned")); err != nil {
		t.Fatalf("canceled jobs were not reaped before cleanup: %v", err)
	}
}

// Relay the queue's acknowledgement through an explicit test boundary. The
// worker completion remains zero, but the real owner rejects the wrong token.
func TestReleaseGateJobsPreserveRealPostCompletionFailure(t *testing.T) {
	self := newReleaseGateJobsFixture(t, `
phase_ok() { true; }
release_gate_start ok phase_ok
original_ack="${release_gate_ack_fds[0]}"
exec {relay_ack}<>"$RELEASE_GATE_QUEUE_FIXTURE_ROOT/ack-relay"
release_gate_ack_fds[0]="$relay_ack"
printf '%s\n' "$release_gate_root" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
release_gate_wait_one
exec {original_ack}>&-
[[ "$release_gate_result" == 125 && "$RELEASE_GATE_JOB_EXIT" == 125 && "$release_gate_active" == 0 ]]
status=0; release_gate_complete || status=$?
exit "$status"
`)
	root := self.line(t, "events")
	acknowledgement, err := os.OpenFile(filepath.Join(root, "job-0", "ack"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = acknowledgement.WriteString("joined\n"); _ = acknowledgement.Close() })
	if got := self.line(t, "ack-relay"); got != "joined" {
		t.Fatalf("queue acknowledgement=%q", got)
	}
	if _, err := acknowledgement.WriteString("wrong-owner\n"); err != nil {
		t.Fatal(err)
	}
	self.join(t, 125)
	if !strings.Contains(self.output.String(), "worker/owner exit mismatch: 0/125") {
		t.Fatalf("real owner failure lost attribution:\n%s", self.output.String())
	}
}

func TestReleaseGateJobsUsePrivateArtifactsCachesAndSourceReferences(t *testing.T) {
	self := newReleaseGateJobsFixture(t, `
[[ "$FOUNDRY_OUT" != "$SLITHER_FOUNDRY_OUT" && "$FOUNDRY_CACHE_PATH" != "$SLITHER_FOUNDRY_CACHE_PATH" ]]
[[ "$(readlink "$release_gate_root/src")" == "$sn_repo/evm/src" ]]
[[ "$(readlink "$release_gate_root/lib")" == "$sn_repo/evm/lib" ]]
[[ "${FOUNDRY_OUT%/*}" == "$release_gate_root" && "${SLITHER_FOUNDRY_OUT%/*}" == "$release_gate_root" ]]
[[ "$release_gate_limit" == 2 && "$GOMAXPROCS" == 4 && "$GOFLAGS" == *-p=4 ]]
release_gate_complete
`)
	self.join(t, 0)
}

func TestReleaseGateJobsTwoForegroundOwnersHaveDisjointMutableRoots(t *testing.T) {
	body := `
phase_hold() { printf '%s\n' "$release_gate_root" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"; }
release_gate_start hold phase_hold
release_gate_complete
`
	first, second := newReleaseGateJobsFixture(t, body), newReleaseGateJobsFixture(t, body)
	firstRoot, secondRoot := first.line(t, "events"), second.line(t, "events")
	if firstRoot == secondRoot || filepath.Dir(firstRoot) != first.root || filepath.Dir(secondRoot) != second.root {
		t.Fatalf("independent gate roots=%q %q", firstRoot, secondRoot)
	}
	first.write(t, "control-a", "release")
	first.join(t, 0)
	if _, err := os.Stat(secondRoot); err != nil {
		t.Fatalf("first owner damaged second root: %v", err)
	}
	second.write(t, "control-a", "release")
	second.join(t, 0)
}

func assertReleaseGateEarlyExit(t *testing.T, parentStatus, expected int) {
	t.Helper()
	self := newReleaseGateJobsFixture(t, `
phase_hold() { printf 'held\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"; }
release_gate_start hold phase_hold
read -r leave < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-b"
exit `+strconv.Itoa(parentStatus)+"\n")
	if got := self.line(t, "events"); got != "held" {
		t.Fatalf("early-exit worker barrier=%q", got)
	}
	self.write(t, "control-b", "leave")
	self.join(t, expected)
	if _, err := os.Stat(filepath.Join(self.root, "cleaned")); err != nil {
		t.Fatalf("early exit failed to join before cleanup: %v", err)
	}
}

func TestReleaseGateJobsExitCannotHideCanceledWorkerFailure(t *testing.T) {
	assertReleaseGateEarlyExit(t, 0, 143)
}

func TestReleaseGateJobsExitPreservesOriginalParentFailure(t *testing.T) {
	assertReleaseGateEarlyExit(t, 17, 17)
}

// Lose the completion endpoint only after admission. The real owner still
// reaps its worker, but cannot publish proof, so services must remain untouched.
func TestReleaseGateJobsLostOwnerRetainsServices(t *testing.T) {
	self := newReleaseGateJobsFixture(t, `
phase_hold() { printf 'held:%s\n' "$RELEASE_GATE_WORKER_PID" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"; }
release_gate_start hold phase_hold
printf 'root:%s\n' "$release_gate_root" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
read -r continue < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-b"
if release_gate_complete; then exit 90; fi
[[ "$release_gate_active" == 1 ]]
exit 1
`)
	root, worker := "", ""
	for range 2 {
		line := self.line(t, "events")
		if strings.HasPrefix(line, "root:") {
			root = strings.TrimPrefix(line, "root:")
		} else if strings.HasPrefix(line, "held:") {
			worker = strings.TrimPrefix(line, "held:")
		} else {
			t.Fatalf("unexpected admission event=%q", line)
		}
	}
	if root == "" {
		t.Fatal("missing admitted owner root")
	}
	if pid, err := strconv.Atoi(worker); err != nil || pid <= 0 {
		t.Fatalf("invalid admitted worker PID=%q: %v", worker, err)
	}
	if err := os.Rename(filepath.Join(root, "completions"), filepath.Join(root, "unlinked-completions")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "completions"), 0o600); err != nil {
		t.Fatal(err)
	}
	self.write(t, "control-a", "release")
	self.write(t, "control-b", "continue")
	self.join(t, 1)
	if _, err := os.Stat(filepath.Join("/proc", worker)); !os.IsNotExist(err) {
		t.Fatalf("lost completion owner did not reap worker %s: %v", worker, err)
	}
	if _, err := os.Stat(filepath.Join(self.root, "cleaned")); !os.IsNotExist(err) {
		t.Fatalf("lost owner permitted service mutation: %v", err)
	}
	if !strings.Contains(self.output.String(), "private services retained") {
		t.Fatalf("lost owner did not report unproven cleanup:\n%s", self.output.String())
	}
}

// A real failed completion must not suppress a held peer's actual ack/wait.
// Either owner order and both incoming exit statuses retain the unproven slot.
func TestReleaseGateJobsExitJoinsSurvivorAfterLostOwner(t *testing.T) {
	for _, lostIndex := range []int{0, 1} {
		for _, parentStatus := range []int{0, 17} {
			self := newReleaseGateJobsFixture(t, `
phase_lost() { printf 'lost-worker:%s\n' "$RELEASE_GATE_WORKER_PID" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"; }
phase_survivor() { printf 'survivor-worker:%s\n' "$RELEASE_GATE_WORKER_PID" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-b"; }
lost_index=`+strconv.Itoa(lostIndex)+`
survivor_index=$((1 - lost_index))
if (( lost_index == 0 )); then
  release_gate_start lost phase_lost
  release_gate_start survivor phase_survivor
else
  release_gate_start survivor phase_survivor
  release_gate_start lost phase_lost
fi
printf 'root:%s\n' "$release_gate_root" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
printf 'owners:%s:%s\n' "${release_gate_pids[lost_index]}" "${release_gate_pids[survivor_index]}" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
read -r endpoint_lost < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-c"
lost_status=0
wait "${release_gate_pids[lost_index]}" || lost_status=$?
[[ "$lost_status" == 1 && "$release_gate_active" == 2 && "${release_gate_pending[lost_index]}" == 1 ]]
printf 'lost-owner-reaped\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
read -r endpoint_restored < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-c"

# Observe the real EXIT boundary before compensating cleanup. The compensation
# keeps this regression safe on the old queue: it cannot change the captured
# pending slots, parent status, or missing production join acknowledgement.
fixture_exiting=0
original_exit="$(declare -f release_gate_jobs_exit)"
eval "${original_exit/release_gate_jobs_exit/release_gate_jobs_exit_real}"
release_gate_jobs_exit() {
  fixture_exiting=1
  release_gate_jobs_exit_real "$@"
}
exit() {
  local status="${1:-0}"
  if (( fixture_exiting == 1 )); then
    printf '%s %s %s %s\n' "$release_gate_active" "${release_gate_pending[lost_index]}" "${release_gate_pending[survivor_index]}" "$status" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/exit-state"
    while [[ "${release_gate_pending[survivor_index]}" == 1 ]] && release_gate_owned_job "$survivor_index"; do
      release_gate_wait_one || :
    done
  fi
  builtin exit "$status"
}
exit `+strconv.Itoa(parentStatus)+"\n")
			admitted := map[string]string{}
			for range 4 {
				line := self.line(t, "events")
				name, value, ok := strings.Cut(line, ":")
				if !ok || admitted[name] != "" {
					t.Fatalf("unexpected admission event=%q", line)
				}
				admitted[name] = value
			}
			root := admitted["root"]
			if filepath.Dir(root) != self.root {
				t.Fatalf("unexpected admitted owner root=%q", root)
			}
			lostOwner, survivorOwner, ok := strings.Cut(admitted["owners"], ":")
			if !ok {
				t.Fatalf("missing actual owner identities=%q", admitted["owners"])
			}
			for _, pid := range []string{lostOwner, survivorOwner, admitted["lost-worker"], admitted["survivor-worker"]} {
				if value, err := strconv.Atoi(pid); err != nil || value <= 0 {
					t.Fatalf("invalid admitted PID=%q: %v", pid, err)
				}
			}
			completion := filepath.Join(root, "completions")
			retainedCompletion := filepath.Join(root, "unlinked-completions")
			if err := os.Rename(completion, retainedCompletion); err != nil {
				t.Fatal(err)
			}
			restoreCompletion := func() error {
				if _, err := os.Stat(retainedCompletion); os.IsNotExist(err) {
					return nil
				} else if err != nil {
					return err
				}
				return os.Rename(retainedCompletion, completion)
			}
			t.Cleanup(func() {
				if err := restoreCompletion(); err != nil {
					t.Errorf("restore owned completion before fixture cleanup: %v", err)
				}
			})
			if err := syscall.Mkfifo(completion, 0o600); err != nil {
				t.Fatal(err)
			}
			self.write(t, "control-a", "release")
			self.write(t, "control-c", "endpoint-lost")
			if got := self.line(t, "events"); got != "lost-owner-reaped" {
				t.Fatalf("lost-owner wait boundary=%q", got)
			}
			for _, pid := range []string{lostOwner, admitted["lost-worker"]} {
				if _, err := os.Stat(filepath.Join("/proc", pid)); !os.IsNotExist(err) {
					t.Fatalf("lost owner failed its actual wait/reap for %s: %v", pid, err)
				}
			}
			for _, pid := range []string{survivorOwner, admitted["survivor-worker"]} {
				value, _ := strconv.Atoi(pid)
				if err := syscall.Kill(value, 0); err != nil {
					t.Fatalf("lost completion affected the independently held survivor %s: %v", pid, err)
				}
			}
			if err := restoreCompletion(); err != nil {
				t.Fatal(err)
			}
			self.write(t, "control-c", "endpoint-restored")
			select {
			case <-self.finished:
			case <-time.After(20 * time.Second):
				t.Fatal("EXIT did not finish the surviving owner's explicit join")
			}
			state, err := os.ReadFile(filepath.Join(self.root, "exit-state"))
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := parentStatus
			if wantStatus == 0 {
				wantStatus = 143
			}
			wantState := "1 1 0 " + strconv.Itoa(wantStatus) + "\n"
			if string(state) != wantState {
				t.Fatalf("lost owner suppressed surviving phase join (lost index=%d, parent=%d): exit state=%q want=%q\n%s", lostIndex, parentStatus, state, wantState, self.output.String())
			}
			self.join(t, wantStatus)
			for _, pid := range []string{survivorOwner, admitted["survivor-worker"]} {
				if _, err := os.Stat(filepath.Join("/proc", pid)); !os.IsNotExist(err) {
					t.Fatalf("surviving owner was not explicitly reaped before EXIT: %s: %v", pid, err)
				}
			}
			if _, err := os.Stat(filepath.Join(self.root, "cleaned")); !os.IsNotExist(err) {
				t.Fatalf("unproven pending owner permitted service mutation: %v", err)
			}
			if !strings.Contains(self.output.String(), "joined survivor (exit 143)") || !strings.Contains(self.output.String(), "private services retained") {
				t.Fatalf("survivor join or unproven cleanup lost attribution:\n%s", self.output.String())
			}
		}
	}
}

// Execute the actual final-fence source from both gates, with only external
// Git/Go/source-snapshot commands replaced by finite recording boundaries.
func TestReleaseGateJobsFailedPhaseStillRunsEveryFinalSourceFence(t *testing.T) {
	for _, name := range []string{"test-release-1.0-local.sh", "test-release-1.0-producer-gate.sh"} {
		data, err := os.ReadFile(filepath.Join("..", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		start := strings.Index(string(data), "# Join every admitted phase before inspecting the source again")
		if start < 0 {
			t.Fatalf("%s final owned-fence boundary is missing", name)
		}
		body := `
phase_fail() { return 23; }
release_gate_start failed phase_fail
go() { printf 'go %s\n' "$*" >> "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/fence-events"; }
git() { printf 'git %s\n' "$*" >> "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/fence-events"; }
sn_repo="$RELEASE_GATE_QUEUE_FIXTURE_ROOT/fence-sn"
release_repos=(sn)
release_source_snapshot=frozen
printf 'fence-ready\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
read -r continue < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"
` + string(data)[start:]
		self := newReleaseGateJobsFixture(t, body)
		if got := self.line(t, "events"); got != "fence-ready" {
			t.Fatalf("fence preparation=%q", got)
		}
		scripts := filepath.Join(self.root, "fence-sn", "scripts")
		if err := os.MkdirAll(scripts, 0o700); err != nil {
			t.Fatal(err)
		}
		checker := "#!/bin/sh\nprintf 'snapshot\\n' >> \"$RELEASE_GATE_QUEUE_FIXTURE_ROOT/fence-events\"\nprintf 'frozen\\n'\n"
		if err := os.WriteFile(filepath.Join(scripts, "check-release-source-freeze.sh"), []byte(checker), 0o700); err != nil {
			t.Fatal(err)
		}
		self.write(t, "control-a", "continue")
		self.join(t, 23)
		events, err := os.ReadFile(filepath.Join(self.root, "fence-events"))
		if err != nil {
			t.Fatal(err)
		}
		gitCount, goCount, snapshotCount := 0, 0, 0
		for _, line := range strings.Split(string(events), "\n") {
			if strings.HasPrefix(line, "git ") {
				gitCount++
			}
			if strings.HasPrefix(line, "go ") {
				goCount++
			}
			if line == "snapshot" {
				snapshotCount++
			}
		}
		wantGo := 1
		if strings.Contains(name, "producer") {
			wantGo = 2
		}
		if gitCount != 2 || goCount != wantGo || snapshotCount != 1 {
			t.Fatalf("%s lost final fences after worker23:\n%s", name, events)
		}
		if strings.Contains(self.output.String(), "gate passed") {
			t.Fatalf("%s hid failed phase behind passing fences", name)
		}
	}
}

func TestReleaseGateJobsEarlierSourceFenceFailureSurvivesLaterSuccess(t *testing.T) {
	data, err := os.ReadFile("../scripts/test-release-1.0-producer-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(data), "# Join every admitted phase before inspecting the source again")
	if start < 0 {
		t.Fatal("missing producer final-fence boundary")
	}
	self := newReleaseGateJobsFixture(t, `
phase_ok() { true; }
release_gate_start ok phase_ok
go() {
  printf 'go %s\n' "$*" >> "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/fence-events"
  [[ "$*" != *TestReleaseGatesPinProviderAndTransportRegressions* ]]
}
git() { :; }
sn_repo="$RELEASE_GATE_QUEUE_FIXTURE_ROOT/fence-sn"
release_repos=(sn)
release_source_snapshot=frozen
printf 'fence-ready\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
read -r continue < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"
`+string(data)[start:])
	if got := self.line(t, "events"); got != "fence-ready" {
		t.Fatalf("fence preparation=%q", got)
	}
	scripts := filepath.Join(self.root, "fence-sn", "scripts")
	if err := os.MkdirAll(scripts, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "check-release-source-freeze.sh"), []byte("#!/bin/sh\nprintf 'frozen\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	self.write(t, "control-a", "continue")
	self.join(t, 1)
	events, err := os.ReadFile(filepath.Join(self.root, "fence-events"))
	if err != nil || !strings.Contains(string(events), "TestReleaseLockMatchesCheckout") {
		t.Fatalf("later source fence did not run: %v\n%s", err, events)
	}
	if strings.Contains(self.output.String(), "gate passed") {
		t.Fatalf("later source success hid the earlier failure:\n%s", self.output.String())
	}
}

func TestReleaseGateIsolationPinsPrivateResourcesAndFinalJoins(t *testing.T) {
	for _, name := range []string{"test-release-1.0-local.sh", "test-release-1.0-producer-gate.sh"} {
		data, err := os.ReadFile(filepath.Join("..", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		script := string(data)
		for _, required := range []string{`source "$sn_repo/scripts/release-gate-jobs.sh"`, "release_gate_jobs_init", `source "$RELEASE_GATE_SERVICE_ENV"`, `source "$workspace/server/test-env.sh"`, "test_env_validate_suite_resource_manifest", `--check "$FOUNDRY_OUT" sim-testnet/contracts_gen.go`, `--check --artifacts "$FOUNDRY_OUT"`, "release_gate_complete || release_gate_status=$?", "release_gate_active != 0", "release_gate_fence_status"} {
			if !strings.Contains(script, required) {
				t.Errorf("%s omits private/owned boundary %q", name, required)
			}
		}
		for _, forbidden := range []string{"local-pg.bringyour.com", "local-redis.bringyour.com", "--check evm/out", "& disown"} {
			if strings.Contains(script, forbidden) {
				t.Errorf("%s retains shared/detached resource %q", name, forbidden)
			}
		}
		join := strings.LastIndex(script, "release_gate_complete")
		fence := strings.LastIndex(script, "final source-freeze checkout")
		if join < 0 || fence <= join {
			t.Errorf("%s final source fence precedes owned joins", name)
		}
		start := strings.Index(script, "release_phase_solidity() {")
		if start < 0 {
			t.Fatalf("%s lacks one full-build/generator owner", name)
		}
		end := strings.Index(script[start:], "\n}")
		if end < 0 {
			t.Fatal("unterminated Solidity phase")
		}
		phase := script[start : start+end]
		build, payload, binding := strings.Index(phase, "forge build"), strings.Index(phase, "gencontracts --check"), strings.Index(phase, "stabi/generate.sh")
		if build < 0 || payload <= build || binding <= payload {
			t.Errorf("%s breaks full build/payload/binding dependency", name)
		}
	}
}
