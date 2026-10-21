//go:build linux || darwin

package clientauth

import (
	"os"
	"testing"
)

// testFilePrivate is the custody's file rule as the platform states it.
func testFilePrivate(_ string, info os.FileInfo) bool {
	return info.Mode().Perm() == 0600
}

// exposeTestFile lets other users read a file the custody must then refuse.
func exposeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
}

// testSymlink makes a symbolic link, or reports false where the account may
// not make one.
func testSymlink(t *testing.T, target string, link string) bool {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return true
}
