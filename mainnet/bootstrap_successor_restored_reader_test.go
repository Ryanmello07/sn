//go:build linux

// Passive restored readers hold exact metadata descriptors; unchanged checks
// read no member payloads, and any lost physical generation stays refused.
package main

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/urfoundation/sn/internal/durablehead"
	"github.com/urnetwork/connect/durablevolume"
	"golang.org/x/sys/unix"
)

// All target files and attributes are produced by the actual public restore.
// Only the private passive capability is supplied directly in these controls.
func bootstrapSuccessorRestoredViewFixture(t *testing.T) (*bootstrapSuccessorMembers, *storageMemberRestoreFixture, durablevolume.PreparationPlan) {
	t.Helper()
	f := newStorageMemberOuterFixture(t, false, "exchange")
	path, hash := storagePreparationFreezeOwnerPlan(t, f.storage.target, "storage-prepare")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan durablevolume.PreparationPlan
	if err := decodePlanJson(raw, &plan); err != nil {
		t.Fatal(err)
	}
	ctx := storageMemberRestoreApply(t, f, path, hash)
	var request durablevolume.PreparationRequest
	if err := decodePlanJson(plan.RequestBytes, &request); err != nil || request.RestoreSource == nil {
		t.Fatal("missing original restore authority", err)
	}
	inventory, err := durablevolume.LoadPhysicalInventory(ctx, request.RestoreSource.Inventory)
	if err != nil {
		t.Fatal(err)
	}
	original, err := planStoragePreparationRestore(ctx, plan.Owners[0].StagingName, plan.Owners[0].Owner, inventory, false)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := buildBootstrapSuccessorRebindCensus(ctx, plan, 0, original, inventory)
	if err != nil || !expected.OuterPending || expected.Census.Pending == nil {
		t.Fatal("actual paired restore lost its original next census", err)
	}
	storage, err := openMainnetDurableDirectory(ctx, f.storage.target.root, durablevolume.ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := storage.close(); err != nil {
			t.Error(err)
		}
	})
	file := storage.directory.File()
	if err := mainnetDurableFlock(int(file.Fd()), unix.LOCK_SH|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	view := &bootstrapSuccessorRestoredMemberView{owner: plan.Owners[0], inventory: inventory, census: expected.Census}
	owner, err := openBootstrapSuccessorRestoredMembers(storage, file, view)
	if err != nil {
		t.Fatal("exact passive paired view was unavailable", err)
	}
	t.Cleanup(func() {
		if err := owner.close(); err != nil {
			t.Error(err)
		}
	})
	return owner, f, plan
}

// Real payload reads are counted, not elapsed time. Existing authenticated
// member observations remain valid only under their exact named stat tuple.
func TestBootstrapSuccessorRestoredReaderReusesUnchangedMembers(t *testing.T) {
	owner, f, _ := bootstrapSuccessorRestoredViewFixture(t)
	before := bootstrapSuccessorPreparationTestFiles(t, f.storage.target.root)
	readBytes := 0
	owner.afterRead = func(_ string, n int) { readBytes += n }
	for repeat := 0; repeat < 8; repeat++ {
		if err := owner.check(); err != nil {
			t.Fatal("unchanged passive owner lost custody", repeat, err)
		}
	}
	if readBytes != 0 {
		t.Fatal("unchanged passive checks reread original member payloads", readBytes)
	}
	if err := owner.publish(owner.census); !errors.Is(err, durablehead.ErrReadOnly) {
		t.Fatal("passive restored view acquired snapshot writer authority", err)
	}
	if err := owner.resumePublished(owner.census.Pending.Name, f.raw); !errors.Is(err, durablehead.ErrReadOnly) {
		t.Fatal("passive restored view attempted a completion sync", err)
	}
	if !maps.Equal(before, bootstrapSuccessorPreparationTestFiles(t, f.storage.target.root)) {
		t.Fatal("passive checks or refused publication mutated original members")
	}
	name := owner.census.Members[0].Name
	path := filepath.Join(f.storage.target.root, name)
	changed := []byte(before[name])
	changed[0] ^= 1
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := owner.check(); !errors.Is(err, durablevolume.ErrIdentity) || readBytes != len(changed) {
		t.Fatal("one changed member did not receive exactly one full admission read", err, readBytes, len(changed))
	}
	if err := os.WriteFile(path, []byte(before[name]), 0600); err != nil {
		t.Fatal(err)
	}
	if err := owner.check(); !errors.Is(err, durablevolume.ErrIdentity) || readBytes != len(changed) {
		t.Fatal("restoring bytes silently reenrolled lost passive custody", err, readBytes)
	}
}

// An exact byte copy on another inode and restoration of an old checkpoint
// cannot heal confirmed loss within an already admitted read-only owner.
func TestBootstrapSuccessorRestoredReaderRetainsMetadataLoss(t *testing.T) {
	for _, fault := range []string{"named-image", "checkpoint"} {
		owner, f, _ := bootstrapSuccessorRestoredViewFixture(t)
		head := owner.restoredHead
		if head == nil || len(head.metadata) != 2 {
			t.Fatal("fixture lacks the original paired physical view")
		}
		held := filepath.Join(f.storage.target.metadata, "held-restored-image")
		path := filepath.Join(f.storage.target.root, head.metadata[0].name)
		if fault == "named-image" {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(path, held); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
		} else if err := unix.Fremovexattr(int(head.root.Fd()), head.attribute); err != nil {
			t.Fatal(err)
		}
		if err := owner.check(); !errors.Is(err, durablevolume.ErrIdentity) {
			t.Fatal("passive metadata custody loss was not permanent", fault, err)
		}
		if fault == "named-image" {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(held, path); err != nil {
				t.Fatal(err)
			}
		} else if err := unix.Fsetxattr(int(head.root.Fd()), head.attribute, head.raw, unix.XATTR_CREATE); err != nil {
			t.Fatal(err)
		}
		if err := owner.check(); !errors.Is(err, durablevolume.ErrIdentity) {
			t.Fatal("physical restoration reenrolled an invalidated passive owner", fault, err)
		}
		if err := owner.close(); err != nil {
			t.Fatal(err)
		}
		if err := owner.storage.close(); err != nil {
			t.Fatal(err)
		}
	}
}
