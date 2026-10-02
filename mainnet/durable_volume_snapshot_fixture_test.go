//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/urfoundation/sn/internal/durablefixture"
)

// Existing fixture composition can share an already prepared marker. This is
// explicit test enrollment before an owner opens; it never runs on a restart.
func prepareMainnetSnapshotTest(t *testing.T, path, kind string, maximum int) {
	t.Helper()
	if _, err := os.Lstat(path + ".lock"); err == nil {
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	name := filepath.Base(path)
	durablefixture.ProvisionSnapshot(t, filepath.Dir(path), kind, name, int64(maximum), name+".lock", map[string][]byte{name + ".lock": nil})
}
