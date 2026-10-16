//go:build linux

// The burst cooldown test's child under a pinned supervisor stop. The barrier
// that holds its shell stopped is a waitid stop report, which is Linux only.
package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// A supervisor stop lands while the cooldown child's shell waits on its poll
// helper, and the shell reaps that helper before it exits. In run 5 the shell
// died of the TERM first, and the release gate's subreaper adopted the helper.
// The state is a FIFO here, so the helper is a grep held in its read, and the
// group is stopped before the TERM, so neither process acts on it until both
// have it. The private owner adopts anything the shell leaves behind.
func TestSupervisorStoppedCooldownChildJoinsItsPollHelper(t *testing.T) {
	runSupervisorDescendantOwnerTest(t, 0, 25*time.Second, assertSupervisorStoppedCooldownChildJoinsItsPollHelperTest)
}

func assertSupervisorStoppedCooldownChildJoinsItsPollHelperTest(t *testing.T) {
	dir := t.TempDir()
	counterPath := filepath.Join(dir, "child.counter")
	statePath := filepath.Join(dir, "supervisor.state.fifo")
	// Two earlier attempts make this the third, the one the cooldown test stops.
	if err := os.WriteFile(counterPath, []byte("2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(statePath, 0o600); err != nil {
		t.Fatal(err)
	}
	spec := supervisorCooldownTestChild(dir, counterPath, statePath)
	command, exited, err := startSpecWithExit(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	joined := false
	t.Cleanup(func() {
		if !joined {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			<-exited
		}
	})
	identity, err := observeStartedSupervisedProcessIdentity(context.Background(), command.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	// Opening the state to write returns once the helper has opened it to
	// read, so from here the shell waits on a live grep.
	type openedState struct {
		file *os.File
		err  error
	}
	opened := make(chan openedState, 1)
	go func() {
		file, err := os.OpenFile(statePath, os.O_WRONLY, 0)
		opened <- openedState{file: file, err: err}
	}()
	select {
	case state := <-opened:
		if state.err != nil {
			t.Fatal(state.err)
		}
		defer state.file.Close()
	case err := <-exited:
		t.Fatalf("cooldown child exited before its poll helper opened the state: %v", err)
	}
	// Stop the group, and wait until the shell's stop is reported, so the
	// supervisor's TERM is pending in both processes before either can run.
	if err := syscall.Kill(-identity.ProcessGroupID, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	var stopped unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, command.Process.Pid, &stopped, unix.WSTOPPED|unix.WNOWAIT, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if !signalSupervisedCommandWithObserver(supervisedCommand{spec: spec, cmd: command, identity: identity}, syscall.SIGTERM, observeSupervisedProcessIdentity, syscall.Kill) {
		t.Fatal("the supervisor's stop did not signal the cooldown child's group")
	}
	if err := syscall.Kill(-identity.ProcessGroupID, syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	// The trap exits 143 once the shell has reaped the helper. A shell that
	// dies of the TERM instead leaves the helper to the private owner.
	err = <-exited
	joined = true
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 143 {
		t.Fatalf("stopped cooldown child exit = %v, want 143 from its TERM trap", err)
	}
}
