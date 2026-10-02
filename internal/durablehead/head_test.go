//go:build linux

// These controls use real files, xattrs, flock, rename and sync. Only kernel
// volume facts are synthetic; crash tests join a distinct process.
package durablehead

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/urfoundation/sn/internal/durablefixture"
	"github.com/urfoundation/sn/internal/durablepath"
	"github.com/urnetwork/connect/durablevolume"
	"golang.org/x/sys/unix"
)

var testSpec = Spec{Kind: "synthetic-history", Name: "history.json", MaximumBytes: 4096, LockName: "history.lock", AuxiliaryNames: []string{"history.lock"}}

type fixture struct {
	root      string
	volume    *durablefixture.Fixture
	directory *durablepath.Directory
	lock      *os.File
	owner     *Owner
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "owner")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	volume := durablefixture.New(t, t.Context(), root)
	durablefixture.ProvisionSnapshot(t, root, testSpec.Kind, testSpec.Name, testSpec.MaximumBytes, testSpec.LockName, map[string][]byte{testSpec.LockName: []byte("prepared\n")})
	self := &fixture{root: root, volume: volume}
	if err := self.open(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := self.close(); err != nil {
			t.Error(err)
		}
	})
	return self
}

func (self *fixture) open(reconcile bool) error {
	directory, err := durablepath.Open(self.volume.Context, self.root, durablevolume.ReadWrite, false)
	if err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(self.root, testSpec.LockName), os.O_RDWR, 0)
	if err != nil {
		directory.Close()
		return err
	}
	open := Open
	if reconcile {
		open = Reconcile
	}
	owner, err := open(self.volume.Context, directory, lock, testSpec)
	if err != nil {
		return errors.Join(err, lock.Close(), directory.Close())
	}
	self.directory, self.lock, self.owner = directory, lock, owner
	return nil
}

func (self *fixture) close() error {
	var errs []error
	if self.owner != nil {
		errs = append(errs, self.owner.Close())
		self.owner = nil
	}
	if self.lock != nil {
		errs = append(errs, self.lock.Close())
		self.lock = nil
	}
	if self.directory != nil {
		errs = append(errs, self.directory.Close())
		self.directory = nil
	}
	return errors.Join(errs...)
}

// Empty is explicit, and deleting a retained snapshot never returns to it.
func TestSnapshotHeadRequiresProvisionedAuthorityAndRetainedMember(t *testing.T) {
	self := newFixture(t)
	if raw, present, err := self.owner.Read(); err != nil || present || len(raw) != 0 {
		t.Fatal("explicit fresh state", present, err)
	}
	raw := []byte("original-signed-completed-history")
	if err := self.owner.Publish(raw, nil); err != nil {
		t.Fatal(err)
	}
	if err := self.close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(self.root, testSpec.Name)
	if err := os.Rename(path, path+".retained"); err != nil {
		t.Fatal(err)
	}
	if err := self.open(false); !errors.Is(err, durablevolume.ErrIdentity) {
		t.Fatal("missing committed history admitted", err)
	}
	if err := self.open(true); !errors.Is(err, durablevolume.ErrIdentity) {
		t.Fatal("reconciliation invented missing history", err)
	}
	if err := os.Rename(path+".retained", path); err != nil {
		t.Fatal(err)
	}
	if err := unix.Removexattr(self.root, Attribute(testSpec.Kind, testSpec.Name)); err != nil {
		t.Fatal(err)
	}
	if err := self.open(false); !errors.Is(err, durablevolume.ErrIdentity) {
		t.Fatal("absent authority enrolled", err)
	}
	if _, err := unix.Getxattr(self.root, Attribute(testSpec.Kind, testSpec.Name), nil); !errors.Is(err, unix.ENODATA) {
		t.Fatal("checkpoint recreated", err)
	}
}

// Pressure and unavailable observations refuse one operation, not the owner.
func TestSnapshotHeadSameOwnerReadAndPressureRecovery(t *testing.T) {
	self := newFixture(t)
	first := []byte("retained-head-one")
	if err := self.owner.Publish(first, nil); err != nil {
		t.Fatal(err)
	}
	for _, readOnly := range []bool{false, true} {
		self.volume.Host.SetReadOnly(readOnly)
		self.volume.Host.SetReserve(0, 0)
		if raw, present, err := self.owner.Read(); err != nil || !present || !bytes.Equal(raw, first) {
			t.Fatal("read-only custody lost", readOnly, err)
		}
		if err := self.owner.Publish([]byte("rejected"), nil); !errors.Is(err, durablevolume.ErrUnavailable) || errors.Is(err, ErrUncertain) || errors.Is(err, durablevolume.ErrIdentity) {
			t.Fatal("pre-admission pressure poisoned owner", err)
		}
	}
	self.volume.Host.SetReadOnly(false)
	self.volume.Host.SetReserve(1<<20, 1024)
	self.owner.at = func(stage string) error {
		if stage == "checkpoint-observe" {
			return unix.EIO
		}
		return nil
	}
	if err := self.owner.CheckWrite(); !errors.Is(err, durablevolume.ErrUnavailable) || !errors.Is(err, unix.EIO) || errors.Is(err, durablevolume.ErrIdentity) {
		t.Fatal("observation failure invented loss", err)
	}
	self.owner.at = nil
	if err := self.owner.Publish([]byte("retained-head-two"), nil); err != nil {
		t.Fatal("same owner failed to resume", err)
	}
}

// Path observation errors never imply that a successfully retained inode was
// replaced. Closed borrowed descriptors are caller errors, not volume loss.
func TestSnapshotHeadNamedObservationTaxonomy(t *testing.T) {
	self := newFixture(t)
	if err := self.owner.Publish([]byte("retained"), nil); err != nil {
		t.Fatal(err)
	}
	for _, cause := range []error{unix.EIO, unix.EMFILE} {
		self.owner.at = func(stage string) error {
			if stage == "named-observe:"+testSpec.Name {
				return cause
			}
			return nil
		}
		if err := self.owner.Check(); !errors.Is(err, durablevolume.ErrUnavailable) || !errors.Is(err, cause) || errors.Is(err, durablevolume.ErrIdentity) {
			t.Fatal("unavailable named observation poisoned identity", err)
		}
		self.owner.at = nil
		if err := self.owner.Check(); err != nil {
			t.Fatal("same retained owner did not recover", err)
		}
	}
	if err := self.lock.Close(); err != nil {
		t.Fatal(err)
	}
	self.lock = nil
	if err := self.owner.Check(); err == nil || errors.Is(err, durablevolume.ErrIdentity) {
		t.Fatal("invalid caller descriptor invented volume loss", err)
	}
}

// Another descriptor/process cannot acquire this exact declared writer lock.
func TestSnapshotHeadExclusiveWriterAndCapacity(t *testing.T) {
	self := newFixture(t)
	other, err := os.OpenFile(filepath.Join(self.root, testSpec.LockName), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if owner, err := Open(self.volume.Context, self.directory, other, testSpec); owner != nil || !errors.Is(err, durablevolume.ErrBusy) {
		t.Fatal("concurrent writer admitted", err)
	}
	if err := self.owner.Publish(bytes.Repeat([]byte{'x'}, int(testSpec.MaximumBytes)+1), nil); !errors.Is(err, durablevolume.ErrUnavailable) || errors.Is(err, ErrUncertain) {
		t.Fatal("oversize publication touched custody", err)
	}
	if err := self.owner.Publish([]byte("small"), nil); err != nil {
		t.Fatal(err)
	}
	if used, maximum, err := self.owner.Capacity(); err != nil || used != 5 || maximum != testSpec.MaximumBytes {
		t.Fatal("declared capacity hidden", used, maximum, err)
	}
}

// Private preprovisioned markers can change their application grammar without
// changing inode. Missing/replaced markers cannot be created by this API.
func TestSnapshotHeadAuxiliaryAdmission(t *testing.T) {
	self := newFixture(t)
	if err := self.owner.WithAuxiliary(testSpec.LockName, true, func(file *os.File) error { _, err := file.WriteAt([]byte("complete\n"), 0); return err }); err != nil {
		t.Fatal(err)
	}
	if err := self.owner.Publish([]byte("completed-history"), nil); err != nil {
		t.Fatal(err)
	}
	if err := self.owner.WithAuxiliary("unknown", true, func(*os.File) error { t.Fatal("undeclared callback called"); return nil }); err == nil {
		t.Fatal("undeclared auxiliary admitted")
	}
	path := filepath.Join(self.root, testSpec.LockName)
	if err := os.Rename(path, path+".retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("complete\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := self.owner.Check(); !errors.Is(err, durablevolume.ErrIdentity) {
		t.Fatal("replacement marker admitted", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".retained", path); err != nil {
		t.Fatal(err)
	}
	if err := self.owner.Check(); !errors.Is(err, durablevolume.ErrIdentity) {
		t.Fatal("invalid generation resurrected", err)
	}
}

// Each real publication cut either keeps exact recoverable next bytes or
// refuses incomplete/unanchored data. No recovery signs or reconstructs it.
func TestSnapshotHeadLostAcknowledgementRecovery(t *testing.T) {
	for _, stage := range []string{"temporary-created", "pending-synced", "temporary-written", "temporary-synced", "snapshot-renamed", "snapshot-directory-synced", "predecessor-removed", "committed-synced"} {
		t.Run(stage, func(t *testing.T) {
			self := newFixture(t)
			if err := self.owner.Publish([]byte("original"), nil); err != nil {
				t.Fatal(err)
			}
			next := []byte("exact-next-original-signed-bytes")
			self.owner.at = func(point string) error {
				if point == stage {
					return unix.EIO
				}
				return nil
			}
			if err := self.owner.Publish(next, nil); !errors.Is(err, ErrUncertain) {
				t.Fatal("lost acknowledgement not retained", err)
			}
			if err := self.owner.Publish([]byte("must-not-replace"), nil); !errors.Is(err, ErrUncertain) {
				t.Fatal("uncertain owner resumed", err)
			}
			if err := self.close(); err != nil {
				t.Fatal(err)
			}
			err := self.open(true)
			if stage == "temporary-created" || stage == "pending-synced" {
				if err == nil {
					t.Fatal("incomplete next bytes recovered")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if raw, present, err := self.owner.Read(); err != nil || !present || !bytes.Equal(raw, next) {
				t.Fatal("recovery changed retained next bytes", string(raw), err)
			}
			if err := self.owner.Publish([]byte("later"), nil); err != nil {
				t.Fatal("bounded recovery could not continue", err)
			}
		})
	}
}

// Complete pending bytes do not authorize collisions, extra temporaries,
// partial writes or hash mismatches. Every negative retains those bytes.
func TestSnapshotHeadRecoveryRejectsUnknownBytes(t *testing.T) {
	for _, kind := range []string{"extra", "collision", "partial", "hash"} {
		t.Run(kind, func(t *testing.T) {
			self := newFixture(t)
			self.owner.at = func(stage string) error {
				if stage == "temporary-synced" {
					return unix.EIO
				}
				return nil
			}
			if err := self.owner.Publish([]byte("exact-original-next"), nil); !errors.Is(err, ErrUncertain) {
				t.Fatal(err)
			}
			pending := self.owner.checkpoint.Pending
			path := filepath.Join(self.root, pending.Temporary)
			switch kind {
			case "extra":
				if err := os.WriteFile(filepath.Join(self.root, self.owner.prefix()+"00000000000000000000000000000000"), []byte("unknown"), 0600); err != nil {
					t.Fatal(err)
				}
			case "collision":
				if err := os.WriteFile(filepath.Join(self.root, testSpec.Name), []byte("unknown"), 0600); err != nil {
					t.Fatal(err)
				}
			case "partial":
				if err := os.Truncate(path, 2); err != nil {
					t.Fatal(err)
				}
			case "hash":
				if err := os.WriteFile(path, []byte("other-original-next"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := self.close(); err != nil {
				t.Fatal(err)
			}
			if err := self.open(true); err == nil {
				t.Fatal("unknown bytes admitted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("negative recovery destroyed evidence", err)
			}
		})
	}
}

// A barrier at actual descriptor read proves named leaf replacement is sticky;
// a separately owned root continues while only the affected owner stops.
func TestSnapshotHeadConcurrentLeafLossIsScoped(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	if err := a.owner.Publish([]byte("a-history"), nil); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	a.owner.at = func(stage string) error {
		if stage == "read-opened" {
			close(entered)
			<-release
		}
		return nil
	}
	result := make(chan error, 1)
	go func() {
		raw, _, err := a.owner.Read()
		if len(raw) != 0 {
			err = errors.Join(err, errors.New("lost leaf returned bytes"))
		}
		result <- err
	}()
	<-entered
	path := filepath.Join(a.root, testSpec.Name)
	if err := os.Rename(path, path+".retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("a-history"), 0600); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-result; !errors.Is(err, durablevolume.ErrIdentity) {
		t.Fatal("replaced descriptor read acknowledged", err)
	}
	if err := b.owner.Publish([]byte("b-history"), nil); err != nil {
		t.Fatal("unrelated owner stopped", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".retained", path); err != nil {
		t.Fatal(err)
	}
	if err := a.owner.CheckWrite(); !errors.Is(err, durablevolume.ErrIdentity) {
		t.Fatal("old owner resurrected", err)
	}
}

// Close/check serialization protects state while the caller retains its own
// descriptors. A canceled context is refused before any attribute observation.
func TestSnapshotHeadCloseAndCancellation(t *testing.T) {
	self := newFixture(t)
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			for range 8 {
				_ = self.owner.Check()
			}
		})
	}
	if err := self.owner.Close(); err != nil {
		t.Fatal(err)
	}
	group.Wait()
	if err := self.owner.Check(); err == nil {
		t.Fatal("closed owner admitted")
	}
	ctx, cancel := context.WithCancel(self.volume.Context)
	cancel()
	if owner, err := Open(ctx, self.directory, self.lock, testSpec); !errors.Is(err, context.Canceled) || owner != nil {
		t.Fatal("canceled admission proceeded", err)
	}
}

// No timing assumption is used: a child exits at a named real-I/O boundary.
func TestSnapshotHeadCrashChild(t *testing.T) {
	root := os.Getenv("SN_SYNTHETIC_SNAPSHOT_CRASH_ROOT")
	if root == "" {
		return
	}
	volume := durablefixture.New(t, t.Context(), root)
	volume.Context = durablevolume.WithReference(volume.Context, durablevolume.Reference{Path: os.Getenv("SN_SYNTHETIC_SNAPSHOT_CRASH_REFERENCE"), Sha256: os.Getenv("SN_SYNTHETIC_SNAPSHOT_CRASH_SHA256")})
	self := &fixture{root: root, volume: volume}
	if err := self.open(false); err != nil {
		t.Fatal(err)
	}
	stage := os.Getenv("SN_SYNTHETIC_SNAPSHOT_CRASH_STAGE")
	self.owner.at = func(point string) error {
		if point == stage {
			os.Exit(86)
		}
		return nil
	}
	if err := self.owner.Publish([]byte("child-original-signed-next"), nil); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash boundary not reached")
}

// Process death releases the real flock; a joined reopen recovers exact bytes.
func TestSnapshotHeadJoinedCrashRecovery(t *testing.T) {
	for _, stage := range []string{"temporary-created", "temporary-written", "snapshot-renamed", "snapshot-directory-synced", "predecessor-removed"} {
		t.Run(stage, func(t *testing.T) {
			self := newFixture(t)
			if err := self.owner.Publish([]byte("prior-acknowledged"), nil); err != nil {
				t.Fatal(err)
			}
			if err := self.close(); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestSnapshotHeadCrashChild$", "-test.timeout=30s")
			cmd.Env = append(os.Environ(), "SN_SYNTHETIC_SNAPSHOT_CRASH_ROOT="+self.root, "SN_SYNTHETIC_SNAPSHOT_CRASH_REFERENCE="+self.volume.Reference.Path, "SN_SYNTHETIC_SNAPSHOT_CRASH_SHA256="+self.volume.Reference.Sha256, "SN_SYNTHETIC_SNAPSHOT_CRASH_STAGE="+stage)
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 86 {
				t.Fatalf("child did not cross real boundary: %v %s", err, output)
			}
			err = self.open(true)
			if stage == "temporary-created" {
				if err == nil {
					t.Fatal("unanchored crash enrolled")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if raw, present, err := self.owner.Read(); err != nil || !present || string(raw) != "child-original-signed-next" {
				t.Fatal("crash bytes changed", string(raw), err)
			}
		})
	}
}
