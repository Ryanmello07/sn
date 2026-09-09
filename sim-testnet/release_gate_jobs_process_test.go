//go:build linux

// Expected process disappearance is a failed identity lookup, not a shell
// diagnostic. The higher-level queue still decides whether absence is safe.
package main

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
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
printf 'live\n'`)
	if status != 0 || stdout != "live\n" || stderr != "" {
		t.Fatalf("live identity: status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}
