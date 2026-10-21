//go:build linux || darwin

package miner

import (
	"os"
	"syscall"
)

// ownerProblem says when a folder isn't owned by this user.
func ownerProblem(dir string, info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if ok && int(stat.Uid) != os.Geteuid() {
		return displayStateDir() + " belongs to another user. Fix: sudo chown -R $(id -un) " + cliShellQuote(dir)
	}
	return ""
}
