//go:build linux

// Restore request construction authenticates the whole retained native history
// before deriving its fixed snapshot owner census. Publication still requires
// the separate reviewed storage-prepare plan/apply; this command is read-only.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"reflect"

	"github.com/urfoundation/sn/internal/durablehead"
	"github.com/urfoundation/sn/internal/durablepath"
	"github.com/urnetwork/connect/durablevolume"
)

const monitorNativeRestoreRequestSchema = "urnetwork-native-monitor-restore-request-v1"

// Preparation retains all independent target choices and additional co-owners.
// Native snapshots are derived from original references, never a filename glob.
type monitorNativeRestoreRequest struct {
	Schema      string                           `json:"schema"`
	Expected    identityExpectation              `json:"expected"`
	Policy      monitorEconomicNativePolicy      `json:"policy"`
	Original    monitorHistoryReference          `json:"original"`
	Preparation durablevolume.PreparationRequest `json:"preparation"`
}

// The original logical root is retained: capacity approvals and segment paths
// cannot be silently rewritten to fit a different deployment namespace.
func buildMonitorNativeRestoreRequest(ctx context.Context, request monitorNativeRestoreRequest) (result durablevolume.PreparationRequest, resultErr error) {
	if err := durablepath.Require(ctx); err != nil {
		return result, err
	}
	if request.Schema != monitorNativeRestoreRequestSchema || request.Preparation.Purpose != "restore" || request.Preparation.Scope != "daemon" || request.Preparation.RestoreSource == nil {
		return result, errors.New("native history restore requires an explicit daemon restore request")
	}
	if err := errors.Join(request.Policy.validate(request.Expected), request.Original.validate()); err != nil {
		return result, err
	}
	preparation := request.Preparation
	report, err := durablevolume.LoadPhysicalInventory(ctx, preparation.RestoreSource.Inventory)
	if err != nil {
		return result, err
	}
	if report.StateRoot.Path != preparation.RootPath || filepath.Dir(request.Original.Path) != report.StateRoot.Path || !monitorHistoryPath(preparation.RestoreSource.Directory) {
		return result, errors.New("native history restore must retain its original logical root")
	}
	entries := make(map[string]durablevolume.InventoryEntry, len(report.Entries))
	for _, entry := range report.Entries {
		entries[entry.Path] = entry
	}
	owners := append([]durablevolume.PreparationOwner(nil), preparation.Owners...)
	seen := map[string]bool{}
	for _, owner := range owners {
		if owner.RestoreCoverage != "complete-union-v1" || owner.Purpose != "restore" {
			return result, errors.New("native history additional owners require complete retained union coverage")
		}
		if owner.Kind == "mainnet-monitor-checkpoint" {
			_, scope, err := storagePreparationSnapshotSpec(false, owner)
			if err != nil || seen[scope.Name] {
				return result, errors.Join(errors.New("native history repeats an additional snapshot owner"), err)
			}
			seen[scope.Name] = true
		}
	}
	read := func(reference monitorHistoryReference) ([]byte, error) {
		if err := reference.validate(); err != nil {
			return nil, err
		}
		if filepath.Dir(reference.Path) != report.StateRoot.Path {
			return nil, errors.New("native history crosses a root requiring its own reviewed restore union")
		}
		name := filepath.Base(reference.Path)
		entry, present := entries[name]
		if !present || entry.Kind != "file" || entry.Size != reference.Bytes || entry.Sha256 != reference.Sha256 || seen[name] {
			return nil, errors.New("native history inventory omits, repeats or changes an original member")
		}
		inputs, err := json.Marshal(storageSnapshotPreparationScope{Schema: "urnetwork-snapshot-preparation-v1", Name: name, MaximumBytes: maxRpcReplyBytes})
		if err != nil {
			return nil, err
		}
		owner := durablevolume.PreparationOwner{Kind: "mainnet-monitor-checkpoint", RelativePath: ".", Purpose: "restore", RestoreCoverage: "complete-union-v1", Inputs: inputs}
		spec, _, err := storagePreparationSnapshotSpec(false, owner)
		if err != nil {
			return nil, err
		}
		if _, err := durablehead.PlanRestore(ctx, "native-history-review", owner, spec, inputs, report); err != nil {
			return nil, err
		}
		raw, err := readBootstrapChainInput(ctx, planFileReference{Path: filepath.Join(preparation.RestoreSource.Directory, name), Sha256: reference.Sha256}, maxRpcReplyBytes)
		if err != nil || uint64(len(raw)) != reference.Bytes {
			return nil, errors.Join(errors.New("native history copied bytes differ from the original reference"), err)
		}
		seen[name] = true
		owners = append(owners, owner)
		return raw, nil
	}
	if err := validateMonitorNativeRestoreHistory(request.Policy, request.Original, read); err != nil {
		return result, err
	}
	preparation.Owners = owners
	if err := validateMonitorNativeRestoreCapacity(preparation, report); err != nil {
		return result, err
	}
	return preparation, ctx.Err()
}

// Each source inventory can live under a different root. The original signed
// paths and exact predecessor summaries, not root placement, bind the history.
func validateMonitorNativeRestoreHistory(policy monitorEconomicNativePolicy, original monitorHistoryReference, read func(monitorHistoryReference) ([]byte, error)) error {
	raw, err := read(original)
	if err != nil {
		return err
	}
	record, err := decodeMonitorEconomicNativeCheckpoint(raw, policy)
	if err != nil {
		return err
	}
	canonical, err := encodeMonitorNativeCheckpoint(record)
	if err != nil || !bytes.Equal(canonical, raw) {
		return errors.Join(errors.New("native restore requires the exact original checkpoint frame"), err)
	}
	if err := record.State.Catalog.checkPath(original.Path); err != nil {
		return err
	}
	var prior *monitorEconomicNativeArchive
	if record.State.Archive != nil {
		for _, reference := range record.State.Archive.Segments {
			raw, err := read(reference)
			if err != nil {
				return err
			}
			segment, err := decodeMonitorEconomicNativeCheckpoint(raw, policy)
			if err != nil {
				return err
			}
			if !record.State.Catalog.retains(segment.State.Catalog) || !reflect.DeepEqual(segment.State.Archive, prior) {
				return errors.New("native restore discarded a signed revision or an original archive predecessor")
			}
			next, err := compactMonitorEconomicNative(segment, reference, policy)
			if err != nil {
				return err
			}
			prior = next.State.Archive
		}
	}
	if !reflect.DeepEqual(prior, record.State.Archive) {
		return errors.New("native restore summary differs from complete original history")
	}
	return nil
}

// Counts, actual encoded bytes and two-times reserve remain independent. A
// larger owner count does not silently increase any per-root physical budget.
func validateMonitorNativeRestoreCapacity(preparation durablevolume.PreparationRequest, report durablevolume.Inventory) error {
	// The complete inventory includes independently declared co-owners. Their
	// exact semantic coverage is enforced again by storage-prepare before effects.
	limits := preparation.Limits
	if limits.MaxEntries < 2*uint64(len(report.Entries)) || limits.MaxBytes < 2*report.TotalBytes ||
		limits.MaxOwnerAttributes < 2*report.TotalOwnerAttributes || limits.MaxOwnerAttributeBytes < 2*report.TotalOwnerAttributes*4096 ||
		preparation.MinAvailableBytes < 2*(report.TotalBytes+report.TotalOwnerAttributes*4096) || preparation.MinAvailableInodes < 2*uint64(len(report.Entries)) {
		return errors.New("native restore requires explicit two-times complete-namespace byte, head and inode reserves")
	}
	if len(preparation.Owners) > 32 || limits.MaxOwnerAttributes > 128 || limits.MaxOwnerAttributeBytes > 128*4096 {
		if preparation.CapacityProfile != "urnetwork-preparation-many-owners-v1" {
			return errors.New("native restore requires the explicit many-owner preparation capacity profile")
		}
	}
	encoded, err := json.Marshal(preparation)
	if err != nil || len(encoded)+1 > maxRpcReplyBytes {
		return errors.Join(errors.New("native restore request exceeds its complete serialized byte bound"), err)
	}
	return nil
}

// This emits a strict storage-prepare request for independent review. It does
// not create any target, change an approval, sign, reconcile or enable a service.
func runMonitorNativeArchiveRestoreRequest(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("monitor-native-archive restore-request", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("request", "", "exact retained native history and target request")
	hash := flags.String("request-sha256", "", "accepted request digest")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *path == "" || *hash == "" {
		fmt.Fprintln(stderr, "native restore requires an exact request and digest")
		return 2
	}
	raw, digest, err := readPlanFile(ctx, *path, maxRpcReplyBytes)
	if err != nil || digest != *hash {
		fmt.Fprintln(stderr, "native restore input differs:", err)
		return 2
	}
	var request monitorNativeRestoreRequest
	if err = decodeMonitorHistoryInput(raw, &request); err == nil {
		var result durablevolume.PreparationRequest
		result, err = buildMonitorNativeRestoreRequest(ctx, request)
		if err == nil {
			raw, err = json.Marshal(result)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "native restore:", err)
		return 2
	}
	raw = append(raw, '\n')
	n, err := stdout.Write(raw)
	if err != nil || n != len(raw) {
		fmt.Fprintln(stderr, "native restore request not delivered:", errors.Join(io.ErrShortWrite, err))
		return 2
	}
	return 0
}
