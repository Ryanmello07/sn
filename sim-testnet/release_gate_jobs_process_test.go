//go:build linux

// Expected process disappearance is a failed identity lookup, not a shell
// diagnostic. The higher-level queue still decides whether absence is safe.
package main

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// Sources the actual helper without initializing jobs or private services.
func releaseGateJobsProcessProbe(t *testing.T, body string) (string, string, int) {
	t.Helper()
	helper, err := filepath.Abs("../scripts/release-gate-jobs.sh")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "bash", "-c", "set -euo pipefail\nsource \"$1\"\n"+body, "release-process-probe-test", helper)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	status := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatal(err)
		}
		status = exitError.ExitCode()
	}
	return stdout.String(), stderr.String(), status
}

// This positive decimal exceeds Linux's signed process-id range, so missing
// proc state is forced without sleeps, polling, or a process-id reuse race.
func TestReleaseGateJobsProcessAbsenceIsQuietAndRefused(t *testing.T) {
	t.Parallel()
	stdout, stderr, status := releaseGateJobsProcessProbe(t, "release_gate_process 9223372036854775807")
	if status != 1 || stdout != "" || stderr != "" {
		t.Fatalf("missing identity: status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

// Malformed identifiers must not reach proc lookup or shell path expansion.
func TestReleaseGateJobsProcessRejectsInvalidIdentifiers(t *testing.T) {
	t.Parallel()
	stdout, stderr, status := releaseGateJobsProcessProbe(t, `for process_id in "" 0 -1 01 self ../self "1/stat"; do
  if release_gate_process "$process_id"; then exit 92; fi
done`)
	if status != 0 || stdout != "" || stderr != "" {
		t.Fatalf("invalid identity: status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

// Suppressing an expected open error must not suppress valid process identity.
// The shell stays alive through the lookup, so no lifecycle timing is assumed.
func TestReleaseGateJobsProcessPreservesLiveIdentity(t *testing.T) {
	t.Parallel()
	stdout, stderr, status := releaseGateJobsProcessProbe(t, `release_gate_process "$BASHPID"
[[ "$RELEASE_GATE_PROCESS_STATE" != Z ]]
[[ "$RELEASE_GATE_PROCESS_GROUP" =~ ^[1-9][0-9]*$ ]]
[[ "$RELEASE_GATE_PROCESS_SESSION" =~ ^[1-9][0-9]*$ ]]
[[ "$RELEASE_GATE_PROCESS_START" =~ ^[1-9][0-9]*$ ]]
fixture_identity="$RELEASE_GATE_PROCESS_GROUP:$RELEASE_GATE_PROCESS_SESSION:$RELEASE_GATE_PROCESS_START"
for fixture_separator in "" ":" ","; do
  IFS="$fixture_separator" release_gate_process "$BASHPID"
  [[ "$RELEASE_GATE_PROCESS_GROUP:$RELEASE_GATE_PROCESS_SESSION:$RELEASE_GATE_PROCESS_START" == "$fixture_identity" ]]
done
printf 'live\n'`)
	if status != 0 || stdout != "live\n" || stderr != "" {
		t.Fatalf("live identity: status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

// Deliver a real signal synchronously inside the actual timed-read invocation.
// Its temporary empty separator is still active when the trap probes identity.
func TestReleaseGateJobsProcessSignalTrapPreservesIdentity(t *testing.T) {
	t.Parallel()
	stdout, stderr, status := releaseGateJobsProcessProbe(t, `
release_gate_process "$BASHPID"
fixture_identity="$RELEASE_GATE_PROCESS_GROUP:$RELEASE_GATE_PROCESS_SESSION:$RELEASE_GATE_PROCESS_START"
trap '
  [[ -z "$IFS" ]] || exit 92
  release_gate_process "$BASHPID"
  [[ "$RELEASE_GATE_PROCESS_GROUP:$RELEASE_GATE_PROCESS_SESSION:$RELEASE_GATE_PROCESS_START" == "$fixture_identity" ]] || exit 93
  printf "trap identity\n"
  exit 143
' TERM
read() {
  local fixture_argument
  for fixture_argument in "$@"; do
    if [[ "$fixture_argument" == -t ]]; then
      [[ -z "$IFS" ]] || exit 94
      kill -TERM "$BASHPID"
      exit 95
    fi
  done
  builtin read "$@"
}
release_gate_completion_fd=0
release_gate_wait_one
exit 96
`)
	if status != 143 || stdout != "trap identity\n" || stderr != "" {
		t.Fatalf("signal identity: status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

// Replace only bytes returned after the real proc read. Short or malformed
// records must refuse without a nounset abort or partially published identity.
func TestReleaseGateJobsProcessRejectsMalformedIdentityRecords(t *testing.T) {
	t.Parallel()
	for _, record := range []string{
		"",
		"123 (fixture) S 1",
		"123 (fixture) invalid 1 2 3 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0 20",
		"123 (fixture) S 1 group 3 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0 20",
		"123 (fixture) S 1 2 session 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0 20",
		"123 (fixture) S 1 2 3 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0 start",
	} {
		stdout, stderr, status := releaseGateJobsProcessProbe(t, `
fixture_record=`+strconv.Quote(record)+`
read() {
  if [[ "$#" == 2 && "$1" == -r && "$2" == line ]]; then
    builtin read "$@" || return
    printf -v line '%s' "$fixture_record"
    return 0
  fi
  builtin read "$@"
}
RELEASE_GATE_PROCESS_STATE=unchanged
RELEASE_GATE_PROCESS_GROUP=unchanged
RELEASE_GATE_PROCESS_SESSION=unchanged
RELEASE_GATE_PROCESS_START=unchanged
if release_gate_process "$BASHPID"; then exit 92; fi
[[ "$RELEASE_GATE_PROCESS_STATE:$RELEASE_GATE_PROCESS_GROUP:$RELEASE_GATE_PROCESS_SESSION:$RELEASE_GATE_PROCESS_START" == unchanged:unchanged:unchanged:unchanged ]]
printf 'refused\n'
`)
		if status != 0 || stdout != "refused\n" || stderr != "" {
			t.Fatalf("malformed identity %q: status=%d stdout=%q stderr=%q", record, status, stdout, stderr)
		}
	}
}
