// Campaign archive admission owns native descriptors through the last Close.
// These regressions use real files/FIFOs and preserve all existing byte limits.
package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// Empty and arbitrary binary files remain valid; the exact fixed ceiling
// succeeds and the next byte is refused before any read operation.
func TestCampaignEvidenceRegularFileCustodyPreservesBytesAndBounds(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "owner")
	if err := ensurePrivateDir(directory); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		name string
		raw  []byte
	}{
		{name: "empty.bin", raw: []byte{}},
		{name: "nested/binary.bin", raw: []byte{0, 0xff, '\n', 'x'}},
	} {
		if err := atomicWrite(filepath.Join(directory, filepath.FromSlash(input.name)), input.raw, 0o600); err != nil {
			t.Fatal(err)
		}
		var opened *os.File
		reads := 0
		got, err := readCampaignEvidenceRegularFileObserved(directory, input.name, func(stage string, file *os.File) error {
			if stage == "opened" {
				opened = file
			}
			if stage == "read" {
				reads++
			}
			return nil
		})
		if err != nil || !bytes.Equal(got, input.raw) || opened == nil || reads != 1 {
			t.Fatalf("%s exact regular bytes changed: reads=%d error=%v", input.name, reads, err)
		}
		if _, err := opened.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("%s returned with an unclosed source: %v", input.name, err)
		}
	}
	for _, name := range []string{"../escape.bin", "/absolute.bin", "nested/../binary.bin", "nested//binary.bin", "nested\\binary.bin", "null\x00.bin", "complete.json", campaignEvidenceManifestFilename, "scenario-complete-commit.operator-1.evidence.json"} {
		opened := 0
		got, err := readCampaignEvidenceRegularFileObserved(directory, name, func(string, *os.File) error { opened++; return nil })
		if err == nil || got != nil || opened != 0 {
			t.Fatalf("%q path admission opened a source: opened=%d error=%v", name, opened, err)
		}
	}
	if maximumCampaignEvidenceRawFileBytes != 32*1024*1024 {
		t.Fatal("campaign raw-file ceiling changed")
	}
	for _, extra := range []int64{0, 1} {
		name := "ceiling.bin"
		path := filepath.Join(directory, name)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		size := int64(maximumCampaignEvidenceRawFileBytes) + extra
		if err := file.Truncate(size); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if _, err := file.WriteAt([]byte{0x7e}, 0); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if _, err := file.WriteAt([]byte{0x33}, size-1); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		reads := 0
		var opened *os.File
		raw, err := readCampaignEvidenceRegularFileObserved(directory, name, func(stage string, file *os.File) error {
			if stage == "opened" {
				opened = file
			}
			if stage == "read" {
				reads++
			}
			return nil
		})
		if extra == 0 {
			if err != nil || len(raw) != maximumCampaignEvidenceRawFileBytes || reads != 1 {
				t.Fatalf("exact campaign ceiling was refused: bytes=%d reads=%d error=%v", len(raw), reads, err)
			}
			if raw[0] != 0x7e || raw[len(raw)-1] != 0x33 || bytes.Count(raw, []byte{0}) != len(raw)-2 {
				t.Fatal("exact campaign ceiling changed sparse source bytes")
			}
		} else if err == nil || raw != nil || reads != 0 {
			t.Fatalf("one-byte-over campaign source crossed stat admission: reads=%d error=%v", reads, err)
		}
		if opened == nil {
			t.Fatal("campaign ceiling control did not acquire its actual file")
		}
		if _, err := opened.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("campaign ceiling source remained open: %v", err)
		}
	}
}

// A held peer lets the original blocking open return without a timeout.
// Fd restores os.Open's runtime-added mode but preserves an explicitly native
// nonblocking NewFile. Only after that causal assertion succeeds do we close
// the peer and repeat the production refusal with no FIFO writer.
func TestCampaignEvidenceRegularFileCustodyUsesNativeNonblockingOpen(t *testing.T) {
	for _, name := range []string{"result.json", "public/history.json", "receipts/postconditions/source.json", "final-inputs/manifest.json"} {
		func() {
			directory := filepath.Join(t.TempDir(), "owner")
			path := filepath.Join(directory, filepath.FromSlash(name))
			original := []byte("1234")
			if err := atomicWrite(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := readCampaignEvidenceRegularFile(directory, name); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("%s regular source prerequisite: %v", name, err)
			}
			if err := os.Rename(path, path+".original"); err != nil {
				t.Fatal(err)
			}
			if err := unix.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
			keeper, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			keeperClosed := false
			defer func() {
				if !keeperClosed {
					if err := unix.Close(keeper); err != nil {
						t.Error(err)
					}
				}
			}()
			refusedBlocking := errors.New("campaign source exposed a blocking native descriptor")
			opens, reads, nativeFlags := 0, 0, 0
			raw, err := readCampaignEvidenceRegularFileObserved(directory, name, func(stage string, file *os.File) error {
				if stage == "read" {
					reads++
					return nil
				}
				if stage != "opened" {
					return errors.New("unexpected campaign custody stage")
				}
				opens++
				info, err := file.Stat()
				if err != nil {
					return err
				}
				if info.Mode()&os.ModeNamedPipe == 0 {
					return errors.New("campaign fixture did not acquire its actual FIFO")
				}
				nativeFlags, err = unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
				if err != nil {
					return err
				}
				if nativeFlags&unix.O_NONBLOCK == 0 {
					return refusedBlocking
				}
				return nil
			})
			if opens != 1 || reads != 0 || nativeFlags&unix.O_NONBLOCK == 0 || errors.Is(err, refusedBlocking) || err == nil || raw != nil {
				t.Fatalf("%s campaign FIFO acquisition is not natively nonblocking: opens=%d reads=%d flags=%d error=%v", name, opens, reads, nativeFlags, err)
			}
			if err := unix.Close(keeper); err != nil {
				t.Fatal(err)
			}
			keeperClosed = true
			if raw, err := readCampaignEvidenceRegularFile(directory, name); err == nil || raw != nil {
				t.Fatalf("%s writer-free campaign FIFO returned bytes: %v", name, err)
			}
			preserved, err := os.ReadFile(path + ".original")
			if err != nil || !bytes.Equal(preserved, original) {
				t.Fatalf("%s FIFO refusal changed original bytes: %v", name, err)
			}
		}()
	}
}

// Resolving back to the same file does not authorize a root, intermediate
// directory or leaf alias in the signed campaign archive namespace.
func TestCampaignEvidenceRegularFileCustodyRejectsSameOwnerAliases(t *testing.T) {
	for _, location := range []string{"root", "parent", "leaf"} {
		directory := filepath.Join(t.TempDir(), "owner")
		name := "public/source.json"
		path := filepath.Join(directory, filepath.FromSlash(name))
		original := []byte("1234")
		if err := atomicWrite(path, original, 0o600); err != nil {
			t.Fatal(err)
		}
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if raw, err := readCampaignEvidenceRegularFile(directory, name); err != nil || !bytes.Equal(raw, original) {
			t.Fatalf("%s direct source prerequisite: %v", location, err)
		}
		switch location {
		case "root":
			alias := directory + "-alias"
			if err := os.Symlink(directory, alias); err != nil {
				t.Fatal(err)
			}
			directory = alias
		case "parent":
			parent := filepath.Dir(path)
			if err := os.Rename(parent, parent+"-original"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Base(parent)+"-original", parent); err != nil {
				t.Fatal(err)
			}
		case "leaf":
			if err := os.Rename(path, path+".original"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Base(path)+".original", path); err != nil {
				t.Fatal(err)
			}
		}
		aliasedPath := filepath.Join(directory, filepath.FromSlash(name))
		after, err := os.Stat(aliasedPath)
		if err != nil || !os.SameFile(before, after) {
			t.Fatalf("%s alias is not the actual same inode: %v", location, err)
		}
		if raw, err := readCampaignEvidenceRegularFile(directory, name); err == nil || raw != nil {
			t.Fatalf("%s campaign alias acquired source authority: %v", location, err)
		}
		preserved, err := os.ReadFile(aliasedPath)
		if err != nil || !bytes.Equal(preserved, original) {
			t.Fatalf("%s alias refusal changed source bytes: %v", location, err)
		}
	}
}

// O_DIRECTORY must admit root/parent components before a FIFO could block.
// A leaf directory is refused without reaching the body-read boundary.
func TestCampaignEvidenceRegularFileCustodyRejectsSpecialComponents(t *testing.T) {
	for _, location := range []string{"root-fifo", "parent-fifo", "leaf-directory"} {
		directory := filepath.Join(t.TempDir(), "owner")
		if err := ensurePrivateDir(directory); err != nil {
			t.Fatal(err)
		}
		name := "source.json"
		switch location {
		case "root-fifo":
			fifo := filepath.Join(directory, "root")
			if err := unix.Mkfifo(fifo, 0o600); err != nil {
				t.Fatal(err)
			}
			directory = fifo
		case "parent-fifo":
			if err := unix.Mkfifo(filepath.Join(directory, "parent"), 0o600); err != nil {
				t.Fatal(err)
			}
			name = "parent/source.json"
		case "leaf-directory":
			if err := ensurePrivateDir(filepath.Join(directory, name)); err != nil {
				t.Fatal(err)
			}
		}
		reads := 0
		raw, err := readCampaignEvidenceRegularFileObserved(directory, name, func(stage string, _ *os.File) error {
			if stage == "read" {
				reads++
			}
			return nil
		})
		if err == nil || raw != nil || reads != 0 {
			t.Fatalf("%s special component became campaign bytes: reads=%d error=%v", location, reads, err)
		}
	}
}

// The real owned descriptor is closed after a complete successful read. The
// function's final Close therefore fails; no bytes survive that failure, even
// when another late failure must also retain its own error identity.
func TestCampaignEvidenceRegularFileCustodyRejectsLateCloseFailure(t *testing.T) {
	for _, additionalFailure := range []bool{false, true} {
		directory := filepath.Join(t.TempDir(), "owner")
		if err := atomicWrite(filepath.Join(directory, "source.json"), []byte("complete source\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		injected := errors.New("campaign read-boundary observation failed")
		reads := 0
		raw, err := readCampaignEvidenceRegularFileObserved(directory, "source.json", func(stage string, file *os.File) error {
			if stage != "read" {
				return nil
			}
			reads++
			if err := file.Close(); err != nil {
				return err
			}
			if additionalFailure {
				return injected
			}
			return nil
		})
		if reads != 1 || raw != nil || !errors.Is(err, os.ErrClosed) || additionalFailure && !errors.Is(err, injected) {
			t.Fatalf("campaign read lost its actual final Close failure: additional=%t reads=%d bytes=%d error=%v", additionalFailure, reads, len(raw), err)
		}
	}
}
