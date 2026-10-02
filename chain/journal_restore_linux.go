//go:build linux

// Offline native restore preserves original JSONL/SCALE bytes and rebinds only
// physical checkpoint coordinates. No signer, RPC or mutable journal is opened.
package chain

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/urnetwork/connect/durablevolume"
	"golang.org/x/sys/unix"
)

const nativeJournalRestoreSchema = "urnetwork-native-journal-restore-v1"

// The reviewed plan retains the complete old checkpoint, including any exact
// append intent; target inode coordinates are derived only during apply.
type nativeJournalRestoreCensus struct {
	Schema          string                        `json:"schema"`
	Scope           NativeJournalPreparationScope `json:"scope"`
	OriginalCustody []byte                        `json:"original_custody"`
}

// Strict public inputs cannot smuggle a signing key or unreviewed extension.
func decodeNativeRestore(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.Join(errors.New("native restore authority has trailing data"), err)
	}
	return nil
}

// Layout validation consumes the exported original inode census. It accepts
// clean custody and exact pending log appends. Ambiguous legacy raw publication
// still needs original predecessor member authority and is never reset here.
func PlanNativeJournalRestore(ctx context.Context, name string, owner durablevolume.PreparationOwner, report durablevolume.Inventory) (durablevolume.PreparationOwnerPlan, error) {
	if ctx == nil {
		return durablevolume.PreparationOwnerPlan{}, errors.New("native restore requires cancellation context")
	}
	if err := ctx.Err(); err != nil {
		return durablevolume.PreparationOwnerPlan{}, err
	}
	if owner.Kind != NativeJournalPreparationKind || owner.Purpose != "restore" || owner.RelativePath != "." || report.Schema != durablevolume.PhysicalInventorySchema || report.RestartAuthorized {
		return durablevolume.PreparationOwnerPlan{}, errors.New("native restore requires exact physical retained authority")
	}
	var scope NativeJournalPreparationScope
	if err := decodeNativeRestore(owner.Inputs, &scope); err != nil {
		return durablevolume.PreparationOwnerPlan{}, err
	}
	if err := scope.validate(); err != nil {
		return durablevolume.PreparationOwnerPlan{}, err
	}
	var root, rawDirectory, journal *durablevolume.InventoryEntry
	var anchorRaw []byte
	files := make([]durablevolume.PreparationFile, 0, len(report.Entries))
	members := map[string]nativeRawMember{}
	var rawBytes uint64
	for index := range report.Entries {
		if err := ctx.Err(); err != nil {
			return durablevolume.PreparationOwnerPlan{}, err
		}
		entry := &report.Entries[index]
		if entry.Physical == nil || entry.Physical.Inode == 0 || entry.Physical.Device != report.PhysicalRoot.Device {
			return durablevolume.PreparationOwnerPlan{}, errors.New("native restore lacks original member generations")
		}
		if entry.Path != "" {
			files = append(files, durablevolume.PreparationFile{Path: entry.Path, Kind: entry.Kind, Mode: entry.Mode, Bytes: entry.Size, Sha256: entry.Sha256})
		}
		for _, attribute := range entry.OwnerAttributes {
			if entry.Path != "" || attribute.Name != NativeJournalCustodyAttribute && attribute.Name != durablevolume.PreparationAttribute {
				return durablevolume.PreparationOwnerPlan{}, errors.New("native restore cannot omit another owner's checkpoint")
			}
			if attribute.Name == NativeJournalCustodyAttribute {
				if anchorRaw != nil || len(attribute.Value) == 0 || len(attribute.Value) > 4096 || "sha256:"+nativeDigest(attribute.Value) != attribute.Sha256 {
					return durablevolume.PreparationOwnerPlan{}, errors.New("native restore original checkpoint differs")
				}
				anchorRaw = append([]byte(nil), attribute.Value...)
			}
		}
		switch entry.Path {
		case "":
			root = entry
		case journalRawDir:
			rawDirectory = entry
		case journalFileName:
			journal = entry
		default:
			if filepath.Dir(entry.Path) != journalRawDir || entry.Kind != "file" || entry.Mode != 0600 || entry.Size < 1 || entry.Size > scope.MaximumRawRecordBytes || entry.Size > scope.MaximumRawBytes-rawBytes {
				return durablevolume.PreparationOwnerPlan{}, errors.New("native restore contains unknown or unbounded retained members")
			}
			memberName := filepath.Base(entry.Path)
			if len(memberName) != 70 || !strings.HasSuffix(memberName, ".scale") || strings.ToLower(memberName) != memberName {
				return durablevolume.PreparationOwnerPlan{}, errors.Join(ErrJournalUncertain, errors.New("native restore contains an unfinished raw publication"))
			}
			if hash, err := hex.DecodeString(memberName[:64]); err != nil || len(hash) != 32 {
				return durablevolume.PreparationOwnerPlan{}, errors.New("native restored raw name is invalid")
			}
			if _, exists := members[memberName]; exists || uint64(len(members)) >= scope.MaximumRawMembers {
				return durablevolume.PreparationOwnerPlan{}, errors.New("native restore raw census is duplicated or exceeds its count")
			}
			rawBytes += entry.Size
			members[memberName] = nativeRawMember{Name: memberName, Inode: entry.Physical.Inode, Size: int64(entry.Size), Sha256: strings.TrimPrefix(entry.Sha256, "sha256:")}
		}
	}
	if root == nil || rawDirectory == nil || journal == nil || anchorRaw == nil || root.Kind != "directory" || root.Mode != 0700 || rawDirectory.Kind != "directory" || rawDirectory.Mode != 0700 || journal.Kind != "file" || journal.Mode != 0600 || journal.Size > scope.MaximumJournalBytes {
		return durablevolume.PreparationOwnerPlan{}, errors.New("native restore is missing original log, raw directory or anchor")
	}
	var anchor nativeJournalCustody
	if err := decodeNativeRestore(anchorRaw, &anchor); err != nil {
		return durablevolume.PreparationOwnerPlan{}, err
	}
	if anchor.Schema != nativeJournalCustodySchema || anchor.DirectoryInode != root.Physical.Inode || anchor.RawDirectoryInode != rawDirectory.Physical.Inode {
		return durablevolume.PreparationOwnerPlan{}, errors.New("native restore original directory generations differ")
	}
	actual := nativeJournalCheckpoint{JournalInode: journal.Physical.Inode, JournalSize: int64(journal.Size), JournalSha256: strings.TrimPrefix(journal.Sha256, "sha256:"), RawCount: len(members), RawSha256: nativeRawDigest(members)}
	if anchor.Pending == nil {
		if actual != anchor.Committed {
			return durablevolume.PreparationOwnerPlan{}, errors.Join(durablevolume.ErrIdentity, errors.New("native restore completed original census differs"))
		}
	} else {
		old, next := anchor.Committed, *anchor.Pending
		if old.JournalInode != next.JournalInode || old.RawCount != next.RawCount || old.RawSha256 != next.RawSha256 || old.RawCount != actual.RawCount || old.RawSha256 != actual.RawSha256 {
			return durablevolume.PreparationOwnerPlan{}, errors.Join(ErrJournalUncertain, errors.New("native raw restore requires original predecessor member authority"))
		}
		if old.JournalSize < 0 || next.JournalSize < old.JournalSize || uint64(next.JournalSize) > scope.MaximumJournalBytes || actual != old && actual != next {
			return durablevolume.PreparationOwnerPlan{}, errors.Join(ErrJournalUncertain, errors.New("native restore append bytes match neither original exact checkpoint"))
		}
		for _, digest := range []string{old.JournalSha256, next.JournalSha256} {
			decoded, err := hex.DecodeString(digest)
			if err != nil || len(decoded) != 32 || strings.ToLower(digest) != digest {
				return durablevolume.PreparationOwnerPlan{}, errors.New("native restore append checkpoint hash is malformed")
			}
		}
	}
	census, err := json.Marshal(nativeJournalRestoreCensus{Schema: nativeJournalRestoreSchema, Scope: scope, OriginalCustody: anchorRaw})
	if err != nil {
		return durablevolume.PreparationOwnerPlan{}, err
	}
	return durablevolume.PreparationOwnerPlan{Owner: owner, StagingName: name, ExclusiveRoot: true, Files: files,
		Attributes: []durablevolume.PreparationAttributeSpec{{Path: ".", Name: NativeJournalCustodyAttribute}}, Census: census}, ctx.Err()
}

// Readback binds payload bytes and physical files through real descriptors.
// Failed post-read admission returns no bytes, including complete reads.
func nativeRestoreRead(ctx context.Context, parent *os.File, name string, expected durablevolume.PreparationFile) (result []byte, stat unix.Stat_t, resultErr error) {
	if ctx == nil || parent == nil || filepath.Base(name) != name || name == "." || name == ".." || expected.Kind != "file" || expected.Mode != 0600 || expected.Bytes > 16*1024*1024 {
		return nil, stat, errors.New("native restore read lacks bounded descriptor authority")
	}
	defer func() {
		if resultErr != nil {
			result = nil
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, stat, err
	}
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, stat, nativeObservation(err, true)
	}
	file := os.NewFile(uintptr(fd), name)
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, stat, nativeObservation(err, false)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&07777 != 0600 || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) || stat.Size < 0 || uint64(stat.Size) != expected.Bytes {
		return nil, stat, errors.Join(durablevolume.ErrIdentity, errors.New("native restore payload metadata differs"))
	}
	result = make([]byte, int(expected.Bytes))
	for offset := 0; offset < len(result); {
		if err := ctx.Err(); err != nil {
			return nil, stat, err
		}
		chunk := result[offset:min(offset+64*1024, len(result))]
		n, err := file.ReadAt(chunk, int64(offset))
		if err != nil || n != len(chunk) {
			return nil, stat, nativeObservation(errors.Join(err, io.ErrUnexpectedEOF), false)
		}
		offset += n
	}
	var after, named unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return nil, stat, nativeObservation(err, false)
	}
	if err := unix.Fstatat(int(parent.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, stat, nativeObservation(err, true)
	}
	if !sameNativeRestoreStat(stat, after) || !sameNativeRestoreStat(after, named) || "sha256:"+nativeDigest(result) != expected.Sha256 {
		return nil, stat, errors.Join(durablevolume.ErrIdentity, errors.New("native restore payload changed or differs from original bytes"))
	}
	return result, stat, ctx.Err()
}

// Access time is irrelevant; content, protection and generation must remain.
func sameNativeRestoreStat(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid && a.Nlink == b.Nlink && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}

// Restore checkpoint construction is read-only. It preserves every original
// append boundary and signature, changing only target inode-bearing hashes.
// Pending appends remain pending for the original joined runtime reconciliation.
func InspectNativeJournalRestore(ctx context.Context, root *os.File, owner durablevolume.PreparationOwnerPlan, report durablevolume.Inventory) (result []durablevolume.PreparedAttribute, resultErr error) {
	defer func() {
		if resultErr != nil {
			result = nil
		}
	}()
	expected, err := PlanNativeJournalRestore(ctx, owner.StagingName, owner.Owner, report)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(expected, owner) {
		return nil, errors.New("native restore plan changed its exact original census")
	}
	var census nativeJournalRestoreCensus
	var anchor nativeJournalCustody
	if err := decodeNativeRestore(owner.Census, &census); err != nil {
		return nil, err
	}
	if err := decodeNativeRestore(census.OriginalCustody, &anchor); err != nil {
		return nil, err
	}
	rootStat, err := nativePreparationStat(root, true)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Openat(int(root.Fd()), journalRawDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nativeObservation(err, true)
	}
	rawDirectory := os.NewFile(uintptr(fd), journalRawDir)
	defer func() { resultErr = errors.Join(resultErr, rawDirectory.Close()) }()
	rawStat, err := nativePreparationStat(rawDirectory, true)
	if err != nil {
		return nil, err
	}
	if rawStat.Dev != rootStat.Dev {
		return nil, errors.Join(durablevolume.ErrIdentity, errors.New("native restore crossed a physical filesystem"))
	}
	members := map[string]nativeRawMember{}
	var journalBytes []byte
	var journalStat unix.Stat_t
	for _, file := range owner.Files {
		if file.Kind == "directory" {
			continue
		}
		parent, name := root, file.Path
		if filepath.Dir(file.Path) == journalRawDir {
			parent, name = rawDirectory, filepath.Base(file.Path)
		}
		raw, stat, err := nativeRestoreRead(ctx, parent, name, file)
		if err != nil {
			return nil, err
		}
		if stat.Dev != rootStat.Dev {
			return nil, errors.Join(durablevolume.ErrIdentity, errors.New("native restored member device differs"))
		}
		if file.Path == journalFileName {
			journalBytes, journalStat = raw, stat
			if _, err := parseNativeJournalEntries(ctx, raw); err != nil {
				return nil, errors.Join(ErrJournalUncertain, err)
			}
		} else {
			if strings.TrimPrefix(ExtrinsicHash(raw).Hex(), "0x") != name[:64] {
				return nil, errors.Join(durablevolume.ErrIdentity, errors.New("native restored SCALE bytes differ from their original extrinsic hash"))
			}
			members[name] = nativeRawMember{Name: name, Inode: stat.Ino, Size: stat.Size, Sha256: nativeDigest(raw)}
		}
	}
	if anchor.Pending != nil && int64(len(journalBytes)) >= anchor.Committed.JournalSize && nativeDigest(journalBytes[:anchor.Committed.JournalSize]) != anchor.Committed.JournalSha256 {
		return nil, errors.Join(durablevolume.ErrIdentity, errors.New("native pending restore changed the acknowledged original log prefix"))
	}
	names, readErr := rawDirectory.Readdirnames(int(census.Scope.MaximumRawMembers + 1))
	if readErr != nil && readErr != io.EOF {
		return nil, nativeObservation(readErr, false)
	}
	if len(names) != len(members) {
		return nil, errors.Join(durablevolume.ErrIdentity, errors.New("native restored raw namespace changed"))
	}
	for _, name := range names {
		if _, ok := members[name]; !ok {
			return nil, errors.Join(durablevolume.ErrIdentity, errors.New("native restore contains an extra raw member"))
		}
	}
	if err := nativePreparationNamed(root, journalRawDir, rawDirectory, rawStat, true); err != nil {
		return nil, err
	}
	after, err := nativePreparationStat(root, true)
	if err != nil {
		return nil, err
	}
	if !sameNativeRestoreStat(rootStat, after) {
		return nil, errors.Join(durablevolume.ErrIdentity, errors.New("native restored root changed during semantic inspection"))
	}
	anchor.DirectoryInode, anchor.RawDirectoryInode = rootStat.Ino, rawStat.Ino
	anchor.Committed.JournalInode, anchor.Committed.RawSha256 = journalStat.Ino, nativeRawDigest(members)
	if anchor.Pending != nil {
		anchor.Pending.JournalInode, anchor.Pending.RawSha256 = journalStat.Ino, anchor.Committed.RawSha256
	}
	encoded, err := json.Marshal(anchor)
	if err := errors.Join(err, ctx.Err()); err != nil {
		return nil, err
	}
	return []durablevolume.PreparedAttribute{{Spec: owner.Attributes[0], Raw: encoded}}, nil
}
