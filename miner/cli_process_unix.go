//go:build linux || darwin

package miner

import (
	"errors"
	"syscall"
)

// minerProcessAlive is true when a process with this pid exists, even if it
// belongs to another user.
func minerProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
