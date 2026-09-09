// Completion framing is tested through the actual queue and retained child
// owner; only the timed builtin read is interrupted at a deterministic byte.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Bash assigns partial data even when read times out. Consume real FIFO bytes
// into its first destination, then report that exact documented timeout class.
const releaseGateTimedFragmentReadFixture = `
fixture_partial_reads=0
read() {
  local fixture_argument fixture_value_argument=0 fixture_destination="" fixture_timed=0
  for fixture_argument in "$@"; do
    if (( fixture_value_argument == 1 )); then fixture_value_argument=0; continue; fi
    case "$fixture_argument" in
      -t) fixture_timed=1; fixture_value_argument=1 ;;
      -n|-N|-u|-d|-p|-a|-i) fixture_value_argument=1 ;;
      -*) ;;
      *) fixture_destination="$fixture_argument"; break ;;
    esac
  done
  if (( fixture_timed == 1 && fixture_partial_reads < fixture_fragment_parts )); then
    fixture_partial_reads=$((fixture_partial_reads + 1))
    [[ -n "$fixture_destination" ]] || return 2
    IFS= builtin read -r -N "$fixture_fragment_bytes" "$fixture_destination" || return 1
    return 142
  fi
  builtin read "$@"
}
`

// The pre-cleanup snapshot is the assertion boundary. Compensation safely
// reaps an old broken implementation, without changing that captured result.
func TestReleaseGateJobsRetainTimedCompletionFragments(t *testing.T) {
	for _, item := range []struct {
		bytes  int
		parts  int
		status int
	}{
		{bytes: 1, parts: 1, status: 0},
		{bytes: 1, parts: 1, status: 23},
		{bytes: 2, parts: 1, status: 23},
		{bytes: 3, parts: 1, status: 23},
		{bytes: 4, parts: 1, status: 23},
		{bytes: 1, parts: 2, status: 255},
	} {
		self := newReleaseGateJobsFixture(t, `
phase_finish() { return `+strconv.Itoa(item.status)+`; }
release_gate_start finish phase_finish
fixture_fragment_bytes=`+strconv.Itoa(item.bytes)+`
fixture_fragment_parts=`+strconv.Itoa(item.parts)+`
`+releaseGateTimedFragmentReadFixture+`
read_result=0
release_gate_wait_one || read_result=$?
printf '%s %s %s %s %s\n' "$read_result" "$release_gate_active" "${release_gate_pending[0]}" "$release_gate_result" "$fixture_partial_reads" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/fragment-state"
if (( read_result != 0 )); then
  unset -f read
  release_gate_owned_job 0
  printf 'joined\n' >&"${release_gate_ack_fds[0]}"
  release_gate_reap_job 0
fi
completion_result=0
release_gate_complete || completion_result=$?
printf '%s %s %s\n' "$release_gate_active" "$release_gate_services_cleaned" "$completion_result" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/fragment-cleanup"
exit 0
`)
		// Explicit exit zero must not mask the retained worker status.
		self.join(t, item.status)
		state, err := os.ReadFile(filepath.Join(self.root, "fragment-state"))
		expected := fmt.Sprintf("0 0 0 %d %d\n", item.status, item.parts)
		if err != nil || string(state) != expected {
			t.Fatalf("partial completion was discarded: bytes=%d parts=%d got=%q want=%q error=%v", item.bytes, item.parts, state, expected, err)
		}
		cleanup, err := os.ReadFile(filepath.Join(self.root, "fragment-cleanup"))
		if err != nil || string(cleanup) != fmt.Sprintf("0 1 %d\n", item.status) {
			t.Fatalf("fragment join lost actual worker status or cleanup: %q error=%v", cleanup, err)
		}
	}
}

// The first fragmented completion must not contaminate a second real owner,
// nor acknowledge that independently held owner before its normal completion.
func TestReleaseGateJobsKeepFragmentedOwnersIndependent(t *testing.T) {
	self := newReleaseGateJobsFixture(t, `
phase_a() { true; }
phase_b() { read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-b"; return 7; }
release_gate_start first phase_a
release_gate_start second phase_b
fixture_fragment_bytes=1
fixture_fragment_parts=1
`+releaseGateTimedFragmentReadFixture+`
read_result=0
release_gate_wait_one || read_result=$?
printf '%s %s %s %s\n' "$read_result" "$release_gate_active" "${release_gate_pending[0]}" "${release_gate_pending[1]}" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/first-fragment-state"
if (( read_result != 0 )); then
  unset -f read
  release_gate_owned_job 0
  printf 'joined\n' >&"${release_gate_ack_fds[0]}"
  release_gate_reap_job 0
fi
release_gate_owned_job 1
printf 'release\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-b"
completion_result=0
release_gate_complete || completion_result=$?
printf '%s %s %s %s\n' "$completion_result" "$release_gate_active" "$release_gate_services_cleaned" "${release_gate_pending[1]}" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/second-fragment-state"
exit 0
`)
	// The exit trap preserves the independently released owner's failure.
	self.join(t, 7)
	first, err := os.ReadFile(filepath.Join(self.root, "first-fragment-state"))
	if err != nil || string(first) != "0 1 0 1\n" {
		t.Fatalf("fragmented completion lost its exact independent owner: %q error=%v", first, err)
	}
	second, err := os.ReadFile(filepath.Join(self.root, "second-fragment-state"))
	if err != nil || string(second) != "7 0 1 0\n" {
		t.Fatalf("next completion lost real failure or final cleanup: %q error=%v", second, err)
	}
}

// Exact-size framing reaches ordinary admission; one byte more, incomplete
// input and malformed fields refuse before any acknowledgement or reap.
func TestReleaseGateJobsBoundCompletionFramingAndExplainRefusals(t *testing.T) {
	for _, item := range []struct {
		name    string
		data    string
		pending int
		message string
	}{
		{name: "exact limit", data: "12345678 255\n", pending: 1, message: "completion index is outside the admitted census"},
		{name: "one over", data: "123456789 255\n", pending: 1, message: "completion record exceeds 12 data bytes"},
		{name: "no delimiter at bound", data: "1234567890123", pending: 1, message: "completion record exceeds 12 data bytes"},
		{name: "unfinished", data: "0 ", pending: 1, message: "completion read ended without a complete record"},
		{name: "empty input", data: "", pending: 1, message: "completion read ended without a complete record"},
		{name: "empty line", data: "\n", pending: 1, message: "completion has malformed index/status fields"},
		{name: "extra field", data: "0 0 extra\n", pending: 1, message: "completion has malformed index/status fields"},
		{name: "noncanonical index", data: "00 0\n", pending: 1, message: "completion has malformed index/status fields"},
		{name: "noncanonical status", data: "0 000\n", pending: 1, message: "completion has malformed index/status fields"},
		{name: "negative status", data: "0 -1\n", pending: 1, message: "completion has malformed index/status fields"},
		{name: "status one over", data: "0 256\n", pending: 1, message: "completion status is outside the process exit range"},
		{name: "inactive slot", data: "0 0\n", pending: 0, message: "completion names an inactive or already joined owner"},
		{name: "foreign owner", data: "0 0\n", pending: 1, message: "completion owner identity is unavailable or changed"},
	} {
		self := newReleaseGateJobsFixture(t, `
printf 'input-ready\n' > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/events"
read -r release < "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/control-a"
original_completion_fd="$release_gate_completion_fd"
exec {fixture_input}< "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/completion-input"
release_gate_completion_fd="$fixture_input"
# No worker is launched. This impossible Linux pid is an unowned declaration;
# even a syntactically valid completion must refuse its missing identity.
release_gate_pids=(999999999)
release_gate_starts=(0)
release_gate_pending=(`+strconv.Itoa(item.pending)+`)
release_gate_ack_fds=()
read_result=0
release_gate_wait_one || read_result=$?
printf '%s %s %s\n' "$read_result" "$release_gate_active" "${release_gate_pending[0]}" > "$RELEASE_GATE_QUEUE_FIXTURE_ROOT/refusal-state"
release_gate_completion_fd="$original_completion_fd"
exec {fixture_input}<&-
release_gate_pids=()
release_gate_starts=()
release_gate_pending=()
release_gate_complete
`)
		if got := self.line(t, "events"); got != "input-ready" {
			t.Fatalf("completion framing fixture admission=%q", got)
		}
		if err := os.WriteFile(filepath.Join(self.root, "completion-input"), []byte(item.data), 0o600); err != nil {
			t.Fatal(err)
		}
		self.write(t, "control-a", "release")
		self.join(t, 0)
		state, err := os.ReadFile(filepath.Join(self.root, "refusal-state"))
		if err != nil || string(state) != fmt.Sprintf("1 0 %d\n", item.pending) {
			t.Fatalf("%s completion mutated an unowned slot: state=%q error=%v", item.name, state, err)
		}
		if !strings.Contains(self.output.String(), item.message) {
			t.Fatalf("%s completion refusal lost its diagnostic %q:\n%s", item.name, item.message, self.output.String())
		}
	}
}
