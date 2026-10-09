//go:build linux || darwin

package clientauth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/unix"
)

// RenewNetworkToken replaces the network token at path with its renewal,
// while the file still holds previous. It answers false, without an error,
// when the file holds another token: an explicit sign-in or another renewer
// wrote it first, and the renewal is discarded.
//
// The renewal holds the token's registration owner lock (the lock a
// measurement registration takes to borrow the token), and is published in
// the lineage before the renewed file is renamed into place
// (network_token.go). ErrNetworkTokenBusy is a held lock; try again later.
func RenewNetworkToken(path string, previous string, renewed string) (_ bool, returnErr error) {
	previous, renewed = strings.TrimSpace(previous), strings.TrimSpace(renewed)
	if previous == "" || renewed == "" || previous == renewed {
		return false, errors.New("network token renewal needs a previous and a different renewed token")
	}
	store, err := openRegistrationStore(path)
	if err != nil {
		if errors.Is(err, errRegistrationOwnerActive) {
			return false, ErrNetworkTokenBusy
		}
		return false, err
	}
	defer func() { returnErr = errors.Join(returnErr, store.close()) }()
	name := filepath.Base(path)
	if err := store.removeNetworkRenewalLeftovers(name); err != nil {
		return false, err
	}
	current, fingerprint, err := store.readNetworkCredential(name)
	if err != nil {
		return false, err
	}
	if current != previous {
		return false, nil
	}
	lineageName := name + ".lineage"
	var members []string
	if raw, err := store.read(lineageName); err == nil {
		members, err = parseNetworkCredentialLineage(raw)
		if err != nil {
			return false, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if !slices.Contains(members, fingerprint) {
		// the current file began its sign-in: an explicit sign-in, or a file
		// renewed before lineages existed
		members = []string{fingerprint}
	}

	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return false, err
	}
	temporary := networkRenewalLeftoverPrefix(name) + hex.EncodeToString(random[:])
	file, err := store.open(temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL)
	if err != nil {
		return false, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			returnErr = errors.Join(returnErr, store.remove(temporary))
		}
	}()
	_, writeErr := io.WriteString(file, renewed)
	syncErr := file.Sync()
	var stat unix.Stat_t
	statErr := unix.Fstat(int(file.Fd()), &stat)
	if err := errors.Join(writeErr, syncErr, statErr, file.Close()); err != nil {
		return false, err
	}
	if hook := networkRenewalTestHooks.afterTemporary; hook != nil && hook() {
		cleanup = false
		return false, errNetworkRenewalTestStop
	}
	renewedFingerprint := networkCredentialFingerprintOf(renewed, stat.Mtim.Nano(), true)
	encoded, err := encodeNetworkCredentialLineage(appendNetworkCredentialLineage(members, renewedFingerprint))
	if err != nil {
		return false, err
	}
	// published first: a crash before the rename leaves the old file, whose
	// fingerprint stays in the lineage, so every marker still blocks
	if err := store.write(lineageName, encoded); err != nil {
		return false, err
	}
	if hook := networkRenewalTestHooks.afterLineage; hook != nil && hook() {
		cleanup = false
		return false, errNetworkRenewalTestStop
	}
	if err := store.check(); err != nil {
		return false, err
	}
	if err := unix.Renameat(int(store.directory.Fd()), temporary, int(store.directory.Fd()), name); err != nil {
		return false, err
	}
	cleanup = false
	return true, errors.Join(store.directory.Sync(), store.check())
}

// networkRenewalTestHooks stop a renewal right after one of its durable
// steps the way a process that dies there would: nothing after the step runs,
// and nothing is cleaned up. A hook that answers true stops the renewal.
var networkRenewalTestHooks struct {
	afterTemporary func() bool
	afterLineage   func() bool
}

var errNetworkRenewalTestStop = errors.New("network renewal stopped by its test hook")

func networkRenewalLeftoverPrefix(name string) string {
	return "." + name + ".renew-"
}

// A renewal interrupted before its rename leaves its renewed file. That file
// was never installed, so it is removed; the lineage member it added names a
// file that never existed and blocks nothing on its own.
func (self *registrationStore) removeNetworkRenewalLeftovers(name string) error {
	names, err := self.names(4096)
	if err != nil {
		return err
	}
	prefix := networkRenewalLeftoverPrefix(name)
	for _, leftover := range names {
		if strings.HasPrefix(leftover, prefix) {
			if err := self.remove(leftover); err != nil {
				return err
			}
		}
	}
	return nil
}

// The store's read of the token, with the fingerprint from the same file.
func (self *registrationStore) readNetworkCredential(name string) (token string, fingerprint string, returnErr error) {
	if err := self.check(); err != nil {
		return "", "", err
	}
	file, err := self.open(name, unix.O_RDONLY)
	if err != nil {
		return "", "", err
	}
	defer func() { returnErr = errors.Join(returnErr, file.Close()) }()
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return "", "", err
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximumRegistrationBytes+1))
	if err != nil {
		return "", "", err
	}
	if len(raw) > maximumRegistrationBytes {
		return "", "", errors.New("network token exceeds its finite bound")
	}
	token = strings.TrimSpace(string(raw))
	if token == "" {
		return "", "", errors.New("network token is empty")
	}
	return token, networkCredentialFingerprintOf(token, stat.Mtim.Nano(), true), self.check()
}
