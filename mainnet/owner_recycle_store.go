// A private exclusive journal marks initial claim completion durably. Missing
// or corrupt completed state is never an unused owner nonce or signing allowance.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const ownerRecycleStoreLimit = ownerSigningRequestLimit + 1024*1024

// Local locking is not global coldkey custody. The caller serializes methods;
// failed writes poison the store, including failures after an atomic rename.
type ownerRecycleStore struct {
	config        ownerRecycleConfig
	key           string
	lock          *os.File
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
	flags := syscall.O_RDWR | syscall.O_CLOEXEC | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if create {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return nil, errors.Join(errors.New("recycle custody already exists; reopen original state"), err)
		}
		flags |= syscall.O_CREAT | syscall.O_EXCL
	}
	fd, err := syscall.Open(path+".lock", flags, 0600)
	if err != nil {
		return nil, err
	}
	self := &ownerRecycleStore{config: config, key: key, lock: os.NewFile(uintptr(fd), path+".lock")}
	defer func() {
		if resultErr != nil {
			self.close()
		}
	}()
	info, err := self.lock.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.Join(errors.New("recycle marker is not a private regular file"), err)
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, errors.Join(errors.New("recycle custody has another local owner"), err)
	}
	marker := rootObjectHash(config) + "\n" + key + "\n"
	if create {
		written, err := self.lock.WriteString(marker)
		if written != len(marker) && err == nil {
			err = io.ErrShortWrite
		}
		if err := errors.Join(err, self.lock.Sync(), self.syncParent()); err != nil {
			return nil, err
		}
	} else {
		raw, err := io.ReadAll(io.LimitReader(self.lock, int64(len(marker)+len(bootstrapRootClaimComplete)+1)))
		if err != nil {
			return nil, err
		}
		if string(raw) == marker+bootstrapRootClaimComplete {
			if _, err := self.load(); err != nil {
				return nil, err
			}
			return self, nil
		}
		if string(raw) != marker {
			return nil, errors.New("recycle marker differs from original independently approved config")
		}
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
	written, err := self.lock.WriteAt([]byte(bootstrapRootClaimComplete), int64(len(marker)))
	if written != len(bootstrapRootClaimComplete) && err == nil {
		err = io.ErrShortWrite
	}
	if err := errors.Join(err, self.lock.Sync()); err != nil {
		return nil, err
	}
	return self, nil
}

// Releasing local ownership preserves the permanent original claim marker.
func (self *ownerRecycleStore) close() error {
	if self.lock == nil {
		return nil
	}
	err := self.lock.Close()
	self.lock = nil
	return err
}

// Bounded, no-follow reads reject truncation, unknown JSON and altered approval.
func (self *ownerRecycleStore) load() (ownerRecycleRecord, error) {
	var record ownerRecycleRecord
	if self.lock == nil || self.failed != nil {
		return record, errors.Join(errors.New("recycle store must reopen"), self.failed)
	}
	raw, _, err := readBootstrapRootFile(context.Background(), self.config.Action.StatePath, ownerRecycleStoreLimit)
	if err != nil {
		return record, err
	}
	if err := decodePlanJson(raw, &record); err != nil {
		return record, err
	}
	return record, record.validate(self.config, self.key)
}

// File sync, atomic rename and parent sync precede acknowledgment. The original
// bytes may already be durable on error; never roll back or retry in this instance.
func (self *ownerRecycleStore) save(record ownerRecycleRecord) (resultErr error) {
	if self.lock == nil || self.failed != nil {
		return errors.Join(errors.New("recycle store must reopen"), self.failed)
	}
	defer func() {
		if resultErr != nil {
			self.failed = resultErr
		}
	}()
	if err := record.validate(self.config, self.key); err != nil {
		return err
	}
	path := self.config.Action.StatePath
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("recycle journal is not a private regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil || len(raw)+1 > ownerRecycleStoreLimit {
		return errors.Join(errors.New("recycle journal exceeds bound"), err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".owner-recycle-action-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	raw = append(raw, '\n')
	written, err := file.Write(raw)
	if written != len(raw) && err == nil {
		err = io.ErrShortWrite
	}
	if err := errors.Join(err, file.Sync(), file.Close()); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	return self.syncParent()
}

// The scoped hook makes post-rename failure deterministic in qualification.
func (self *ownerRecycleStore) syncParent() error {
	directory, err := os.Open(filepath.Dir(self.config.Action.StatePath))
	if err != nil {
		return err
	}
	syncDirectory := self.syncDirectory
	if syncDirectory == nil {
		syncDirectory = (*os.File).Sync
	}
	return errors.Join(syncDirectory(directory), directory.Close())
}
