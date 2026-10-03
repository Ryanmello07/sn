//go:build linux

// Member restoration derives only unsigned physical coordinates. The original
// census remains in the reviewed plan; signed member and pending payload bytes
// never change, and existing execution approvals remain separately required.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/urfoundation/sn/internal/durablehead"
	"github.com/urnetwork/connect/durablevolume"
	"golang.org/x/sys/unix"
)

type storagePreparationMemberRestoreCensus struct {
	Schema             string          `json:"schema"`
	Profile            json.RawMessage `json:"profile"`
	OriginalCheckpoint []byte          `json:"original_checkpoint"`
}

// Legacy restoration owns the complete root. Explicit shared local custody
// selects its fixed namespace; an unfinished outer snapshot is still refused.
func planStoragePreparationMembersRestore(ctx context.Context, name string, owner durablevolume.PreparationOwner, report durablevolume.Inventory, ownerLocal bool) (durablevolume.PreparationOwnerPlan, error) {
	spec, profile, err := storagePreparationMembersProfile(owner, ownerLocal)
	if err != nil {
		return durablevolume.PreparationOwnerPlan{}, err
	}
	report, err = storagePreparationMembersRestoreView(ctx, owner, spec, report)
	if err != nil {
		return durablevolume.PreparationOwnerPlan{}, err
	}
	if ctx == nil || owner.Purpose != "restore" || name == "" || filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsRune(name, 0) || report.Schema != durablevolume.PhysicalInventorySchema || report.RestartAuthorized || len(report.Entries) == 0 || len(report.Entries) > maximumBootstrapSuccessorMemberCount+8 || report.Entries[0].Path != "" {
		return durablevolume.PreparationOwnerPlan{}, errors.New("member restore requires its bounded complete original physical inventory")
	}
	attribute := durablevolume.PreparationAttributeSpec{Path: ".", Name: durablehead.Attribute(spec.Kind, spec.Name)}
	files := []durablevolume.PreparationFile{}
	entries := map[string]durablevolume.InventoryEntry{}
	inodes := map[uint64]bool{}
	var original []byte
	var total uint64
	for _, entry := range report.Entries {
		if err := ctx.Err(); err != nil {
			return durablevolume.PreparationOwnerPlan{}, err
		}
		if entry.Physical == nil || entry.Physical.Inode == 0 || entry.Physical.Device != report.PhysicalRoot.Device || inodes[entry.Physical.Inode] {
			return durablevolume.PreparationOwnerPlan{}, errors.New("member restore original generations are missing or aliased")
		}
		inodes[entry.Physical.Inode] = true
		if _, found := entries[entry.Path]; found {
			return durablevolume.PreparationOwnerPlan{}, errors.New("member restore original names are duplicated")
		}
		entries[entry.Path] = entry
		if entry.Path == "" {
			if entry.Kind != "directory" || entry.Mode != 0700 || *entry.Physical != report.PhysicalRoot {
				return durablevolume.PreparationOwnerPlan{}, errors.New("member restore original root differs")
			}
		} else {
			maximum := uint64(maximumBootstrapSuccessorExecutionBytes)
			if entry.Path == spec.Name {
				maximum = uint64(spec.MaximumBytes)
			}
			if entry.Kind != "file" || entry.Mode != 0600 || filepath.Base(entry.Path) != entry.Path || entry.Path == "." || entry.Path == ".." || len(entry.Path) > 255 || strings.ContainsRune(entry.Path, 0) || entry.Size > maximum || !planSha256(entry.Sha256) || entry.Path != spec.Name && !bootstrapSuccessorMemberOwns(spec, owner.Kind == "mainnet-successor-nonce-members", entry.Path) {
				return durablevolume.PreparationOwnerPlan{}, errors.New("member restore has an unknown name, structure or capacity")
			}
			if entry.Size > uint64(maximumBootstrapSuccessorMemberTotalBytes+maximumBootstrapSuccessorMemberCensusBytes)-total {
				return durablevolume.PreparationOwnerPlan{}, errors.New("member restore exceeds its finite retained byte capacity")
			}
			total += entry.Size
			files = append(files, durablevolume.PreparationFile{Path: entry.Path, Kind: entry.Kind, Mode: entry.Mode, Bytes: entry.Size, Sha256: entry.Sha256})
		}
		for _, item := range entry.OwnerAttributes {
			if entry.Path == "" && item.Name == durablevolume.PreparationAttribute {
				continue
			}
			if entry.Path != "" || item.Name != attribute.Name || original != nil || len(item.Value) == 0 || len(item.Value) > 4096 || safeReleaseHash(item.Value) != item.Sha256 {
				return durablevolume.PreparationOwnerPlan{}, errors.New("member restore cannot discard another owner head or change checkpoint bytes")
			}
			original = append([]byte(nil), item.Value...)
		}
	}
	if original == nil {
		return durablevolume.PreparationOwnerPlan{}, errors.Join(durablevolume.ErrIdentity, errors.New("member restore original checkpoint is missing"))
	}
	var head durablehead.Checkpoint
	if err := decodePlanJson(original, &head); err != nil {
		return durablevolume.PreparationOwnerPlan{}, err
	}
	if head.Schema != durablehead.Schema || head.Kind != spec.Kind || head.Name != spec.Name || head.MaximumBytes != spec.MaximumBytes || head.DirectoryInode != report.PhysicalRoot.Inode || head.LockName != "" || len(head.Auxiliaries) != 0 {
		return durablevolume.PreparationOwnerPlan{}, errors.New("member restore original checkpoint differs from its fixed profile")
	}
	if head.Pending != nil {
		return durablevolume.PreparationOwnerPlan{}, errors.Join(durablehead.ErrUncertain, errors.New("member restore requires the separate unfinished outer-census profile"))
	}
	metadata, present := entries[spec.Name]
	var physical *durablevolume.PreparationPhysicalMetadata
	if head.Committed.Present {
		if !present || head.Committed.Inode != metadata.Physical.Inode || head.Committed.Size <= 0 || uint64(head.Committed.Size) != metadata.Size || head.Committed.Size > spec.MaximumBytes || "sha256:"+head.Committed.Sha256 != metadata.Sha256 {
			return durablevolume.PreparationOwnerPlan{}, errors.Join(durablevolume.ErrIdentity, errors.New("member restore lost its acknowledged original census"))
		}
		physical = &durablevolume.PreparationPhysicalMetadata{Path: spec.Name, MaximumBytes: uint64(spec.MaximumBytes)}
	} else if head.Committed != (durablehead.Member{}) || present || len(files) != 0 {
		return durablevolume.PreparationOwnerPlan{}, errors.Join(durablevolume.ErrIdentity, errors.New("member restore absent head cannot authorize historical members"))
	}
	census, err := json.Marshal(storagePreparationMemberRestoreCensus{Schema: "urnetwork-successor-members-restore-v1", Profile: profile, OriginalCheckpoint: original})
	if err != nil {
		return durablevolume.PreparationOwnerPlan{}, err
	}
	return durablevolume.PreparationOwnerPlan{Owner: owner, StagingName: name, ExclusiveRoot: owner.RestoreCoverage == "", Files: files, Attributes: []durablevolume.PreparationAttributeSpec{attribute}, Census: census, PhysicalMetadata: physical}, ctx.Err()
}

// Every original member and pending stage must be accounted for. An unfinished
// payload is admissible only as the exact prefix of its retained approved
// payload, and its original pending phase is preserved.
func validateStoragePreparationMemberImage(census bootstrapSuccessorMemberCensus, spec durablehead.Spec, entries map[string]durablevolume.InventoryEntry) error {
	if err := validateBootstrapSuccessorMemberCensus(census, spec, spec.Kind == "mainnet-successor-nonce-members"); err != nil {
		return err
	}
	allowed := map[string]bool{"": true, spec.Name: true}
	pending := census.Pending
	for _, member := range census.Members {
		actual, found := entries[member.Name]
		if !found || actual.Physical == nil || actual.Physical.Inode != member.Inode {
			return errors.New("member restore lost an acknowledged member generation")
		}
		allowed[member.Name] = true
		if pending != nil && pending.Append && pending.Name == member.Name {
			payload, _ := base64.StdEncoding.Strict().DecodeString(pending.Payload)
			if member.Size >= pending.Size || actual.Size < uint64(member.Size) || actual.Size > uint64(pending.Size) || safeReleaseHash(payload[:member.Size]) != member.Sha256 || safeReleaseHash(payload[:actual.Size]) != actual.Sha256 {
				return errors.New("member restore append differs from its original exact prefix")
			}
		} else if actual.Size != uint64(member.Size) || actual.Sha256 != member.Sha256 {
			return errors.New("member restore acknowledged payload bytes differ")
		}
	}
	if pending != nil && !pending.Append {
		stage, hasStage := entries[pending.Stage]
		final, hasFinal := entries[pending.Name]
		if hasStage && hasFinal || pending.StageInode != 0 && !hasStage && !hasFinal || hasFinal && pending.StageInode == 0 {
			return errors.New("member restore pending stage is missing or ambiguous")
		}
		allowed[pending.Stage], allowed[pending.Name] = true, true
		actual := stage
		if hasFinal {
			actual = final
		}
		if hasStage || hasFinal {
			payload, _ := base64.StdEncoding.Strict().DecodeString(pending.Payload)
			if actual.Physical == nil || pending.StageInode != 0 && actual.Physical.Inode != pending.StageInode || actual.Size > uint64(pending.Size) || hasFinal && actual.Size != uint64(pending.Size) || safeReleaseHash(payload[:actual.Size]) != actual.Sha256 {
				return errors.New("member restore pending payload differs from its original exact prefix")
			}
		}
	}
	for name := range entries {
		if !allowed[name] {
			return errors.New("member restore contains an unacknowledged namespace member")
		}
	}
	return nil
}

// This pure transformation has no file or key access. It changes only inode
// fields after proving every original byte and target identity in the plan.
func rebindStoragePreparationMembersRestore(ctx context.Context, owner durablevolume.PreparationOwnerPlan, report durablevolume.Inventory, raw []byte, targets []durablevolume.PreparationSource, ownerLocal bool) ([]byte, error) {
	expected, err := planStoragePreparationMembersRestore(ctx, owner.StagingName, owner.Owner, report, ownerLocal)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(owner, expected) || expected.PhysicalMetadata == nil {
		return nil, errors.New("member restore derivation changed its exact original owner")
	}
	spec := bootstrapSuccessorMemberSpec(owner.Owner.Kind == "mainnet-successor-nonce-members")
	report, err = storagePreparationMembersRestoreView(ctx, owner.Owner, spec, report)
	if err != nil {
		return nil, err
	}
	entries := map[string]durablevolume.InventoryEntry{}
	for _, entry := range report.Entries {
		entries[entry.Path] = entry
	}
	metadata := entries[spec.Name]
	if uint64(len(raw)) != metadata.Size || safeReleaseHash(raw) != metadata.Sha256 {
		return nil, errors.New("member restore original census bytes differ")
	}
	var census bootstrapSuccessorMemberCensus
	if err := decodePlanJson(raw, &census); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(census)
	if err != nil || !bytes.Equal(raw, canonical) {
		return nil, errors.Join(errors.New("member restore original census is not canonical"), err)
	}
	if err := validateStoragePreparationMemberImage(census, spec, entries); err != nil {
		return nil, err
	}
	rebound := map[uint64]uint64{}
	seen := map[string]bool{}
	inodes := map[uint64]bool{}
	var device uint64
	for _, target := range targets {
		original, found := entries[target.File.Path]
		if !found || target.File.Path == "" || target.File.Path == spec.Name || seen[target.File.Path] || target.File != (durablevolume.PreparationFile{Path: original.Path, Kind: original.Kind, Mode: original.Mode, Bytes: original.Size, Sha256: original.Sha256}) || target.Identity.Inode == 0 || inodes[target.Identity.Inode] || target.Identity.Mode&unix.S_IFMT != unix.S_IFREG || target.Identity.Mode&07777 != 0600 {
			return nil, errors.New("member restore target member is missing, aliased or changed")
		}
		if len(seen) != 0 && target.Identity.Device != device {
			return nil, errors.New("member restore target spans physical devices")
		}
		device = target.Identity.Device
		seen[target.File.Path], inodes[target.Identity.Inode] = true, true
		rebound[original.Physical.Inode] = target.Identity.Inode
	}
	if len(targets) != len(expected.Files)-1 {
		return nil, errors.New("member restore target omits original members")
	}
	for index := range census.Members {
		census.Members[index].Inode = rebound[census.Members[index].Inode]
	}
	if census.Pending != nil && census.Pending.StageInode != 0 {
		census.Pending.StageInode = rebound[census.Pending.StageInode]
	}
	result, err := json.Marshal(census)
	if err := errors.Join(err, ctx.Err()); err != nil {
		return nil, err
	}
	return result, nil
}

// All inspection is descriptor-relative, bounded and identity checked before
// admitting bytes. No partial result escapes a failed post-read check.
func readStoragePreparationMember(ctx context.Context, root *os.File, parent unix.Stat_t, member durablevolume.PreparationFile) (raw []byte, observed unix.Stat_t, resultErr error) {
	defer func() {
		if resultErr != nil {
			raw = nil
		}
	}()
	if member.Kind != "file" || member.Mode != 0600 || member.Bytes > maximumBootstrapSuccessorMemberCensusBytes || member.Path == "" || member.Path == "." || member.Path == ".." || filepath.Base(member.Path) != member.Path || strings.ContainsRune(member.Path, 0) {
		return nil, observed, errors.New("member restore read exceeds its fixed bounds")
	}
	if err := ctx.Err(); err != nil {
		return nil, observed, err
	}
	fd, err := unix.Openat(int(root.Fd()), member.Path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, observed, mainnetDurableUnavailable("cannot open restored member", err)
	}
	file := os.NewFile(uintptr(fd), member.Path)
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	if err := unix.Fstat(fd, &observed); err != nil {
		return nil, observed, mainnetDurableUnavailable("cannot inspect restored member", err)
	}
	if observed.Dev != parent.Dev || observed.Mode&unix.S_IFMT != unix.S_IFREG || observed.Mode&07777 != 0600 || observed.Uid != parent.Uid || observed.Nlink != 1 || observed.Size < 0 || uint64(observed.Size) != member.Bytes {
		return nil, observed, errors.Join(durablevolume.ErrIdentity, errors.New("restored member generation or bounds differ"))
	}
	raw = make([]byte, int(member.Bytes))
	for offset := 0; offset < len(raw); {
		if err := ctx.Err(); err != nil {
			return nil, observed, err
		}
		part := raw[offset:min(offset+64*1024, len(raw))]
		n, err := file.ReadAt(part, int64(offset))
		if err != nil || n != len(part) {
			return nil, observed, mainnetDurableUnavailable("cannot read restored member", errors.Join(err, io.ErrUnexpectedEOF))
		}
		offset += n
	}
	var after, named unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return nil, observed, mainnetDurableUnavailable("cannot recheck restored member", err)
	}
	if err := unix.Fstatat(int(root.Fd()), member.Path, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, observed, mainnetDurableUnavailable("cannot recheck restored member name", err)
	}
	if !bootstrapSuccessorMemberSameStat(observed, after) || !bootstrapSuccessorMemberSameStat(after, named) || safeReleaseHash(raw) != member.Sha256 {
		return nil, observed, errors.Join(durablevolume.ErrIdentity, errors.New("restored member changed during exact inspection"))
	}
	return raw, observed, ctx.Err()
}

// Reverse only the permitted physical fields and require the original census
// digest. This independently prevents an accepted plan from changing a pending
// payload, signed member hash, phase, name, count or application capacity.
func inspectStoragePreparationMembersRestore(ctx context.Context, root *os.File, owner durablevolume.PreparationOwnerPlan, report durablevolume.Inventory, ownerLocal bool) ([]durablevolume.PreparedAttribute, error) {
	expected, err := planStoragePreparationMembersRestore(ctx, owner.StagingName, owner.Owner, report, ownerLocal)
	if err != nil {
		return nil, err
	}
	spec := bootstrapSuccessorMemberSpec(owner.Owner.Kind == "mainnet-successor-nonce-members")
	report, err = storagePreparationMembersRestoreView(ctx, owner.Owner, spec, report)
	if err != nil {
		return nil, err
	}
	if root == nil || len(expected.Files) != len(owner.Files) {
		return nil, errors.New("member restore inspection lost its complete target")
	}
	for index, member := range expected.Files {
		if member.Path == spec.Name {
			if owner.Files[index].Path != member.Path || owner.Files[index].Kind != member.Kind || owner.Files[index].Mode != member.Mode || owner.Files[index].Bytes == 0 || owner.Files[index].Bytes > uint64(spec.MaximumBytes) {
				return nil, errors.New("member restore changed the metadata profile")
			}
			expected.Files[index] = owner.Files[index]
		}
	}
	if !reflect.DeepEqual(expected, owner) {
		return nil, errors.New("member restore changed original signed member authority")
	}
	var parent unix.Stat_t
	if err := unix.Fstat(int(root.Fd()), &parent); err != nil {
		return nil, mainnetDurableUnavailable("cannot inspect restored member root", err)
	}
	if parent.Mode&unix.S_IFMT != unix.S_IFDIR || parent.Mode&07777 != 0700 || parent.Uid != uint32(os.Geteuid()) {
		return nil, errors.Join(durablevolume.ErrIdentity, errors.New("restored member root is not private"))
	}
	originalEntries := map[string]durablevolume.InventoryEntry{}
	for _, entry := range report.Entries {
		originalEntries[entry.Path] = entry
	}
	entries := map[string]durablevolume.InventoryEntry{}
	inodes := map[uint64]bool{parent.Ino: true}
	var metadataRaw []byte
	var metadataStat unix.Stat_t
	for _, member := range owner.Files {
		raw, stat, err := readStoragePreparationMember(ctx, root, parent, member)
		if err != nil {
			return nil, err
		}
		if inodes[stat.Ino] {
			return nil, errors.Join(durablevolume.ErrIdentity, errors.New("restored member aliases another owner inode"))
		}
		inodes[stat.Ino] = true
		entry := originalEntries[member.Path]
		entry.Physical = &durablevolume.PhysicalRoot{Device: durablevolume.Device{Major: unix.Major(stat.Dev), Minor: unix.Minor(stat.Dev)}, Inode: stat.Ino}
		entry.Size, entry.Sha256 = member.Bytes, member.Sha256
		entries[member.Path] = entry
		if member.Path == spec.Name {
			metadataRaw, metadataStat = raw, stat
		}
	}
	var retained storagePreparationMemberRestoreCensus
	var head durablehead.Checkpoint
	if err := decodePlanJson(owner.Census, &retained); err != nil {
		return nil, err
	}
	if err := decodePlanJson(retained.OriginalCheckpoint, &head); err != nil {
		return nil, err
	}
	if head.Committed.Present {
		var census bootstrapSuccessorMemberCensus
		if err := decodePlanJson(metadataRaw, &census); err != nil {
			return nil, err
		}
		canonical, err := json.Marshal(census)
		if err != nil || !bytes.Equal(canonical, metadataRaw) {
			return nil, errors.Join(errors.New("restored member census is not canonical"), err)
		}
		if err := validateStoragePreparationMemberImage(census, spec, entries); err != nil {
			return nil, err
		}
		for index := range census.Members {
			census.Members[index].Inode = originalEntries[census.Members[index].Name].Physical.Inode
		}
		if pending := census.Pending; pending != nil && pending.StageInode != 0 {
			original, present := originalEntries[pending.Stage]
			if !present {
				original = originalEntries[pending.Name]
			}
			pending.StageInode = original.Physical.Inode
		}
		originalRaw, err := json.Marshal(census)
		original := originalEntries[spec.Name]
		if err != nil || uint64(len(originalRaw)) != original.Size || safeReleaseHash(originalRaw) != original.Sha256 {
			return nil, errors.Join(errors.New("restored census changes authority beyond physical inode fields"), err)
		}
		head.Committed = durablehead.Member{Present: true, Inode: metadataStat.Ino, Size: int64(len(metadataRaw)), Sha256: strings.TrimPrefix(safeReleaseHash(metadataRaw), "sha256:")}
	}
	head.DirectoryInode = parent.Ino
	var after unix.Stat_t
	if err := unix.Fstat(int(root.Fd()), &after); err != nil {
		return nil, mainnetDurableUnavailable("cannot recheck restored member root", err)
	}
	if !bootstrapSuccessorMemberSameStat(parent, after) {
		return nil, errors.Join(durablevolume.ErrIdentity, errors.New("restored member root changed during inspection"))
	}
	raw, err := json.Marshal(head)
	if err := errors.Join(err, ctx.Err()); err != nil {
		return nil, err
	}
	return []durablevolume.PreparedAttribute{{Spec: owner.Attributes[0], Raw: raw}}, nil
}
