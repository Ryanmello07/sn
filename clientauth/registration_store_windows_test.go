//go:build windows

package clientauth

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The descriptor the store creates: this user as owner, a protected DACL and
// no entry for anyone but this user, SYSTEM and Administrators.
func requireWindowsPrivateDescriptor(t *testing.T, path string, directory bool) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("%s inherits its DACL: %s", path, sd)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil || !owner.Equals(user.User.Sid) {
		t.Errorf("%s owner = %v, want %v", path, owner, user.User.Sid)
	}
	sddl := sd.String()
	for _, part := range strings.Split(sddl[strings.Index(sddl, "D:"):], "(")[1:] {
		fields := strings.Split(strings.TrimSuffix(part, ")"), ";")
		if len(fields) < 6 || fields[0] != "A" {
			t.Errorf("%s has an unexpected entry %q", path, part)
			continue
		}
		if who := fields[5]; who != "SY" && who != "BA" && who != user.User.Sid.String() {
			t.Errorf("%s grants %s: %s", path, who, sddl)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	_ = directory
}

func windowsCustodyPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "state", "jwt")
}

func TestWindowsCustodyCreatesPrivateDirectoryAndFiles(t *testing.T) {
	path := windowsCustodyPath(t)
	store, err := openRegistrationStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	requireWindowsPrivateDescriptor(t, filepath.Dir(path), true)
	if err := store.write("jwt.registration", []byte("request")); err != nil {
		t.Fatal(err)
	}
	requireWindowsPrivateDescriptor(t, path+".registration", false)
	requireWindowsPrivateDescriptor(t, path+".registration.lock", false)
	raw, err := store.read("jwt.registration")
	if err != nil || string(raw) != "request" {
		t.Fatalf("read = %q, %v", raw, err)
	}
	if err := store.write("jwt.registration", []byte("replaced")); err != nil {
		t.Fatal(err)
	}
	if raw, err := store.read("jwt.registration"); err != nil || string(raw) != "replaced" {
		t.Fatalf("replaced read = %q, %v", raw, err)
	}
	names, err := store.names(16)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(names)
	if want := []string{"jwt.registration", "jwt.registration.lock"}; !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v (no temporary may remain)", names, want)
	}
}

func TestWindowsCustodyRemoveLeavesNothing(t *testing.T) {
	path := windowsCustodyPath(t)
	store, err := openRegistrationStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	if err := store.write("jwt.lineage", []byte("lineage")); err != nil {
		t.Fatal(err)
	}
	if err := store.remove("jwt.lineage"); err != nil {
		t.Fatal(err)
	}
	if err := store.remove("jwt.lineage"); err != nil {
		t.Fatalf("removing a missing leaf: %v", err)
	}
	if _, err := store.read("jwt.lineage"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed leaf read: %v", err)
	}
	names, err := store.names(16)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names, []string{"jwt.registration.lock"}) {
		t.Fatalf("names after removal = %v", names)
	}
}

func TestWindowsCustodyOwnerLockIsExclusive(t *testing.T) {
	path := windowsCustodyPath(t)
	store, err := openRegistrationStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := openRegistrationStore(path); !errors.Is(err, errRegistrationOwnerActive) || second != nil {
		t.Fatalf("second owner: %v", err)
	}
	slot, err := openRegistrationStoreForOwner(filepath.Join(filepath.Dir(path), "provider.jwt"), store)
	if err != nil {
		t.Fatalf("a slot of the same directory: %v", err)
	}
	if err := errors.Join(slot.close(), store.close()); err != nil {
		t.Fatal(err)
	}
	again, err := openRegistrationStore(path)
	if err != nil {
		t.Fatalf("the lock outlived its owner: %v", err)
	}
	again.close()
}

func TestWindowsCustodyRefusesDirectoryOthersCanWrite(t *testing.T) {
	path := windowsCustodyPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	grantTestAccess(t, filepath.Dir(path), "S-1-5-32-545", 0x2 /* FILE_ADD_FILE */, windows.NO_INHERITANCE)
	if store, err := openRegistrationStore(path); err == nil || store != nil || !strings.Contains(err.Error(), "not writable by other users") {
		t.Fatalf("a directory Users can write was admitted: %v", err)
	}
}

func TestWindowsCustodyAdmitsDirectoryOthersCanOnlyRead(t *testing.T) {
	path := windowsCustodyPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	// the Unix rule refuses only group and other write
	grantTestAccess(t, filepath.Dir(path), "S-1-5-32-545", windows.FILE_LIST_DIRECTORY|windows.FILE_TRAVERSE|windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.SYNCHRONIZE, windows.NO_INHERITANCE)
	store, err := openRegistrationStore(path)
	if err != nil {
		t.Fatalf("a directory others can only read was refused: %v", err)
	}
	store.close()
}

func TestWindowsCustodyRefusesFileOthersCanRead(t *testing.T) {
	path := windowsCustodyPath(t)
	store, err := openRegistrationStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	if err := store.write("jwt.registration", []byte("request")); err != nil {
		t.Fatal(err)
	}
	exposeTestFile(t, path+".registration")
	if _, err := store.read("jwt.registration"); err == nil || !strings.Contains(err.Error(), "unsafe ownership") {
		t.Fatalf("a file Everyone can read was admitted: %v", err)
	}
}

func TestWindowsCustodyRefusesChangedDirectory(t *testing.T) {
	t.Run("acl", func(t *testing.T) {
		path := windowsCustodyPath(t)
		store, err := openRegistrationStore(path)
		if err != nil {
			t.Fatal(err)
		}
		defer store.close()
		grantTestAccess(t, filepath.Dir(path), "S-1-5-32-545", windows.FILE_LIST_DIRECTORY, windows.NO_INHERITANCE)
		if err := store.check(); err == nil {
			t.Fatal("a changed descriptor kept custody")
		}
	})
	t.Run("replaced", func(t *testing.T) {
		path := windowsCustodyPath(t)
		store, err := openRegistrationStore(path)
		if err != nil {
			t.Fatal(err)
		}
		defer store.close()
		dir := filepath.Dir(path)
		if err := os.Rename(dir, dir+".moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := store.check(); err == nil {
			t.Fatal("a replaced directory kept custody")
		}
		if err := store.write("jwt.registration", []byte("request")); err == nil {
			t.Fatal("a write reached a replaced directory")
		}
	})
}

func TestWindowsCustodyRefusesLinkInPath(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if !testSymlink(t, real, link) {
		return
	}
	if store, err := openRegistrationStore(filepath.Join(link, "jwt")); err == nil || store != nil {
		t.Fatal("custody followed a directory link")
	}
}

func TestWindowsCustodyPathsAreLocalAndPlain(t *testing.T) {
	for _, path := range []string{
		`\\server\share\state\jwt`,
		`\\?\C:\state\jwt`,
		`C:\state\jwt:stream`,
		`C:\state.\jwt`,
		`C:\NUL\jwt`,
		`state\jwt`,
	} {
		if store, err := openRegistrationStore(path); err == nil || store != nil {
			t.Errorf("%s was admitted", path)
		}
	}
}
