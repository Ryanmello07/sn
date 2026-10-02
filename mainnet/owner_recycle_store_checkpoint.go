// Retained native intent keeps its physical marker, private directory and exact
// predecessor throughout local ownership. Cross-host rollback needs its own fence.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Restoring a pathname after observed custody loss cannot revive this instance.
func (self *ownerRecycleStore) failIntegrity(err error) error {
	if self.failed == nil {
		self.failed = fmt.Errorf("%w: recycle original custody: %w", errRpcIntegrity, err)
	}
	return self.failed
}

// A flock does not follow replacement of its name or its containing directory.
func (self *ownerRecycleStore) checkpoint() error {
	if self == nil || self.lock == nil || self.directory == nil {
		return errors.New("recycle store is closed")
	}
	if self.failed != nil {
		return self.failed
	}
	if err := self.marker.checkpoint(); err != nil {
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

// Only an incomplete empty initial claim may lack its first reserved record.
func (self *ownerRecycleStore) readRecord() ([]byte, error) {
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
	raw, err := io.ReadAll(io.LimitReader(file, ownerRecycleStoreLimit+1))
	if err != nil || len(raw) == 0 || len(raw) > ownerRecycleStoreLimit {
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

// A durable successor replaces only its still-retained original predecessor;
// no in-memory signature or receipt authorizes recreating a deleted journal.
func (self *ownerRecycleStore) publishRecord(record ownerRecycleRecord, raw []byte) error {
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
	stage := ".owner-recycle-action-" + hex.EncodeToString(nonce[:])
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
