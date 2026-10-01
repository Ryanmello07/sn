// Descriptor-relative original action custody refuses missing journals and
// replaced physical fences throughout an owner's lifetime.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// A held flock protects its inode only. The approved path must still name that
// private single-link marker under the same physical directory.
func (self *evmActionStore) checkpoint() error {
	if self == nil || self.lock == nil || self.directory == nil {
		return errors.New("EVM journal is closed")
	}
	physical, err := bootstrapSuccessorPhysicalRoot(self.config.Plan.RunDirectory)
	if err != nil || physical != self.root {
		return errors.Join(errors.New("EVM journal physical directory changed"), err)
	}
	var directoryStat, markerStat, namedStat unix.Stat_t
	if err := unix.Fstat(int(self.directory.Fd()), &directoryStat); err != nil {
		return err
	}
	if uint64(directoryStat.Dev) != self.root.Device || directoryStat.Ino != self.root.Inode || directoryStat.Mode&0077 != 0 {
		return errors.New("EVM journal directory descriptor changed")
	}
	if err := bootstrapSuccessorPrivateRegular(self.lock); err != nil {
		return err
	}
	if err := errors.Join(unix.Fstat(int(self.lock.Fd()), &markerStat),
		unix.Fstatat(int(self.directory.Fd()), filepath.Base(self.path)+".lock", &namedStat, unix.AT_SYMLINK_NOFOLLOW)); err != nil {
		return err
	}
	if markerStat.Dev != namedStat.Dev || markerStat.Ino != namedStat.Ino || markerStat.Size != int64(len(self.marker)) {
		return errors.New("EVM journal ownership marker changed")
	}
	raw, err := io.ReadAll(io.NewSectionReader(self.lock, 0, int64(len(self.marker))+1))
	if err != nil || string(raw) != self.marker {
		return errors.Join(errors.New("EVM journal ownership marker bytes changed"), err)
	}
	return nil
}

// Reads share the acquired directory and reject links or a changed path while
// retaining the same bounded record format used by existing approvals.
func (self *evmActionStore) readRecord() ([]byte, error) {
	if err := self.checkpoint(); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(int(self.directory.Fd()), filepath.Base(self.path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), self.path)
	defer file.Close()
	if err := bootstrapSuccessorPrivateRegular(file); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(file, 512*1024+1))
	if err != nil || len(raw) == 0 || len(raw) > 512*1024 {
		return nil, errors.Join(errors.New("EVM journal is not a bounded complete record"), err)
	}
	var fileStat, namedStat unix.Stat_t
	if err := errors.Join(unix.Fstat(fd, &fileStat),
		unix.Fstatat(int(self.directory.Fd()), filepath.Base(self.path), &namedStat, unix.AT_SYMLINK_NOFOLLOW), self.checkpoint()); err != nil {
		return nil, err
	}
	if fileStat.Dev != namedStat.Dev || fileStat.Ino != namedStat.Ino {
		return nil, errors.New("EVM journal changed while reading custody")
	}
	return raw, nil
}

// Only an incomplete, never-loaded initial claim may create its first journal.
// Every later write compares the exact retained predecessor before replacement
// and reopens the published result after directory durability acknowledgement.
func (self *evmActionStore) publishRecord(record evmActionRecord, raw []byte) error {
	checkPrevious := func() error {
		_, err := self.load()
		if errors.Is(err, os.ErrNotExist) && !self.complete && self.retainedHash == "" && record.Signed == "" {
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
	stage := ".sn-mainnet-evm-" + hex.EncodeToString(nonce[:])
	directoryFd := int(self.directory.Fd())
	fd, err := unix.Openat(directoryFd, stage, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(directoryFd, stage, 0)
	file := os.NewFile(uintptr(fd), stage)
	written, err := file.Write(raw)
	if written != len(raw) && err == nil {
		err = io.ErrShortWrite
	}
	if err := errors.Join(err, file.Sync(), file.Close(), checkPrevious()); err != nil {
		return err
	}
	if err := unix.Renameat(directoryFd, stage, directoryFd, filepath.Base(self.path)); err != nil {
		return err
	}
	self.retainedHash = record.ContentHash
	if err := self.syncParent(); err != nil {
		return err
	}
	_, err = self.load()
	return err
}
