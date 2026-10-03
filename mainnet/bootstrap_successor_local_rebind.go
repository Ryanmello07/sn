// Local restoration needs a separate original-approver receipt. It preserves
// signed preparation/execution bytes while admitting one reviewed physical root.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/urnetwork/connect/durablevolume"
	"golang.org/x/sys/unix"
)

const bootstrapSuccessorLocalRebindSchema = "urnetwork-mainnet-successor-local-rebind-v1"
const bootstrapSuccessorLocalRebindEnvelopeSchema = "urnetwork-mainnet-successor-local-rebind-envelope-v1"
const bootstrapSuccessorLocalRebindFile = bootstrapSuccessorExecutionPrefix + "-local-rebind.json"

// The original signature domain, logical path and every nonce remain fixed.
// Only this separately signed receipt authorizes the new physical coordinate.
type bootstrapSuccessorLocalRebindPlan struct {
	Schema                   string                         `json:"schema"`
	ExecutionApprovalHash    string                         `json:"execution_approval_hash"`
	LocalDirectory           string                         `json:"local_directory"`
	OriginalLocal            bootstrapSuccessorRootIdentity `json:"original_local"`
	RestoredLocal            bootstrapSuccessorRootIdentity `json:"restored_local"`
	RestoredGeneration       string                         `json:"restored_generation_sha256"`
	RuntimeDeclaration       durablevolume.Reference        `json:"runtime_declaration"`
	RestorePlan              durablevolume.Reference        `json:"restore_plan"`
	OriginalInventory        durablevolume.Reference        `json:"original_inventory"`
	OriginalFormerWriter     durablevolume.Reference        `json:"original_former_writer_fence"`
	OriginalMemberCensusHash string                         `json:"original_member_census_sha256"`
	DerivedMemberCensusHash  string                         `json:"derived_member_census_sha256"`
}

type bootstrapSuccessorLocalRebindApproval struct {
	Schema    string                            `json:"schema"`
	Plan      bootstrapSuccessorLocalRebindPlan `json:"plan"`
	Signature string                            `json:"signature_ed25519"`
}

// An unsigned preview can borrow only a passive reader. Writer admission is
// constructed separately after the original approver's signature is verified.
type bootstrapSuccessorLocalInspection struct {
	preparationHash string
	physical        bootstrapSuccessorRootIdentity
	writer          bool
}

func (self bootstrapSuccessorLocalRebindPlan) signingBytes(original bootstrapSuccessorExecutionApproval, profile *safeExecutionProfile) ([]byte, error) {
	if err := original.validate(original.Plan, profile); err != nil {
		return nil, err
	}
	preparation := original.Plan.Review.Preparation.Approval.Plan
	if self.Schema != bootstrapSuccessorLocalRebindSchema || self.ExecutionApprovalHash != rootObjectHash(original) ||
		self.LocalDirectory != preparation.Proposal.OriginalRunDirectory || self.OriginalLocal != preparation.Root ||
		self.RestoredLocal.Inode == 0 || self.RestoredLocal == self.OriginalLocal || !planSha256(self.RestoredGeneration) ||
		!planSha256(self.OriginalMemberCensusHash) || !planSha256(self.DerivedMemberCensusHash) {
		return nil, errors.New("successor local rebind changes original scope or lacks exact restored custody")
	}
	for _, reference := range []durablevolume.Reference{self.RuntimeDeclaration, self.RestorePlan, self.OriginalInventory, self.OriginalFormerWriter} {
		if !bootstrapRootAbsolutePath(reference.Path) || !planSha256(reference.Sha256) || reference.Path == self.LocalDirectory || strings.HasPrefix(reference.Path, self.LocalDirectory+"/") {
			return nil, errors.New("successor local rebind reference is absent, unpinned or overlaps original custody")
		}
	}
	raw, err := json.Marshal(self)
	if err != nil || len(raw) > maximumBootstrapSuccessorRegistryRebindBytes {
		return nil, errors.Join(errors.New("successor local rebind exceeds its finite approval bound"), err)
	}
	return append([]byte(bootstrapSuccessorLocalRebindSchema+"\x00"), raw...), nil
}

// This initial profile covers a settled local member census and every co-owned
// fixed snapshot, all at their original logical path. It never repairs heads.
func buildBootstrapSuccessorLocalRebind(ctx context.Context, original bootstrapSuccessorExecutionApproval, profile *safeExecutionProfile, reference durablevolume.Reference) (result bootstrapSuccessorLocalRebindPlan, resultErr error) {
	if ctx == nil || ctx.Err() != nil {
		return result, errors.New("successor local rebind requires an active inspection context")
	}
	if err := original.validate(original.Plan, profile); err != nil {
		return result, err
	}
	declarationReference, found := durablevolume.ReferenceFromContext(ctx)
	if !found {
		return result, errors.New("successor local rebind requires its explicit runtime declaration")
	}
	raw, hash, err := readBootstrapRootFile(ctx, reference.Path, 32*1024*1024)
	if err != nil || hash != reference.Sha256 {
		return result, errors.Join(errors.New("successor local restore plan pin differs"), err)
	}
	var plan durablevolume.PreparationPlan
	if err := decodePlanJson(raw, &plan); err != nil {
		return result, err
	}
	var request durablevolume.PreparationRequest
	if err := decodePlanJson(plan.RequestBytes, &request); err != nil {
		return result, err
	}
	p := original.Plan.Review.Preparation.Approval.Plan
	if plan.Schema != durablevolume.PreparationPlanSchema || plan.RestartAuthorized || request.Purpose != "restore" || request.Scope != "daemon" || request.RestoreSource == nil ||
		plan.RequestSha256 != plan.Request.Sha256 || safeReleaseHash(plan.RequestBytes) != plan.RequestSha256 || request.RootPath != p.Proposal.OriginalRunDirectory ||
		len(plan.Owners) < 2 || len(plan.Owners) > 32 || len(plan.Owners) != len(request.Owners) || len(plan.Derivations) != 1 || len(plan.Generation) != durablevolume.RootGenerationBytes || len(plan.Lease) != 32 {
		return result, errors.New("successor local rebind requires the complete original shared-owner restore")
	}
	inventory, err := durablevolume.LoadPhysicalInventory(ctx, request.RestoreSource.Inventory)
	if err != nil {
		return result, err
	}
	if inventory.PhysicalRoot.Inode != p.Root.Inode || unix.Mkdev(inventory.PhysicalRoot.Device.Major, inventory.PhysicalRoot.Device.Minor) != p.Root.Device || inventory.StateRoot.Path != request.RootPath || inventory.FormerWriterFence != request.RestoreSource.FormerWriterFence {
		return result, errors.New("successor local restore source is not its original approved generation")
	}
	fenceRaw, fenceHash, err := readBootstrapRootFile(ctx, inventory.FormerWriterFence.Path, 16*1024)
	var fence durablevolume.FormerWriterFence
	if err == nil {
		err = decodePlanJson(fenceRaw, &fence)
	}
	if err != nil || fenceHash != inventory.FormerWriterFence.Sha256 || fence.Schema != durablevolume.FormerWriterFenceSchema || !fence.FormerWritersStopped || strings.TrimSpace(fence.Evidence) == "" || fence.RootPath != inventory.StateRoot.Path || fence.DeclarationSha256 != inventory.Declaration.Sha256 || fence.LeaseSha256 != inventory.StateRoot.LeaseSha256 {
		return result, errors.Join(errors.New("successor local original former-writer assertion differs"), err)
	}
	memberIndex := -1
	originalOwners := make([]durablevolume.PreparationOwnerPlan, 0, len(plan.Owners))
	for index, owner := range plan.Owners {
		if !reflect.DeepEqual(owner.Owner, request.Owners[index]) || owner.Owner.RestoreCoverage != durablevolume.PreparationCompleteUnion {
			return result, errors.New("successor local restore omitted explicit complete owner coverage")
		}
		if owner.Owner.Kind == "mainnet-successor-local-members" {
			if memberIndex != -1 {
				return result, errors.New("successor local restore repeats its member owner")
			}
			memberIndex = index
		} else if _, _, err := storagePreparationSnapshotSpec(false, owner.Owner); err != nil {
			return result, errors.Join(errors.New("successor local restore contains an unsupported co-owner"), err)
		}
		expected, err := planStoragePreparationRestore(ctx, owner.StagingName, owner.Owner, inventory, false)
		if err != nil {
			return result, err
		}
		originalOwners = append(originalOwners, expected)
		if owner.Owner.Kind == "mainnet-successor-local-members" {
			derivation := plan.Derivations[0]
			if derivation.OwnerIndex != index || expected.PhysicalMetadata == nil || derivation.Original.File.Path != expected.PhysicalMetadata.Path || derivation.Derived.Path != expected.PhysicalMetadata.Path {
				return result, errors.New("successor local restore lost its exact census derivation")
			}
			expected.Files = append([]durablevolume.PreparationFile(nil), expected.Files...)
			found = false
			for fileIndex, file := range expected.Files {
				if file.Path == expected.PhysicalMetadata.Path {
					if file != derivation.Original.File {
						return result, errors.New("successor local derivation changed original census authority")
					}
					expected.Files[fileIndex], found = derivation.Derived, true
				}
			}
			if !found {
				return result, errors.New("successor local derivation omitted original census")
			}
		}
		if !reflect.DeepEqual(expected, owner) {
			return result, errors.New("successor local restore changed original owner bytes or capacity")
		}
	}
	if memberIndex == -1 {
		return result, errors.New("successor local restore lacks its immutable member census")
	}
	if err := validateBootstrapSuccessorLocalCoverage(inventory, originalOwners); err != nil {
		return result, err
	}
	derivation := plan.Derivations[0]
	memberOwner := plan.Owners[memberIndex]
	originalRaw, originalHash, err := readBootstrapRootFile(ctx, derivation.Original.Path, maximumBootstrapSuccessorMemberCensusBytes)
	if err != nil || originalHash != derivation.Original.File.Sha256 || uint64(len(originalRaw)) != derivation.Original.File.Bytes {
		return result, errors.Join(errors.New("successor local original census lineage is unavailable"), err)
	}
	files := map[string]durablevolume.PreparationFile{}
	memberFiles := map[string]bool{}
	for index, owner := range plan.Owners {
		for _, file := range owner.Files {
			if _, exists := files[file.Path]; exists {
				return result, errors.New("successor local restore overlaps target ownership")
			}
			files[file.Path] = file
			if index == memberIndex && file.Path != memberOwner.PhysicalMetadata.Path {
				memberFiles[file.Path] = true
			}
		}
	}
	var targets []durablevolume.PreparationSource
	for _, source := range plan.Sources {
		if expected, ok := files[source.File.Path]; !ok || expected != source.File {
			return result, errors.New("successor local restore lost or repeated a reviewed target source")
		}
		delete(files, source.File.Path)
		if memberFiles[source.File.Path] {
			targets = append(targets, source)
		}
	}
	if len(files) != 0 {
		return result, errors.New("successor local restore omitted a reviewed target source")
	}
	derived, err := rebindStoragePreparationMembersRestore(ctx, originalOwners[memberIndex], inventory, originalRaw, targets, false)
	if err != nil || uint64(len(derived)) != derivation.Derived.Bytes || safeReleaseHash(derived) != derivation.Derived.Sha256 {
		return result, errors.Join(errors.New("successor local derived census differs from its reviewed original"), err)
	}
	var derivedCensus bootstrapSuccessorMemberCensus
	if err := decodePlanJson(derived, &derivedCensus); err != nil {
		return result, err
	}
	if derivedCensus.Pending != nil {
		return result, errors.New("successor local rebind requires a settled original publication")
	}
	declaration, err := durablevolume.Load(declarationReference)
	if err != nil {
		return result, err
	}
	found = false
	for _, volume := range declaration.Volumes {
		for _, root := range volume.StateRoots {
			if root.Path == request.RootPath {
				if found || volume.FilesystemUuid != request.FilesystemUuid || volume.FilesystemType != request.FilesystemType || root.RootInode != plan.Root.Inode || root.GenerationSha256 != safeReleaseHash(plan.Generation) || root.LeasePath != request.LeasePath || root.LeaseSha256 != safeReleaseHash(plan.Lease) {
					return result, errors.New("successor local runtime declaration differs from exact restored custody")
				}
				found = true
			}
		}
	}
	physical, err := bootstrapSuccessorPhysicalRoot(request.RootPath)
	if err != nil || !found || physical != (bootstrapSuccessorRootIdentity{Device: plan.Root.Device, Inode: plan.Root.Inode}) {
		return result, errors.Join(errors.New("successor local restored physical generation differs"), err)
	}
	if err := inspectBootstrapSuccessorRebindTarget(ctx, request.RootPath, derivedCensus, false); err != nil {
		return result, err
	}
	if err := inspectBootstrapSuccessorLocalSnapshots(ctx, request.RootPath, plan.Owners, inventory); err != nil {
		return result, err
	}
	result = bootstrapSuccessorLocalRebindPlan{Schema: bootstrapSuccessorLocalRebindSchema, ExecutionApprovalHash: rootObjectHash(original), LocalDirectory: request.RootPath,
		OriginalLocal: p.Root, RestoredLocal: physical, RestoredGeneration: safeReleaseHash(plan.Generation), RuntimeDeclaration: declarationReference,
		RestorePlan: reference, OriginalInventory: request.RestoreSource.Inventory, OriginalFormerWriter: request.RestoreSource.FormerWriterFence,
		OriginalMemberCensusHash: derivation.Original.File.Sha256, DerivedMemberCensusHash: derivation.Derived.Sha256}
	_, err = result.signingBytes(original, profile)
	return result, errors.Join(err, ctx.Err())
}

// Original complete-union coverage is checked before any physical derivation;
// map deletion rejects overlaps instead of silently collapsing duplicate owners.
func validateBootstrapSuccessorLocalCoverage(report durablevolume.Inventory, owners []durablevolume.PreparationOwnerPlan) error {
	files := map[string]durablevolume.PreparationFile{}
	attributes := map[durablevolume.PreparationAttributeSpec]bool{}
	for _, entry := range report.Entries {
		if entry.Path != "" {
			if _, found := files[entry.Path]; found {
				return errors.New("successor local coverage repeats original member")
			}
			files[entry.Path] = durablevolume.PreparationFile{Path: entry.Path, Kind: entry.Kind, Mode: entry.Mode, Bytes: entry.Size, Sha256: entry.Sha256}
		}
		for _, attribute := range entry.OwnerAttributes {
			if entry.Path == "" && attribute.Name == durablevolume.PreparationAttribute {
				continue
			}
			path := entry.Path
			if path == "" {
				path = "."
			}
			key := durablevolume.PreparationAttributeSpec{Path: path, Name: attribute.Name}
			if attributes[key] {
				return errors.New("successor local coverage repeats original checkpoint")
			}
			attributes[key] = true
		}
	}
	for _, owner := range owners {
		if owner.ExclusiveRoot || owner.Owner.RelativePath != "." || owner.Owner.Purpose != "restore" || owner.Owner.RestoreCoverage != durablevolume.PreparationCompleteUnion {
			return errors.New("successor local coverage changed its original shared scope")
		}
		for _, file := range owner.Files {
			if expected, ok := files[file.Path]; !ok || expected != file {
				return errors.New("successor local coverage overlaps or changes original bytes")
			}
			delete(files, file.Path)
		}
		for _, attribute := range owner.Attributes {
			if !attributes[attribute] {
				return errors.New("successor local coverage overlaps or invents original checkpoint")
			}
			delete(attributes, attribute)
		}
	}
	if len(files) != 0 || len(attributes) != 0 {
		return errors.New("successor local coverage omits original members or checkpoints")
	}
	return nil
}

// Every co-owner must still have its original bytes and exact derived physical
// head. Shared marker locks prevent cooperating snapshot writers during review.
func inspectBootstrapSuccessorLocalSnapshots(ctx context.Context, path string, owners []durablevolume.PreparationOwnerPlan, inventory durablevolume.Inventory) (resultErr error) {
	storage, err := openMainnetDurableDirectory(ctx, path, durablevolume.ReadOnly)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, storage.close()) }()
	root := storage.directory.File()
	if err := mainnetDurableFlock(int(root.Fd()), unix.LOCK_SH); err != nil {
		return err
	}
	for _, owner := range owners {
		if owner.Owner.Kind == "mainnet-successor-local-members" {
			continue
		}
		spec, _, err := storagePreparationSnapshotSpec(false, owner.Owner)
		if err != nil || spec.LockName == "" {
			return errors.Join(errors.New("successor local co-owner lacks an independent marker"), err)
		}
		if err := func() (resultErr error) {
			fd, err := unix.Openat(int(root.Fd()), spec.LockName, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}
			file := os.NewFile(uintptr(fd), filepath.Join(path, spec.LockName))
			defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
			if err := mainnetDurableFlock(fd, unix.LOCK_SH|unix.LOCK_NB); err != nil {
				return err
			}
			attributes, err := inspectStoragePreparationRestore(ctx, root, owner, inventory, false)
			if err != nil || len(attributes) != 1 || attributes[0].Spec.Path != spec.LockName {
				return errors.Join(errors.New("successor local co-owner physical head differs"), err)
			}
			raw := make([]byte, 4097)
			n, err := unix.Fgetxattr(fd, attributes[0].Spec.Name, raw)
			if err != nil || n < 0 || n > 4096 || !bytes.Equal(raw[:n], attributes[0].Raw) {
				return errors.Join(errors.New("successor local co-owner head was lost or changed"), err)
			}
			var opened, named unix.Stat_t
			if err := unix.Fstat(fd, &opened); err != nil {
				return err
			}
			if err := unix.Fstatat(int(root.Fd()), spec.LockName, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return err
			}
			if opened.Dev != named.Dev || opened.Ino != named.Ino {
				return errors.New("successor local co-owner marker changed during inspection")
			}
			return storage.check(root)
		}(); err != nil {
			return err
		}
	}
	return storage.check(root)
}

// A valid independent signature grants only the separately checked new root.
func (self bootstrapSuccessorLocalRebindApproval) validate(ctx context.Context, original bootstrapSuccessorExecutionApproval, profile *safeExecutionProfile) error {
	message, err := self.Plan.signingBytes(original, profile)
	key, keyErr := rootReceiptHex(original.Plan.Review.Preparation.Approval.Plan.ApprovalPublicKey, ed25519.PublicKeySize)
	signature, signatureErr := rootOfflineSignatureBytes(self.Signature)
	if self.Schema != bootstrapSuccessorLocalRebindEnvelopeSchema || err != nil || keyErr != nil || signatureErr != nil || !ed25519.Verify(key, message, signature) {
		return errors.Join(errors.New("successor local independent rebind approval is invalid"), err, keyErr, signatureErr)
	}
	expected, err := buildBootstrapSuccessorLocalRebind(ctx, original, profile, self.Plan.RestorePlan)
	if err != nil || expected != self.Plan {
		return errors.Join(errors.New("successor local approval differs from exact restored lineage"), err)
	}
	return nil
}

// The constructor calls this only after original claim, nonce and history checks.
func (self *bootstrapSuccessorExecutionStore) retainLocalRebind() error {
	if self.localRebind == nil {
		return nil
	}
	raw, err := json.Marshal(self.localRebind)
	if err != nil {
		return err
	}
	return self.local.publish(bootstrapSuccessorLocalRebindFile, "local-rebind", raw)
}

// Review output contains no inferred runtime start, transaction or signer grant.
func (self bootstrapSuccessorLocalRebindPlan) preview(original bootstrapSuccessorExecutionApproval, profile *safeExecutionProfile) (any, error) {
	message, err := self.signingBytes(original, profile)
	if err != nil {
		return nil, err
	}
	return struct {
		Schema       string                            `json:"schema"`
		Plan         bootstrapSuccessorLocalRebindPlan `json:"plan"`
		SigningBytes string                            `json:"signing_bytes"`
	}{Schema: "urnetwork-mainnet-successor-local-rebind-preview-v1", Plan: self, SigningBytes: "0x" + hex.EncodeToString(message)}, nil
}
