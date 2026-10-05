package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/urnetwork/connect/durablevolume"
)

// Failures follow actual descriptor operations and cannot supply replacement
// source bytes. The same original journal can proceed after observation returns.
func TestRepairOperatorResourceObservationUnavailableRetainsCustody(t *testing.T) {
	for _, operation := range []string{"operator-resource-open-stat", "operator-resource-read", "operator-resource-close", "operator-resource-directory-stat", "operator-resource-directory-close", "operator-quota-open-stat"} {
		f := newRepairOperatorFixture(t)
		f.claim()
		before := mainnetNamespaceTest(t, f.envelope.original.Plan.StateDirectory)
		faults := 0
		ctx := context.WithValue(f.ctx(), repairValidatorObservationKey{}, func(observed string) error {
			if observed == operation && faults == 0 {
				faults++
				return syscall.EIO
			}
			return nil
		})
		status, complete, err := f.envelope.resume(ctx, f.key, f.base.host, func() time.Time { return f.base.now })
		disposition, _ := repairControllerCause(err)
		if faults != 1 || status != "source-refused" || complete || f.base.starts != 0 || !errors.Is(err, syscall.EIO) || !errors.Is(err, durablevolume.ErrUnavailable) || errors.Is(err, errRpcIntegrity) || errors.Is(err, durablevolume.ErrIdentity) || disposition != "pending" {
			t.Fatal("unavailable operator descriptor changed original custody meaning", operation, faults, status, complete, err, f.base.starts, disposition)
		}
		raw, err := os.ReadFile(f.envelope.approval.Plan.StatePath)
		if err != nil {
			t.Fatal(err)
		}
		var record repairProcessRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		if err := record.validate(f.envelope, f.key); err != nil || record.Observations != 1 || !record.StartAt.IsZero() || record.Generation != nil {
			t.Fatal("descriptor refusal reset a reservation or acquired a start", operation, err, record)
		}
		status, complete, err = f.resume()
		if err != nil || !complete || status != "resumed-generation-observed" || f.base.starts != 1 {
			t.Fatal("restored observation did not retain the original start allowance", operation, status, complete, err, f.base.starts)
		}
		if !reflect.DeepEqual(before, mainnetNamespaceTest(t, f.envelope.original.Plan.StateDirectory)) {
			t.Fatal("descriptor retry rewrote original journals or quota custody", operation)
		}
	}
}

// A returned contradictory protection mode is independent of a failed close.
// The availability wrapper cannot soften already observed authority loss.
func TestRepairOperatorResourceContradictionDominatesCloseFailure(t *testing.T) {
	f := newRepairOperatorFixture(t)
	path := filepath.Join(f.envelope.original.Plan.env("WARP_VAULT_HOME"), "main", "1.0.0", "st.yml")
	if err := os.Chmod(path, 0622); err != nil {
		t.Fatal(err)
	}
	faults := 0
	ctx := context.WithValue(f.ctx(), repairValidatorObservationKey{}, func(operation string) error {
		if operation == "operator-resource-close" {
			faults++
			return syscall.EIO
		}
		return nil
	})
	_, _, err := readRepairOperatorFile(ctx, f.base.host, path, f.base.host.rootUid, 2*1024*1024)
	disposition, cause := repairControllerCause(err)
	if faults != 1 || !errors.Is(err, syscall.EIO) || !errors.Is(err, durablevolume.ErrUnavailable) || !errors.Is(err, errRpcIntegrity) || disposition != "held" || cause != "integrity" {
		t.Fatal("close unavailability hid the observed resource contradiction", faults, err, disposition, cause)
	}
}
