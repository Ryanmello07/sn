//go:build linux

// Native history may occupy several original roots. Planning authenticates its
// complete signed-path lineage, stages fixed owners and retains one reviewable
// cohort. Actual target mutations belong to storage-prepare cohort-apply.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/urfoundation/sn/internal/durablehead"
	"github.com/urfoundation/sn/internal/durablepath"
	"github.com/urnetwork/connect/durablevolume"
)

const monitorNativeRestoreCohortSchema = "urnetwork-native-monitor-restore-cohort-v1"

// Each request retains its independent source fence and target generation.
// Sorted logical paths, all count/byte dimensions and unchanged roots are bound
// before the command can produce a usable cohort reference.
type monitorNativeRestoreCohortRequest struct {
	Schema       string                                `json:"schema"`
	Expected     identityExpectation                   `json:"expected"`
	Policy       monitorEconomicNativePolicy           `json:"policy"`
	Original     monitorHistoryReference               `json:"original"`
	Limits       durablevolume.PreparationCohortLimits `json:"limits"`
	Preparations []durablevolume.PreparationRequest    `json:"preparations"`
}

// This in-memory review borrows immutable original inventories and byte refs.
type monitorNativeRestoreRootReview struct {
	request   durablevolume.PreparationRequest
	inventory durablevolume.Inventory
	entries   map[string]durablevolume.InventoryEntry
	seen      map[string]bool
}

// An additional co-owner has exact explicit coverage; unknown files never get
// assigned by suffix or erased to make a native-only history appear complete.
func newMonitorNativeRestoreRootReview(ctx context.Context, request durablevolume.PreparationRequest, declared map[string]durablevolume.StateRootSpec) (*monitorNativeRestoreRootReview, error) {
	if request.Purpose != "restore" || request.Scope != "daemon" || request.RestoreSource == nil {
		return nil, errors.New("native cohort requires explicit daemon restore requests")
	}
	report, err := durablevolume.LoadPhysicalInventory(ctx, request.RestoreSource.Inventory)
	if err != nil {
		return nil, err
	}
	if report.StateRoot.Path != request.RootPath || declared[request.RootPath] != report.StateRoot || !monitorHistoryPath(request.RestoreSource.Directory) {
		return nil, errors.New("native cohort changed an original declared logical root")
	}
	self := &monitorNativeRestoreRootReview{request: request, inventory: report, entries: map[string]durablevolume.InventoryEntry{}, seen: map[string]bool{}}
	self.request.Owners = append([]durablevolume.PreparationOwner(nil), request.Owners...)
	for _, entry := range report.Entries {
		if _, present := self.entries[entry.Path]; present {
			return nil, errors.New("native cohort inventory repeats an original member")
		}
		self.entries[entry.Path] = entry
	}
	for _, owner := range request.Owners {
		if owner.RestoreCoverage != durablevolume.PreparationCompleteUnion || owner.Purpose != "restore" {
			return nil, errors.New("native cohort additional owner lacks complete retained union coverage")
		}
		if owner.Kind == "mainnet-monitor-checkpoint" {
			_, scope, err := storagePreparationSnapshotSpec(false, owner)
			if err != nil || self.seen[scope.Name] {
				return nil, errors.Join(errors.New("native cohort repeats an additional snapshot"), err)
			}
			self.seen[scope.Name] = true
		}
	}
	return self, nil
}

// Only the exact signed-history reference selects a new fixed snapshot owner.
func (self *monitorNativeRestoreRootReview) read(ctx context.Context, reference monitorHistoryReference) ([]byte, error) {
	if err := reference.validate(); err != nil {
		return nil, err
	}
	if filepath.Dir(reference.Path) != self.request.RootPath {
		return nil, errors.New("native cohort reference changed its original root")
	}
	name := filepath.Base(reference.Path)
	entry, present := self.entries[name]
	if !present || entry.Kind != "file" || entry.Size != reference.Bytes || entry.Sha256 != reference.Sha256 || self.seen[name] {
		return nil, errors.New("native cohort inventory omits, repeats or changes an original member")
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
	if _, err := durablehead.PlanRestore(ctx, "native-cohort-review", owner, spec, inputs, self.inventory); err != nil {
		return nil, err
	}
	raw, err := readBootstrapChainInput(ctx, planFileReference{Path: filepath.Join(self.request.RestoreSource.Directory, name), Sha256: reference.Sha256}, maxRpcReplyBytes)
	if err != nil || uint64(len(raw)) != reference.Bytes {
		return nil, errors.Join(errors.New("native cohort copied bytes differ from retained reference"), err)
	}
	self.seen[name] = true
	self.request.Owners = append(self.request.Owners, owner)
	return raw, nil
}

// Read the complete source union first. Later staging may create public plan
// artifacts, but no target control/root/head changes until separate apply.
func buildMonitorNativeRestoreCohort(ctx context.Context, request monitorNativeRestoreCohortRequest) (durablevolume.PreparationCohortResult, error) {
	var empty durablevolume.PreparationCohortResult
	if err := durablepath.Require(ctx); err != nil {
		return empty, err
	}
	if request.Schema != monitorNativeRestoreCohortSchema || len(request.Preparations) < 2 || len(request.Preparations) > 256 || uint64(len(request.Preparations)) > request.Limits.MaxRoots {
		return empty, errors.New("native cohort schema or complete root capacity is invalid")
	}
	if err := errors.Join(request.Policy.validate(request.Expected), request.Original.validate()); err != nil {
		return empty, err
	}
	reference, _ := durablevolume.ReferenceFromContext(ctx)
	declaration, err := durablevolume.Load(reference)
	if err != nil {
		return empty, err
	}
	declared := map[string]durablevolume.StateRootSpec{}
	for _, volume := range declaration.Volumes {
		for _, root := range volume.StateRoots {
			declared[root.Path] = root
		}
	}
	byRoot := map[string]*monitorNativeRestoreRootReview{}
	roots := make([]*monitorNativeRestoreRootReview, 0, len(request.Preparations))
	for index, preparation := range request.Preparations {
		if index > 0 && request.Preparations[index-1].RootPath >= preparation.RootPath {
			return empty, errors.New("native cohort roots must be distinct and sorted")
		}
		root, err := newMonitorNativeRestoreRootReview(ctx, preparation, declared)
		if err != nil {
			return empty, err
		}
		roots = append(roots, root)
		byRoot[preparation.RootPath] = root
	}
	read := func(ref monitorHistoryReference) ([]byte, error) {
		root := byRoot[filepath.Dir(ref.Path)]
		if root == nil {
			return nil, errors.New("native cohort omits a root named by original history")
		}
		return root.read(ctx, ref)
	}
	if err := validateMonitorNativeRestoreHistory(request.Policy, request.Original, read); err != nil {
		return empty, err
	}
	for _, root := range roots {
		if err := validateMonitorNativeRestoreCapacity(root.request, root.inventory); err != nil {
			return empty, err
		}
	}
	cohort := durablevolume.PreparationCohort{Schema: durablevolume.PreparationCohortSchema, Scope: "daemon", RetainedDeclaration: &reference, Limits: request.Limits}
	adapter := storagePreparationAdapter(false)
	for _, root := range roots {
		requestReference, err := durablevolume.RetainPreparationRequest(ctx, root.request)
		if err != nil {
			return empty, err
		}
		plan, err := durablepath.PlanPreparation(ctx, requestReference, adapter)
		if err != nil {
			return empty, err
		}
		planReference, err := durablevolume.RetainPreparationPlan(ctx, plan)
		if err != nil {
			return empty, err
		}
		cohort.Plans = append(cohort.Plans, planReference)
	}
	cohortReference, err := durablevolume.RetainPreparationCohort(ctx, cohort, roots[0].request.StagingDirectory)
	if err != nil {
		return empty, err
	}
	return durablepath.CheckPreparationCohort(ctx, cohortReference, adapter)
}

// A complete reviewable file reference is the public output. Lost stdout leaves
// only bounded staging artifacts; it cannot accidentally activate a target.
func runMonitorNativeArchiveRestoreCohort(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("monitor-native-archive restore-cohort-plan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("request", "", "exact complete native-history cohort request")
	hash := flags.String("request-sha256", "", "reviewed request digest")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *path == "" || *hash == "" {
		fmt.Fprintln(stderr, "native cohort requires only its exact request and digest")
		return 2
	}
	raw, digest, err := readPlanFile(ctx, *path, 8*maxRpcReplyBytes)
	if err != nil || digest != *hash {
		fmt.Fprintln(stderr, "native cohort input differs:", err)
		return 2
	}
	var request monitorNativeRestoreCohortRequest
	var result durablevolume.PreparationCohortResult
	if err = decodeMonitorHistoryInput(raw, &request); err == nil {
		result, err = buildMonitorNativeRestoreCohort(ctx, request)
	}
	if err != nil {
		fmt.Fprintln(stderr, "native cohort:", err)
		return 2
	}
	raw, err = json.Marshal(result)
	if err != nil {
		fmt.Fprintln(stderr, "native cohort result:", err)
		return 2
	}
	raw = append(raw, '\n')
	n, err := stdout.Write(raw)
	if err != nil || n != len(raw) {
		fmt.Fprintln(stderr, "native cohort result was not delivered:", errors.Join(io.ErrShortWrite, err))
		return 2
	}
	return 0
}
