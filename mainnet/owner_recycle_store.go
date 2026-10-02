// A private exclusive journal marks initial claim completion durably. Missing
// or corrupt completed state is never an unused owner nonce or signing allowance.
package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

const ownerRecycleStoreLimit = ownerSigningRequestLimit + 1024*1024

// Local locking is not global coldkey custody. The caller serializes methods;
// failed writes poison the store, including failures after an atomic rename.
type ownerRecycleStore struct {
	config        ownerRecycleConfig
	key           string
	lock          *os.File
	directory     *os.File
	marker        *bootstrapContractReadinessMarker
	complete      bool
	expectedHash  string
	failed        error
	syncDirectory func(*os.File) error
}

// Reserve uses exclusive creation; resume accepts only the same approved marker.
// No path comes from a portable request on an owner's separate signing computer.
func openOwnerRecycleStore(config ownerRecycleConfig, key string, create bool) (_ *ownerRecycleStore, resultErr error) {
	path := config.Action.StatePath
	if err := errors.Join(config.validate(key), bootstrapRootDirectory(filepath.Dir(path))); err != nil {
		return nil, err
	}
	root, err := bootstrapSuccessorPhysicalRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	directoryFd, err := unix.Open(filepath.Dir(path), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	self := &ownerRecycleStore{config: config, key: key, directory: os.NewFile(uintptr(directoryFd), filepath.Dir(path))}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, self.close())
		}
	}()
	flags := syscall.O_RDWR | syscall.O_CLOEXEC | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if create {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return nil, errors.Join(errors.New("recycle custody already exists; reopen original state"), err)
		}
		flags |= syscall.O_CREAT | syscall.O_EXCL
	}
	fd, err := unix.Openat(directoryFd, filepath.Base(path)+".lock", flags, 0600)
	if err != nil {
		return nil, err
	}
	self.lock = os.NewFile(uintptr(fd), path+".lock")
	self.marker = &bootstrapContractReadinessMarker{file: self.lock, path: path + ".lock", root: root}
	if err := bootstrapSuccessorPrivateRegular(self.lock); err != nil {
		return nil, errors.Join(errors.New("recycle marker is not a private regular file"), err)
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, errors.Join(errors.New("recycle custody has another local owner"), err)
	}
	marker := rootObjectHash(config) + "\n" + key + "\n"
	if create {
		if err := self.checkpoint(); err != nil {
			return nil, err
		}
		written, err := self.lock.WriteString(marker)
		if written != len(marker) && err == nil {
			err = io.ErrShortWrite
		}
		self.marker.expected = marker
		if err := errors.Join(err, self.lock.Sync(), self.syncParent(), self.checkpoint()); err != nil {
			return nil, err
		}
	} else {
		raw, err := io.ReadAll(io.LimitReader(self.lock, int64(len(marker)+len(bootstrapRootClaimComplete)+1)))
		if err != nil {
			return nil, err
		}
		if string(raw) == marker+bootstrapRootClaimComplete {
			self.marker.expected, self.complete = string(raw), true
			if _, err := self.load(); err != nil {
				return nil, err
			}
			return self, nil
		}
		if string(raw) != marker {
			return nil, errors.New("recycle marker differs from original independently approved config")
		}
		self.marker.expected = marker
	}
	record, err := self.load()
	if errors.Is(err, os.ErrNotExist) {
		record = ownerRecycleRecord{Schema: ownerRecycleRecordSchema, Config: config, ApprovalKey: key, Phase: "reserved"}
		record.ContentHash = rootObjectHash(record)
		if err := self.save(record); err != nil {
			return nil, err
		}
	} else if err != nil || record.Phase != "reserved" {
		return nil, errors.Join(errors.New("recycle interrupted claim has invalid or advanced progress"), err)
	}
	if _, err := self.load(); err != nil {
		return nil, err
	}
	written, err := self.lock.WriteAt([]byte(bootstrapRootClaimComplete), int64(len(marker)))
	if written != len(bootstrapRootClaimComplete) && err == nil {
		err = io.ErrShortWrite
	}
	self.marker.expected, self.complete = marker+bootstrapRootClaimComplete, true
	if err := errors.Join(err, self.lock.Sync(), self.checkpoint()); err != nil {
		return nil, err
	}
	return self, nil
}

// Releasing local ownership preserves the permanent original claim marker.
func (self *ownerRecycleStore) close() error {
	if self == nil {
		return nil
	}
	var err error
	for _, file := range []*os.File{self.lock, self.directory} {
		if file != nil {
			err = errors.Join(err, file.Close())
		}
	}
	self.lock, self.directory = nil, nil
	return err
}

// Bounded, no-follow reads reject truncation, unknown JSON and altered approval.
func (self *ownerRecycleStore) load() (ownerRecycleRecord, error) {
	var record ownerRecycleRecord
	raw, err := self.readRecord()
	if err != nil {
		return record, err
	}
	if err := decodePlanJson(raw, &record); err != nil {
		return ownerRecycleRecord{}, self.failIntegrity(err)
	}
	if err := record.validate(self.config, self.key); err != nil {
		return ownerRecycleRecord{}, self.failIntegrity(err)
	}
	self.expectedHash = monitorReadDigest(raw)
	return record, nil
}

// File sync, atomic rename and parent sync precede acknowledgment. The original
// bytes may already be durable on error; never roll back or retry in this instance.
func (self *ownerRecycleStore) save(record ownerRecycleRecord) (resultErr error) {
	defer func() {
		if resultErr != nil {
			self.failed = resultErr
		}
	}()
	if err := self.checkpoint(); err != nil {
		return err
	}
	if err := record.validate(self.config, self.key); err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil || len(raw)+1 > ownerRecycleStoreLimit {
		return errors.Join(errors.New("recycle journal exceeds bound"), err)
	}
	return self.publishRecord(record, append(raw, '\n'))
}

// The scoped hook makes post-rename failure deterministic in qualification.
func (self *ownerRecycleStore) syncParent() error {
	if self.directory == nil {
		return errors.New("recycle directory is closed")
	}
	syncDirectory := self.syncDirectory
	if syncDirectory == nil {
		syncDirectory = (*os.File).Sync
	}
	return syncDirectory(self.directory)
}
