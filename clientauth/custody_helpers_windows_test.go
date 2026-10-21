//go:build windows

package clientauth

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/urfoundation/sn/internal/wincustody"
)

// testFilePrivate is the custody's file rule as the platform states it: the
// file's owner and DACL pass the same check the store makes.
func testFilePrivate(path string, _ os.FileInfo) bool {
	file, err := openCredentialFile(path)
	if err != nil {
		return false
	}
	defer file.Close()
	_, err = wincustody.Privacy(windows.Handle(file.Fd()), false, false)
	return err == nil
}

// exposeTestFile lets Everyone read a file the custody must then refuse.
func exposeTestFile(t *testing.T, path string) {
	t.Helper()
	grantTestAccess(t, path, "S-1-1-0", windows.GENERIC_READ, windows.NO_INHERITANCE)
}

// grantTestAccess adds an allow entry for sid to path's DACL.
func grantTestAccess(t *testing.T, path string, sid string, mask windows.ACCESS_MASK, inheritance uint32) {
	t.Helper()
	trustee, err := windows.StringToSid(sid)
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	updated, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: mask,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(trustee),
		},
	}}, dacl)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, updated, nil); err != nil {
		t.Fatal(err)
	}
}

// testSymlink makes a symbolic link, or reports false where the account may
// not make one (no Developer Mode and no elevation).
func testSymlink(t *testing.T, target string, link string) bool {
	t.Helper()
	err := os.Symlink(target, link)
	if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
		t.Log("symbolic links need Developer Mode or elevation here; that case is not run")
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
}
