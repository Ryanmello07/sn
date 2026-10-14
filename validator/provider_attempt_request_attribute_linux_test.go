//go:build linux

// A real ext4 block admits the forecast's widest attributes. This is the
// filesystem the forecast models; other filesystems give each value more room.
package validator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/urnetwork/connect/durablevolume"
	"golang.org/x/sys/unix"
)

// Create the four attributes of a prepared ledger root in the order
// preparation publishes them, the request attribute holding request.
func writeProviderRequestTestRootAttributes(path string, preparation ProviderAttemptRequestPreparation, request []byte) error {
	custody, anchor, err := providerAttemptRequestWidestRootAttributes(preparation)
	if err != nil {
		return err
	}
	for _, attribute := range []struct {
		name  string
		value []byte
	}{{durablevolume.RootGenerationAttribute, bytes.Repeat([]byte{7}, durablevolume.RootGenerationBytes)}, {attemptLedgerCustodyAttribute, custody}, {ProviderAttemptRequestAttribute, request}, {durablevolume.PreparationAttribute, anchor}} {
		if err := unix.Setxattr(path, attribute.name, attribute.value, unix.XATTR_CREATE); err != nil {
			return fmt.Errorf("%s: %w", attribute.name, err)
		}
	}
	return nil
}

// The custody checkpoint at exactly the forecast budget fits one real ext4
// block beside the widest anchor, the generation and the request record, and
// same-width replacements still fit. Beside a production-sized identity's
// widest attributes, the sealed v1 checkpoint gets ENOSPC and v2 fits.
func TestProviderRequestRootAttributesFitOneRealExt4Block(t *testing.T) {
	parent := t.TempDir()
	var filesystem unix.Statfs_t
	if err := unix.Statfs(parent, &filesystem); err != nil {
		t.Fatal(err)
	}
	if filesystem.Type != unix.EXT4_SUPER_MAGIC || filesystem.Bsize != ext4AttributeBlockBytes {
		t.Skip("TMPDIR is not ext4 with 4 KiB blocks", filesystem.Type, filesystem.Bsize)
	}
	directory := func(name string) string {
		path := filepath.Join(parent, name)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	budget := providerRequestBudgetPreparation(t, 2844)
	if err := budget.Validate(); err != nil {
		t.Fatal(err)
	}
	widest := providerRequestWidestTestCheckpoint(budget)
	request, err := encodeProviderAttemptRequestCheckpoint(widest)
	if err != nil {
		t.Fatal(err)
	}
	path := directory("budget")
	if err := writeProviderRequestTestRootAttributes(path, budget, request); err != nil {
		t.Fatal("ext4 refused the forecast's widest root attributes", err)
	}
	custody, _, err := providerAttemptRequestWidestRootAttributes(budget)
	if err != nil {
		t.Fatal(err)
	}
	fresh := widest
	fresh.Pending, fresh.Closed = nil, nil
	replacement, err := encodeProviderAttemptRequestCheckpoint(fresh)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string][]byte{attemptLedgerCustodyAttribute: custody, ProviderAttemptRequestAttribute: replacement} {
		if err := unix.Setxattr(path, name, value, unix.XATTR_REPLACE); err != nil {
			t.Fatal("ext4 refused a same-width replacement", name, err)
		}
	}
	if raw := providerRequestTestAttribute(t, path); !bytes.Equal(raw, replacement) {
		t.Fatal("request replacement did not read back")
	}
	production := providerRequestTestPreparation("sn25-mainnet-20261007-01")
	legacy, err := json.Marshal(providerRequestWidestTestCheckpoint(production))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeProviderRequestTestRootAttributes(directory("v1"), production, legacy); !errors.Is(err, unix.ENOSPC) {
		t.Fatal("sealed v1 checkpoint fit beside production-sized root attributes", len(legacy), err)
	}
	request, err = encodeProviderAttemptRequestCheckpoint(providerRequestWidestTestCheckpoint(production))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeProviderRequestTestRootAttributes(directory("v2"), production, request); err != nil {
		t.Fatal("ext4 refused the v2 record beside production-sized root attributes", err)
	}
}
