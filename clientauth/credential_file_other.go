//go:build !windows

package clientauth

import "os"

// openCredentialFile opens a credential for reading. A Unix rename never
// waits on a reader.
func openCredentialFile(path string) (*os.File, error) {
	return os.Open(path)
}
