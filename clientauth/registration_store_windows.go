//go:build windows

package clientauth

// The Windows owner of registration custody keeps the Unix store's guarantees
// with Windows primitives (internal/wincustody). One handle to the private
// directory, opened one path component at a time without following reparse
// points, anchors every leaf open, publication and removal. Privacy is the
// directory's and each file's security descriptor in place of the owner uid
// and mode bits, and an exclusive LockFileEx lock on
// <client>.registration.lock replaces flock. A rename is durable once the
// renamed file is flushed; a removal renames the leaf aside to a
// .registration-<random> name that way before deleting it, so a crash leaves
// only a name like an interrupted write's temporary.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	"github.com/urfoundation/sn/internal/wincustody"
)

const maximumRegistrationBytes = 32 * 1024

// Leaf open modes shared with the Unix store's callers.
const (
	registrationOpenRead   = wincustody.OpenRead
	registrationOpenLock   = wincustody.OpenLock
	registrationOpenCreate = wincustody.OpenCreate
)

type registrationAnchor struct {
	identity wincustody.Identity
	// owner and DACL in SDDL; a change while custody is held is refused
	security string
}

type registrationStore struct {
	directory *os.File
	lock      *os.File
	path      string
	client    string
	anchor    registrationAnchor
}

// Refuse namespace redirection before opening secrets or a durable request.
func openRegistrationStore(clientPath string) (_ *registrationStore, returnErr error) {
	if !filepath.IsAbs(clientPath) || filepath.Clean(clientPath) != clientPath || filepath.Dir(clientPath) == clientPath {
		return nil, errors.New("registration credential path is not canonical absolute")
	}
	dir := filepath.Dir(clientPath)
	if _, _, err := wincustody.SplitPath(dir); err != nil {
		return nil, err
	}
	if err := wincustody.MakeDirectory(dir); err != nil {
		return nil, err
	}
	handle, err := wincustody.OpenPath(dir)
	if err != nil {
		return nil, err
	}
	self := &registrationStore{directory: os.NewFile(uintptr(handle), dir), path: dir, client: filepath.Base(clientPath)}
	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, self.close())
		}
	}()
	self.anchor.identity, err = wincustody.HandleIdentity(handle)
	if err != nil {
		return nil, err
	}
	self.anchor.security, err = wincustody.Privacy(handle, true, false)
	if errors.Is(err, wincustody.ErrDirectoryNotPrivate) {
		return nil, errors.New("registration parent must be owned and not writable by other users")
	}
	if err != nil {
		return nil, err
	}
	if err := self.lockOwner(); err != nil {
		return nil, err
	}
	return self, nil
}

// Provider slots borrow the key owner's physical directory, never a fresh
// pathname selection. The handle remains tied to that directory across reads,
// writes and retries; check refuses a renamed/replaced visible namespace.
func openRegistrationStoreForOwner(clientPath string, owner *registrationStore) (_ *registrationStore, returnErr error) {
	if owner == nil || owner.directory == nil || owner.lock == nil || !filepath.IsAbs(clientPath) || filepath.Clean(clientPath) != clientPath || filepath.Dir(clientPath) != owner.path {
		return nil, errors.New("provider client custody differs from its retained directory owner")
	}
	if err := owner.check(); err != nil {
		return nil, err
	}
	// an empty name relative to a handle reopens that same directory
	handle, err := wincustody.OpenDirectory(windows.Handle(owner.directory.Fd()), "")
	if err != nil {
		return nil, err
	}
	self := &registrationStore{directory: os.NewFile(uintptr(handle), owner.path), path: owner.path, client: filepath.Base(clientPath), anchor: owner.anchor}
	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, self.close())
		}
	}()
	if err := self.check(); err != nil {
		return nil, err
	}
	if err := self.lockOwner(); err != nil {
		return nil, err
	}
	if err := self.check(); err != nil {
		return nil, err
	}
	return self, nil
}

func (self *registrationStore) lockOwner() error {
	var err error
	self.lock, err = self.open(self.client+".registration.lock", registrationOpenLock)
	if err != nil {
		return err
	}
	err = wincustody.Lock(self.lock)
	if errors.Is(err, wincustody.ErrLocked) {
		return errRegistrationOwnerActive
	}
	return err
}

// All leaf opens are relative, never follow a reparse point, and then check
// handle metadata and the file's descriptor.
func (self *registrationStore) open(name string, mode int) (*os.File, error) {
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." || !wincustody.ValidName(name) {
		return nil, errors.New("registration leaf is invalid")
	}
	file, err := wincustody.OpenLeaf(windows.Handle(self.directory.Fd()), self.path, name, mode, maximumRegistrationBytes)
	if errors.Is(err, wincustody.ErrFileNotPrivate) {
		return nil, errors.New("registration file has unsafe ownership, mode, links or size")
	}
	return file, err
}

// Reopen only to authenticate namespace identity; operations keep using the
// original handle even if a path is maliciously changed between checks.
func (self *registrationStore) check() error {
	info, err := os.Lstat(self.path)
	if err != nil {
		return err
	}
	owned, err := self.directory.Stat()
	if err != nil {
		return err
	}
	handle := windows.Handle(self.directory.Fd())
	identity, err := wincustody.HandleIdentity(handle)
	if err != nil {
		return err
	}
	security, err := wincustody.Descriptor(handle)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !os.SameFile(info, owned) || identity != self.anchor.identity || security != self.anchor.security {
		return errors.New("registration custody directory identity changed")
	}
	return nil
}

func (self *registrationStore) read(name string) ([]byte, error) {
	if err := self.check(); err != nil {
		return nil, err
	}
	file, err := self.open(name, registrationOpenRead)
	if err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maximumRegistrationBytes+1))
	closeErr := file.Close()
	if len(raw) > maximumRegistrationBytes {
		readErr = errors.Join(readErr, errors.New("registration data exceeds its finite bound"))
	}
	if err := errors.Join(readErr, closeErr, self.check()); err != nil {
		return nil, err
	}
	return raw, nil
}

// A single bounded census detects old identity files before first-key creation.
func (self *registrationStore) names(maximum int) ([]string, error) {
	if maximum < 1 || maximum > 4096 {
		return nil, errors.New("registration directory census bound differs")
	}
	if err := self.check(); err != nil {
		return nil, err
	}
	if _, err := self.directory.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	names, err := self.directory.Readdirnames(maximum + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > maximum {
		return nil, errors.New("registration directory census exceeds its bound")
	}
	return names, self.check()
}

// Deletion uses the same owned directory as publication. A missing leaf is
// already absent; the name is durably gone before the caller proceeds.
func (self *registrationStore) remove(name string) error {
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." || !wincustody.ValidName(name) {
		return errors.New("registration removal leaf is invalid")
	}
	if err := self.check(); err != nil {
		return err
	}
	aside, err := registrationTemporaryName()
	if err != nil {
		return err
	}
	if err := wincustody.Remove(windows.Handle(self.directory.Fd()), self.path, name, aside); err != nil {
		return err
	}
	return self.check()
}

// Rename publishes complete bytes; a failed flush remains an error and never
// licenses a second operation or a credential before durable handoff.
func (self *registrationStore) write(name string, raw []byte) (returnErr error) {
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." || !wincustody.ValidName(name) || len(raw) == 0 || len(raw) > maximumRegistrationBytes {
		return errors.New("registration write exceeds its finite bound")
	}
	if err := self.check(); err != nil {
		return err
	}
	temporary, err := registrationTemporaryName()
	if err != nil {
		return err
	}
	file, err := self.open(temporary, registrationOpenCreate)
	if err != nil {
		return err
	}
	renamed := false
	defer func() {
		if returnErr != nil && !renamed {
			_ = wincustody.Delete(windows.Handle(self.directory.Fd()), self.path, temporary)
		}
	}()
	_, writeErr := io.Copy(file, bytes.NewReader(raw))
	if err := errors.Join(writeErr, file.Sync(), file.Close(), self.check()); err != nil {
		return err
	}
	if err := self.renameLeaf(temporary, name); err != nil {
		return err
	}
	renamed = true
	return self.check()
}

func registrationTemporaryName() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return ".registration-" + hex.EncodeToString(random[:]), nil
}

// renameLeaf replaces to with from inside the owned directory and flushes the
// renamed file, which is how Windows makes a rename durable.
func (self *registrationStore) renameLeaf(from string, to string) error {
	return wincustody.Rename(windows.Handle(self.directory.Fd()), self.path, from, to)
}

// The Unix store syncs its directory after a rename; renameLeaf already
// flushed the renamed file, and Windows has no directory flush.
func (self *registrationStore) syncDirectory() error {
	return nil
}

func registrationModTimeNano(file *os.File) (int64, error) {
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	return info.ModTime().UnixNano(), nil
}

// Releasing the lock and directory is part of the caller's completed result.
func (self *registrationStore) close() error {
	if self == nil {
		return nil
	}
	var err error
	if self.lock != nil {
		err = self.lock.Close()
		self.lock = nil
	}
	if self.directory != nil {
		err = errors.Join(err, self.directory.Close())
		self.directory = nil
	}
	return err
}
