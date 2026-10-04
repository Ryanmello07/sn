//go:build linux

// A complete economic restore derives native artifact ownership from the exact
// admitted checkpoint and signed policy. A caller cannot replace the cursor by
// supplying a directory census, or turn copied jobs into amount authority.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urnetwork/connect/durablevolume"
)

// Config approvals outside restored durable roots remain exact independently
// signed inputs. Files inside a copied artifact namespace are read from that
// copy; a missing declared root is never replaced by reading a live namespace.
func reviewEconomicNativeProducerRestore(ctx context.Context, policy economicEmissionPolicy, checkpoint monitorHistoryReference, storage *economicConservationStorageProfile, state monitorEconomicNativeState, roots []*monitorHistoryRestoreRootReview, declared map[string]durablevolume.StateRootSpec) (resultErr error) {
	if ctx == nil || policy.Execution == nil || policy.Execution.Producer == nil {
		return errors.New("native restore lacks original producer policy")
	}
	var selected *monitorHistoryRestoreRootReview
	for _, root := range roots {
		if _, found := storageNativeRestoreRelative(root.request.RootPath, policy.Execution.Directory); found {
			if selected != nil {
				return errors.New("native artifact restore has overlapping original roots")
			}
			selected = root
		}
	}
	if selected == nil {
		return errors.New("economic restore omits original native artifact root")
	}
	scope := storageNativeProducerScope{Schema: storageNativeProducerSchema, Checkpoint: checkpoint, CheckpointStorage: storage, Policy: policy, Cursor: state.Cursor, State: state.ExecutionProducer}
	for _, reference := range append([]planFileReference{policy.Execution.Producer.Authority}, policy.Execution.Producer.Renewals...) {
		var approvalRoot *monitorHistoryRestoreRootReview
		var member durablevolume.PreparationFile
		for _, root := range roots {
			if path, found := storageNativeRestoreRelative(root.request.RootPath, reference.Path); found {
				if approvalRoot != nil {
					return errors.New("native approval has overlapping original roots")
				}
				entry, present := root.entries[path]
				if !present || entry.Kind != "file" || entry.Size == 0 || entry.Size > nativeProducerAuthorityLimit || entry.Sha256 != reference.Sha256 {
					return errors.New("native restore omitted original signed approval member")
				}
				approvalRoot = root
				member = durablevolume.PreparationFile{Path: path, Kind: entry.Kind, Mode: entry.Mode, Bytes: entry.Size, Sha256: entry.Sha256}
			}
		}
		var raw []byte
		var err error
		if approvalRoot != nil {
			raw, err = readNativeProducerRestoreSource(ctx, approvalRoot, member)
		} else {
			for path := range declared {
				if _, found := storageNativeRestoreRelative(path, reference.Path); found {
					return errors.New("economic restore omits original native approval root")
				}
			}
			raw, err = nativeProducerReadApproval(ctx, reference)
		}
		if err != nil {
			return err
		}
		scope.Approvals = append(scope.Approvals, raw)
	}
	prefix, err := filepath.Rel(selected.request.RootPath, policy.Execution.Directory)
	if err != nil {
		return err
	}
	shared := map[string]bool{}
	for _, snapshot := range selected.nested {
		for path := filepath.Dir(snapshot); path != "."; path = filepath.Dir(path) {
			if strings.HasPrefix(prefix, path+"/") {
				shared[path] = true
			}
		}
	}
	for path := range shared {
		scope.SharedDirectories = append(scope.SharedDirectories, path)
	}
	sort.Strings(scope.SharedDirectories)
	inputs, err := json.Marshal(scope)
	if err != nil {
		return err
	}
	if len(inputs) > maxRpcReplyBytes {
		return errors.New("native restore original approval and checkpoint inputs exceed reviewed request capacity")
	}
	owner := durablevolume.PreparationOwner{Kind: storageNativeProducerKind, RelativePath: ".", Purpose: "restore", RestoreCoverage: durablevolume.PreparationCompleteUnion, Inputs: inputs}
	for _, previous := range selected.request.Owners {
		if previous.Kind == storageNativeProducerKind {
			return errors.New("economic restore repeats native artifact ownership")
		}
	}
	plan, err := planStorageNativeProducerRestore(ctx, "native-artifact-source-review", owner, selected.inventory, false)
	if err != nil {
		return err
	}
	authorities, err := storageNativeProducerAuthorities(ctx, scope)
	if err != nil {
		return err
	}
	directory, err := openMonitorDirectory(selected.request.RestoreSource.Directory, nil)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.check(), directory.close()) }()
	if err := validateStorageNativeProducerHistory(ctx, scope, authorities, prefix, plan.Files, func(member durablevolume.PreparationFile) ([]byte, error) {
		if err := directory.check(); err != nil {
			return nil, err
		}
		raw, err := readStorageNativeProducerMember(ctx, directory.file, member)
		return raw, errors.Join(err, directory.check())
	}); err != nil {
		return err
	}
	selected.request.Owners = append(selected.request.Owners, owner)
	artifactDirectories := map[string]bool{}
	for _, artifact := range plan.Files {
		if artifact.Kind == "directory" {
			artifactDirectories[artifact.Path] = true
		}
	}
	for _, root := range roots {
		members := storageNativeApprovalMembers(scope, root.request.RootPath)
		if len(members) == 0 {
			continue
		}
		approvalScope := storageNativeApprovalScope{Native: scope}
		shared := map[string]bool{}
		for member := range members {
			for parent := filepath.Dir(member); parent != "."; parent = filepath.Dir(parent) {
				for _, snapshot := range root.nested {
					if strings.HasPrefix(snapshot, parent+"/") {
						shared[parent] = true
					}
				}
				if root == selected {
					if artifactDirectories[parent] {
						shared[parent] = true
					}
				}
			}
		}
		for path := range shared {
			approvalScope.SharedDirectories = append(approvalScope.SharedDirectories, path)
		}
		sort.Strings(approvalScope.SharedDirectories)
		inputs, err := json.Marshal(approvalScope)
		if err != nil {
			return err
		}
		approvalOwner := durablevolume.PreparationOwner{Kind: storageNativeApprovalKind, RelativePath: ".", Purpose: "restore", RestoreCoverage: durablevolume.PreparationCompleteUnion, Inputs: inputs}
		for _, existing := range root.request.Owners {
			if existing.Kind == storageNativeApprovalKind {
				return errors.New("economic restore repeats original approval ownership")
			}
		}
		if _, err := planStorageNativeApprovalRestore(ctx, "native-approval-source-review", approvalOwner, root.inventory, false); err != nil {
			return err
		}
		root.request.Owners = append(root.request.Owners, approvalOwner)
	}
	return ctx.Err()
}

// Copied-source readback uses the same retained inode/byte checks as target
// inspection. Read or close failures never become fabricated digest conflicts.
func readNativeProducerRestoreSource(ctx context.Context, root *monitorHistoryRestoreRootReview, member durablevolume.PreparationFile) (raw []byte, resultErr error) {
	directory, err := openMonitorDirectory(root.request.RestoreSource.Directory, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, directory.check(), directory.close())
		if resultErr != nil {
			raw = nil
		}
	}()
	return readStorageNativeProducerMember(ctx, directory.file, member)
}
