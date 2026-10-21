//go:build windows

package clientauth

import (
	"os"

	"golang.org/x/sys/windows"
)

// openCredentialFile opens a credential for reading the way a Unix reader
// holds it: sharing delete as well, so a renewal, a quarantine or an explicit
// sign-in can rename over the file while it is read. os.Open does not share
// delete, and every such rename would fail for as long as a reader held it.
func openCredentialFile(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}
