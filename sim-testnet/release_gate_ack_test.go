package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The actual acknowledgement loop reads native FIFO descriptors. Its local
// clock advances only after a real empty readiness observation; an extra wait
// releases a real ack so the neutral implementation cannot strand the probe.
func releaseGateAcknowledgementControl(t *testing.T, mode string) {
	t.Helper()
	helper, err := filepath.Abs("../scripts/release-gate-child.py")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "python3", "-c", `import errno, importlib.util, os, select, sys
from pathlib import Path
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("owned_ack_probe", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
root = Path(sys.argv[2])
mode = sys.argv[3]
os.mkfifo(root / "ack", 0o600)
writer = os.open(root / "ack", os.O_RDWR | os.O_NONBLOCK)
stop = [143 if mode == "cancel-before" else 0]
state = {"now": 0, "waits": 0, "descriptor": None}
empty_waits = 2 if mode == "cancel-after" else 1

def observe_wait(readers, writers, errors, timeout):
    state["waits"] += 1
    state["descriptor"] = readers[0]
    if state["waits"] > empty_waits:
        # The live control deliberately acknowledges after the virtual wait.
        # A canceled neutral loop gets the same cooperative cleanup, but its
        # extra wait is retained as the primary failure evidence.
        os.write(writer, b"joined\n")
        return select.select(readers, writers, errors, 0)
    ready = select.select(readers, writers, errors, 0)
    assert not ready[0], "owned acknowledgement was unexpectedly readable"
    assert 0 < timeout <= 1, "acknowledgement wait escaped its remaining bound"
    state["now"] += 11
    if mode == "cancel-after":
        stop[0] = 143
    return ready

try:
    result = module.wait_for_acknowledgement(
        root, os.getppid(), 0, stop,
        clock=lambda: state["now"], wait=observe_wait,
    )
    try:
        os.fstat(state["descriptor"])
    except OSError as error:
        assert error.errno == errno.EBADF, "acknowledgement descriptor close changed error"
    else:
        raise AssertionError("acknowledgement reader was not closed")
    if mode == "live":
        assert result == 0 and state["waits"] == 2, "live acknowledgement acquired a cancellation deadline"
    else:
        assert result == 125 and state["waits"] == empty_waits, (
            "canceled acknowledgement waited beyond its owned deadline: "
            f"result={result} waits={state['waits']}"
        )
    print("acknowledgement control joined")
finally:
    os.close(writer)
`, helper, t.TempDir(), mode)
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "acknowledgement control joined") {
		t.Fatalf("%s acknowledgement control: %v\n%s", mode, err, output)
	}
}

// Cancellation before or during the ack phase has the same finite ownership.
func TestReleaseGateChildCanceledAcknowledgementHasBoundedJoin(t *testing.T) {
	for _, mode := range []string{"cancel-before", "cancel-after"} {
		releaseGateAcknowledgementControl(t, mode)
	}
}

// Normal completion may wait while the foreground owner performs other work.
func TestReleaseGateChildLiveAcknowledgementDoesNotExpire(t *testing.T) {
	releaseGateAcknowledgementControl(t, "live")
}

// An expired EXIT deadline is checked before another completion read. The
// real FIFO holds one invalid record so the old path returns, not hangs; its
// actual read is recorded independently of the refusal result.
func TestReleaseGateJobsExpiredExitDeadlinePrecedesCompletionRead(t *testing.T) {
	self := newReleaseGateJobsFixture(t, `
deadline=$((SECONDS + 1))
SECONDS="$deadline"
printf 'invalid-completion\n' >&"$release_gate_completion_fd"
completion_reads=0
read() {
  completion_reads=$((completion_reads + 1))
  builtin read "$@"
}
if release_gate_wait_one "$deadline"; then
  echo 'expired deadline accepted a completion wait' >&2
  exit 91
fi
if (( completion_reads != 0 )); then
  echo 'expired deadline entered completion read' >&2
  exit 92
fi
unset -f read
release_gate_complete
`)
	self.join(t, 0)
}

// Lose one real publication or close its actual ack descriptor. EXIT still
// joins the held peer, but must keep the failed slot and private services.
func TestReleaseGateJobsAcknowledgementLossRetainsFailureAndJoinsPeer(t *testing.T) {
	for _, loss := range []string{"completion", "ack"} {
		self := newReleaseGateJobsFixture(t, `
phase_bad() { printf 'bad-held\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"; }
phase_peer() { printf 'peer-held\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"; read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-b"; }
release_gate_start bad phase_bad
release_gate_start peer phase_peer
printf 'owners:%s:%s\n' "${release_gate_pids[0]}" "${release_gate_pids[1]}" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
printf 'root:%s\n' "$release_gate_root" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
read -r setup < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-c"
if [[ "`+loss+`" == completion ]]; then
  # Consume the genuine owner publication, without fabricating an accepted
  # completion or acknowledging its still-retained owner.
  IFS=' ' read -r -t 15 index status extra <&"$release_gate_completion_fd"
  [[ "$index" == 0 && "$status" == 0 && -z "$extra" ]]
else
  # The bad owner still emits its genuine completion. The parent cannot
  # acknowledge it through the descriptor that was actually admitted.
  bad_ack="${release_gate_ack_fds[0]}"
  exec {bad_ack}>&-
fi
printf 'loss-installed\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
read -r leave < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-c"
original_reap="$(declare -f release_gate_reap_job)"
eval "${original_reap/release_gate_reap_job/release_gate_reap_job_real}"
release_gate_reap_job() {
  release_gate_reap_job_real "$@" || return
  if [[ "$1" == 1 ]]; then
    printf '%s %s %s\n' "$release_gate_active" "${release_gate_pending[0]}" "${release_gate_pending[1]}" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/peer-joined"
  fi
}
fixture_exiting=0
original_exit="$(declare -f release_gate_jobs_exit)"
eval "${original_exit/release_gate_jobs_exit/release_gate_jobs_exit_real}"
release_gate_jobs_exit() { fixture_exiting=1; release_gate_jobs_exit_real "$@"; }
exit() {
  local status="${1:-0}"
  if (( fixture_exiting == 1 )); then
    printf '%s %s %s %s\n' "$release_gate_active" "${release_gate_pending[0]}" "${release_gate_pending[1]}" "$status" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/exit-state"
  fi
  builtin exit "$status"
}
exit 0
`)
		root, owners := "", ""
		seen := map[string]bool{}
		for range 4 {
			line := self.line(t, "events")
			if strings.HasPrefix(line, "owners:") {
				owners = strings.TrimPrefix(line, "owners:")
			} else if strings.HasPrefix(line, "root:") {
				root = strings.TrimPrefix(line, "root:")
			} else {
				seen[line] = true
			}
		}
		if !seen["bad-held"] || !seen["peer-held"] || filepath.Dir(root) != self.root {
			t.Fatalf("owned loss fixture admission=%v/%q", seen, root)
		}
		ownerPIDs := strings.Split(owners, ":")
		if len(ownerPIDs) != 2 || ownerPIDs[0] == ownerPIDs[1] {
			t.Fatalf("distinct actual owner identities=%q", owners)
		}
		for _, pid := range ownerPIDs {
			if value, err := strconv.Atoi(pid); err != nil || value <= 0 {
				t.Fatalf("invalid owner PID=%q: %v", pid, err)
			}
		}
		self.write(t, "control-a", "release")
		self.write(t, "control-c", "setup")
		if got := self.line(t, "events"); got != "loss-installed" {
			t.Fatalf("actual loss boundary=%q", got)
		}
		self.write(t, "control-c", "leave")
		self.join(t, 143)
		joined, err := os.ReadFile(filepath.Join(self.root, "peer-joined"))
		if err != nil || string(joined) != "1 1 0\n" {
			t.Fatalf("%s loss did not explicitly join the valid peer while preserving the failed slot: %q/%v\n%s", loss, joined, err, self.output.String())
		}
		state, err := os.ReadFile(filepath.Join(self.root, "exit-state"))
		if err != nil || string(state) != "1 1 0 143\n" {
			t.Fatalf("%s loss blessed an unproven completion: %q/%v\n%s", loss, state, err, self.output.String())
		}
		if _, err := os.Stat(filepath.Join(self.root, "cleaned")); !os.IsNotExist(err) {
			t.Fatalf("%s loss allowed unproven service cleanup: %v", loss, err)
		}
		log, err := os.ReadFile(filepath.Join(root, "logs", "bad.log"))
		if err != nil || !strings.Contains(string(log), "canceled owner acknowledgement deadline expired") {
			t.Fatalf("%s loss did not end at the actual child acknowledgement boundary: %v\n%s", loss, err, log)
		}
		for _, pid := range ownerPIDs {
			if _, err := os.Stat(filepath.Join("/proc", pid)); !os.IsNotExist(err) {
				t.Fatalf("%s loss left retained owner %s unjoined: %v", loss, pid, err)
			}
		}
	}
}
