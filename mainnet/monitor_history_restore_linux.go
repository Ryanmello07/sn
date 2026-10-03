//go:build linux

// Fixed snapshot owners are selected by exact original history references.
// This shared source review never infers ownership from a filename pattern.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/urfoundation/sn/internal/durablehead"
	"github.com/urnetwork/connect/durablevolume"
)

// This in-memory review borrows immutable original inventories and byte refs.
type monitorHistoryRestoreRootReview struct {
	request   durablevolume.PreparationRequest
	inventory durablevolume.Inventory
	entries   map[string]durablevolume.InventoryEntry
	seen      map[string]bool
}

// An additional co-owner has exact explicit coverage; unknown files never get
// assigned by suffix or erased to make a single-role history appear complete.
func newMonitorHistoryRestoreRootReview(ctx context.Context, request durablevolume.PreparationRequest, declared map[string]durablevolume.StateRootSpec) (*monitorHistoryRestoreRootReview, error) {
	if request.Purpose != "restore" || request.Scope != "daemon" || request.RestoreSource == nil {
		return nil, errors.New("monitor history requires explicit daemon restore requests")
	}
	report, err := durablevolume.LoadPhysicalInventory(ctx, request.RestoreSource.Inventory)
	if err != nil {
		return nil, err
	}
	if report.StateRoot.Path != request.RootPath || declared[request.RootPath] != report.StateRoot || !monitorHistoryPath(request.RestoreSource.Directory) {
		return nil, errors.New("monitor history changed an original declared logical root")
	}
	self := &monitorHistoryRestoreRootReview{request: request, inventory: report, entries: map[string]durablevolume.InventoryEntry{}, seen: map[string]bool{}}
	self.request.Owners = append([]durablevolume.PreparationOwner(nil), request.Owners...)
	for _, entry := range report.Entries {
		if _, present := self.entries[entry.Path]; present {
			return nil, errors.New("monitor history inventory repeats an original member")
		}
		self.entries[entry.Path] = entry
	}
	for _, owner := range request.Owners {
		if owner.RestoreCoverage != durablevolume.PreparationCompleteUnion || owner.Purpose != "restore" {
			return nil, errors.New("monitor history additional owner lacks complete retained union coverage")
		}
		if owner.Kind == "mainnet-monitor-checkpoint" {
			_, scope, err := storagePreparationSnapshotSpec(false, owner)
			if err != nil || self.seen[scope.Name] {
				return nil, errors.Join(errors.New("monitor history repeats an additional snapshot"), err)
			}
			self.seen[scope.Name] = true
		}
	}
	return self, nil
}

// Only the exact signed-history reference selects a new fixed snapshot owner.
func (self *monitorHistoryRestoreRootReview) read(ctx context.Context, reference monitorHistoryReference) ([]byte, error) {
	if err := reference.validate(); err != nil {
		return nil, err
	}
	if filepath.Dir(reference.Path) != self.request.RootPath {
		return nil, errors.New("monitor history reference changed its original root")
	}
	name := filepath.Base(reference.Path)
	entry, present := self.entries[name]
	if !present || entry.Kind != "file" || entry.Size != reference.Bytes || entry.Sha256 != reference.Sha256 || self.seen[name] {
		return nil, errors.New("monitor history inventory omits, repeats or changes an original member")
	}
	inputs, err := json.Marshal(storageSnapshotPreparationScope{Schema: "urnetwork-snapshot-preparation-v1", Name: name, MaximumBytes: maxRpcReplyBytes})
	if err != nil {
		return nil, err
	}
	owner := durablevolume.PreparationOwner{Kind: "mainnet-monitor-checkpoint", RelativePath: ".", Purpose: "restore", RestoreCoverage: durablevolume.PreparationCompleteUnion, Inputs: inputs}
	spec, _, err := storagePreparationSnapshotSpec(false, owner)
	if err != nil {
		return nil, err
	}
	if _, err := durablehead.PlanRestore(ctx, "monitor-history-review", owner, spec, inputs, self.inventory); err != nil {
		return nil, err
	}
	raw, err := readBootstrapChainInput(ctx, planFileReference{Path: filepath.Join(self.request.RestoreSource.Directory, name), Sha256: reference.Sha256}, maxRpcReplyBytes)
	if err != nil {
		return nil, fmt.Errorf("monitor history copied member read: %w", err)
	}
	if uint64(len(raw)) != reference.Bytes {
		return nil, errors.New("monitor history copied bytes differ from retained reference")
	}
	self.seen[name] = true
	self.request.Owners = append(self.request.Owners, owner)
	return raw, nil
}

// Read-only declaration admission keeps original root generations bound even
// while replacement targets exist at the same independently approved paths.
func monitorHistoryRestoreDeclaration(ctx context.Context) (durablevolume.Reference, map[string]durablevolume.StateRootSpec, error) {
	reference, present := durablevolume.ReferenceFromContext(ctx)
	if !present {
		return reference, nil, errors.New("monitor history restore declaration is absent")
	}
	declaration, err := durablevolume.Load(reference)
	if err != nil {
		return reference, nil, err
	}
	declared := map[string]durablevolume.StateRootSpec{}
	for _, volume := range declaration.Volumes {
		for _, root := range volume.StateRoots {
			declared[root.Path] = root
		}
	}
	return reference, declared, nil
}

// Counts, actual encoded bytes and two-times reserve remain independent. A
// larger owner count does not silently increase any per-root physical budget.
func validateMonitorHistoryRestoreCapacity(preparation durablevolume.PreparationRequest, report durablevolume.Inventory) error {
	// The complete inventory includes independently declared co-owners. Their
	// exact semantic coverage is enforced again by storage-prepare before effects.
	limits := preparation.Limits
	if limits.MaxEntries < 2*uint64(len(report.Entries)) || limits.MaxBytes < 2*report.TotalBytes ||
		limits.MaxOwnerAttributes < 2*report.TotalOwnerAttributes || limits.MaxOwnerAttributeBytes < 2*report.TotalOwnerAttributes*4096 ||
		preparation.MinAvailableBytes < 2*(report.TotalBytes+report.TotalOwnerAttributes*4096) || preparation.MinAvailableInodes < 2*uint64(len(report.Entries)) {
		return errors.New("monitor history restore requires explicit two-times complete-namespace byte, head and inode reserves")
	}
	if len(preparation.Owners) > 32 || limits.MaxOwnerAttributes > 128 || limits.MaxOwnerAttributeBytes > 128*4096 {
		if preparation.CapacityProfile != "urnetwork-preparation-many-owners-v1" {
			return errors.New("monitor history restore requires the explicit many-owner preparation capacity profile")
		}
	}
	encoded, err := json.Marshal(preparation)
	if err != nil || len(encoded)+1 > maxRpcReplyBytes {
		return errors.Join(errors.New("monitor history restore request exceeds its complete serialized byte bound"), err)
	}
	return nil
}
