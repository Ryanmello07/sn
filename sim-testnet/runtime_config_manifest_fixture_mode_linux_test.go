//go:build linux

// The fixture must produce the real renderer's exact inventory under both
// restrictive and ordinary host masks. Only private child processes change it.
package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// Exercise the actual shared constructor, including its own manifest writer
// and verifier. A child mask makes the original permission failure certain.
func TestRuntimeConfigManifestFixtureModesIgnoreUmask(t *testing.T) {
	const ownerKey = "SN_TEST_RUNTIME_FIXTURE_MODE_OWNER"
	const maskKey = "SN_TEST_RUNTIME_FIXTURE_MODE_MASK"
	if os.Getenv(ownerKey) != t.Name() {
		for _, mask := range []string{"077", "022"} {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.count=1", "-test.timeout=45s")
			command.Env = append(os.Environ(), ownerKey+"="+t.Name(), maskKey+"="+mask)
			command.WaitDelay = 5 * time.Second
			output, err := command.CombinedOutput()
			cancel()
			if err != nil {
				t.Fatalf("fixture umask %s child failed: %v\n%s", mask, err, output)
			}
		}
		return
	}
	mask, err := strconv.ParseUint(os.Getenv(maskKey), 8, 9)
	if err != nil || mask != 0o077 && mask != 0o022 {
		t.Fatal("private fixture child has no exact declared mask")
	}
	previous := syscall.Umask(int(mask))
	defer syscall.Umask(previous)
	cfg, stateDir := runtimeConfigManifestFixtureForOperators(t, 2)
	expected, err := expectedRuntimeConfigFiles(cfg, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	for relative, mode := range expected {
		info, err := os.Lstat(filepath.Join(stateDir, filepath.FromSlash(relative)))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != mode.Perm() {
			t.Fatalf("fixture umask %03o did not retain exact %s mode %04o: %v", mask, relative, mode.Perm(), err)
		}
	}
	if _, err := verifyRuntimeConfigManifest(cfg, stateDir); err != nil {
		t.Fatalf("exact generated fixture failed authentication: %v", err)
	}
}
