// Native custody retains the physical marker and exact preceding journal across
// publication. All file effects stay relative to the acquired private directory.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Observed custody loss permanently poisons this instance, even after restoration.
func (self *ownerTrimStore) failIntegrity(err error) error {
	if self.failed == nil {
		self.failed = fmt.Errorf("%w: owner trim original custody: %w", errRpcIntegrity, err)
	}
	return self.failed
}

// A held flock does not follow a renamed pathname or a replaced parent.
func (self *ownerTrimStore) checkpoint() error {
	if self == nil || self.lock == nil || self.directory == nil {
		return errors.New("owner trim store is closed")
	}
	if self.failed != nil {
		return self.failed
	}
	if err := errors.Join(self.retained.checkpoint(context.Background()), self.marker.checkpoint()); err != nil {
		return self.failIntegrity(err)
	}
	var directory unix.Stat_t
	if err := unix.Fstat(int(self.directory.Fd()), &directory); err != nil {
		return self.failIntegrity(err)
	}
	if uint64(directory.Dev) != self.marker.root.Device || directory.Ino != self.marker.root.Inode || directory.Mode&0077 != 0 {
		return self.failIntegrity(errors.New("physical directory descriptor changed"))
	}
	return nil
}

// Only the original incomplete empty claim may lack its first reserved record.
// Completed or already observed custody cannot be reconstructed from memory.
func (self *ownerTrimStore) readRecord() ([]byte, error) {
	if err := self.checkpoint(); err != nil {
		return nil, err
	}
	name := filepath.Base(self.config.Action.StatePath)
	fd, err := unix.Openat(int(self.directory.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !self.complete && self.expectedHash == "" {
			return nil, err
		}
		return nil, self.failIntegrity(err)
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	if err := bootstrapSuccessorPrivateRegular(file); err != nil {
		return nil, self.failIntegrity(err)
	}
	raw, err := io.ReadAll(io.LimitReader(file, ownerTrimStoreLimit+1))
	if err != nil || len(raw) == 0 || len(raw) > ownerTrimStoreLimit {
		return nil, self.failIntegrity(errors.Join(errors.New("journal is not a bounded complete record"), err))
	}
	var opened, named unix.Stat_t
	if err := errors.Join(unix.Fstat(fd, &opened), unix.Fstatat(int(self.directory.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW), self.checkpoint()); err != nil {
		return nil, self.failIntegrity(err)
	}
	if opened.Dev != named.Dev || opened.Ino != named.Ino || self.expectedHash != "" && monitorReadDigest(raw) != self.expectedHash {
		return nil, self.failIntegrity(errors.New("retained journal changed during ownership"))
	}
	return raw, nil
}

// Validate the same predecessor before rename and reread after directory sync.
// A successful write never grants permission to recreate a missing signed row.
func (self *ownerTrimStore) publishRecord(record ownerTrimRecord, raw []byte) error {
	checkPrevious := func() error {
		_, err := self.load()
		if errors.Is(err, os.ErrNotExist) && !self.complete && self.expectedHash == "" && record.Phase == "reserved" && record.Signature == "" {
			return self.checkpoint()
		}
		return err
	}
	if err := checkPrevious(); err != nil {
		return err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	stage := ".owner-trim-action-" + hex.EncodeToString(nonce[:])
	directoryFd := int(self.directory.Fd())
	fd, err := unix.Openat(directoryFd, stage, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(directoryFd, stage, 0)
	file := os.NewFile(uintptr(fd), stage)
	written, writeErr := file.Write(raw)
	if written != len(raw) && writeErr == nil {
		writeErr = io.ErrShortWrite
	}
	if err := errors.Join(writeErr, file.Sync(), file.Close(), checkPrevious()); err != nil {
		return err
	}
	if err := unix.Renameat(directoryFd, stage, directoryFd, filepath.Base(self.config.Action.StatePath)); err != nil {
		return err
	}
	self.expectedHash = monitorReadDigest(raw)
	if err := self.syncParent(); err != nil {
		return err
	}
	_, err = self.load()
	return err
}
