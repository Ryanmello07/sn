//go:build linux

// Actual public dispatch consumes exact signed config, stopped physical roots
// and retained ledger heads. Only synthetic kernel facts and keys are supplied.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/urfoundation/sn/internal/durablefixture"
	"github.com/urfoundation/sn/internal/durablepath"
	"github.com/urfoundation/sn/validator"
	"github.com/urnetwork/connect/durablevolume"
	"golang.org/x/sys/unix"
)

// The root contains two separately authenticated ledgers and the coordinator
// directory, exercising one shared physical lease without a self-deadlock.
type validatorCapacityCommandFixture struct {
	validator *bootstrapChainValidatorFixture
	storage   *durablefixture.Fixture
	request   validator.ProductionCapacityRequest
	root      string
	metadata  string
	path      string
}

func newValidatorCapacityCommandFixture(t *testing.T) *validatorCapacityCommandFixture {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	_, _, scope := newSubnetFixture(t)
	scope.RuntimeVersion.SpecName, scope.RuntimeVersion.TransactionVersion = "node-subtensor", 1
	chain := &bootstrapChainFixture{path: filepath.Join(parent, "synthetic-chain.json"), config: bootstrapChainConfig{DeploymentId: "synthetic-capacity-deployment"}}
	producer := newBootstrapChainValidatorFixture(t, chain, scope, 0)
	root := filepath.Dir(producer.config.StateDir)
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(producer.config.StateDir, 0700); err != nil {
		t.Fatal(err)
	}
	request := validator.ProductionCapacityRequest{Schema: validator.ProductionCapacityRequestSchema,
		SuccessorApprovalPath: filepath.Join(parent, "successor-approval.json"), OriginalAuthorityPath: filepath.Join(parent, "original-authority.json"),
		FutureHistoryBytes: 2 * 1024 * 1024, FutureCaptureFiles: 32}
	for index, operator := range producer.config.Operators {
		key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{byte(0xc1 + index)}, ed25519.SeedSize))
		identity := validator.AttemptLedgerIdentity{DeploymentID: producer.config.DeploymentID, ChainID: producer.config.ChainID, GenesisHash: producer.config.GenesisHash,
			Netuid: producer.config.Netuid, ValidatorID: producer.config.ValidatorID, ValidatorUID: uint16(index), NoID: operator.NoID,
			ValidatorVPK: "0x" + hex.EncodeToString(key.Public().(ed25519.PublicKey))}
		ledgerScope := validator.AttemptLedgerPreparationScope{Identity: identity, Coordinator: producer.config.Coordinator,
			Limits: producer.config.EvidenceV2.Bounds.Disk, ExpectedHead: validator.AttemptLedgerHead{Root: "0x" + strings.Repeat("00", 32)}}
		directory, err := os.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		census, buildErr := validator.BuildFreshAttemptLedgerPreparation(t.Context(), directory, filepath.Base(operator.StateDir), ledgerScope)
		if err := errors.Join(buildErr, directory.Close()); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(operator.StateDir)
		if err != nil {
			t.Fatal(err)
		}
		checkpoint, checkpointErr := validator.BuildAttemptLedgerPreparationCheckpoint(t.Context(), file, ledgerScope, census)
		if checkpointErr == nil {
			checkpointErr = unix.Fsetxattr(int(file.Fd()), "user.urnetwork.attempt-ledger-custody", checkpoint, unix.XATTR_CREATE)
		}
		if err := errors.Join(checkpointErr, file.Sync(), file.Close()); err != nil {
			t.Fatal(err)
		}
		request.Sources = append(request.Sources, validator.ProductionCapacityInput{Ledger: ledgerScope, FutureRecords: 4, FutureTrails: 1,
			FutureRecordBytes: 4 * ledgerScope.Limits.MaxRecordBytes, FutureProofBytes: producer.config.EvidenceV2.Bounds.Replay.MaxProofBytes,
			FutureStorageBytes: 1024 * 1024, FutureStorageFiles: 16})
	}
	producer.publish(t)
	if _, err := validator.LoadReleaseConfig(producer.path); err != nil {
		t.Fatal("fixture did not load its exact original signed production config", err)
	}
	raw, err := os.ReadFile(producer.path)
	if err != nil {
		t.Fatal(err)
	}
	request.Config = validator.ReleaseEvidenceV2File{Path: producer.path, Bytes: uint64(len(raw)), SHA256: fmt.Sprintf("0x%x", sha256.Sum256(raw))}
	request.Bounds = producer.config.EvidenceV2.Bounds
	request.Bounds.Disk.MaxRecordCount *= 4
	request.Bounds.Disk.MaxTrailCount *= 4
	request.Bounds.Disk.MaxRawRecordBytes *= 4
	request.Bounds.Disk.MaxProofBytes *= 4
	request.Bounds.Disk.MaxStorageBytes *= 4
	request.Bounds.Disk.MaxStorageFiles *= 4
	request.Bounds.Replay.MaxTrails *= 4
	request.Bounds.Replay.MaxScratchBytes *= 4
	request.Bounds.Replay.MaxScratchFiles *= 4
	for _, stream := range []*validator.AttemptStreamV2Bounds{&request.Bounds.Cut.Records, &request.Bounds.Cut.Proofs} {
		stream.MaxItems *= 4
		stream.MaxDataBytes *= 4
		stream.MaxChunks = stream.MaxItems
		stream.MaxPages = stream.MaxChunks
	}
	request.Bounds.MaxHistoryBytes = 16 * 1024 * 1024
	request.Bounds.MaxCaptureFiles = producer.config.EvidenceV2.Bounds.CaptureFileLimit() * 2
	storage := durablefixture.New(t, t.Context(), root)
	request.Roots = []validator.ProductionCapacityRoot{{Path: root, FormerWriterFence: storageInspectionTestFence(t, storage, root),
		Limits: durablevolume.InventoryLimits{MaxEntries: 256, MaxBytes: 64 * 1024 * 1024, MaxDepth: 4, MaxOwnerAttributes: 32, MaxOwnerAttributeBytes: 128 * 1024}}}
	return &validatorCapacityCommandFixture{validator: producer, storage: storage, request: request, root: root, metadata: parent, path: filepath.Join(parent, "capacity-request.json")}
}

// Rewriting an unsigned test request never modifies its independently signed
// original config, approval, physical root declaration or ledger checkpoint.
func (self *validatorCapacityCommandFixture) args(t *testing.T) []string {
	t.Helper()
	raw, err := json.Marshal(self.request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(self.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return []string{"validator-capacity-preview", "--request", self.path, "--request-sha256", durablefixture.Digest(raw)}
}

func TestValidatorCapacityPreviewBindsJoinedRetainedCensus(t *testing.T) {
	f := newValidatorCapacityCommandFixture(t)
	before := mainnetNamespaceTest(t, f.root)
	var first, second, diagnostic bytes.Buffer
	args := f.args(t)
	for _, output := range []*bytes.Buffer{&first, &second} {
		if code := runMain(f.storage.Context, args, output, &diagnostic); code != 0 {
			t.Fatal("public capacity preview refused original retained owners", code, diagnostic.String())
		}
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) || !reflect.DeepEqual(before, mainnetNamespaceTest(t, f.root)) {
		t.Fatal("repeatable planning changed original custody or its unsigned draft")
	}
	var preview validator.ProductionCapacityPreview
	if err := json.Unmarshal(first.Bytes(), &preview); err != nil || preview.RestartAuthorized || len(preview.Ledgers) != 2 || len(preview.Physical) != 1 || preview.Config == nil {
		t.Fatal("preview lost joined physical/logical census or granted restart", err)
	}
	for _, path := range []string{f.request.SuccessorApprovalPath, f.request.OriginalAuthorityPath, f.validator.config.HotkeySeedFile} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("preview created authority or signing state", path, err)
		}
	}
	if err := os.WriteFile(preview.OriginalAuthority.Path, preview.OriginalAuthorityBytes, 0600); err != nil {
		t.Fatal(err)
	}
	// Only this explicit synthetic independent approval turns the draft into
	// loadable config. Its original economic body/window remains unchanged.
	oldApproval := f.validator.approval
	f.validator.config, f.validator.approval = *preview.Config, preview.Approval
	f.validator.path = filepath.Join(f.metadata, "signed-successor.yml")
	f.validator.publish(t)
	message, err := f.validator.approval.SigningMessage()
	if err != nil || "0x"+hex.EncodeToString(message) != preview.SigningBytes {
		t.Fatal("preview signing bytes differed from actual normalized config", err)
	}
	loaded, err := validator.LoadReleaseConfig(f.validator.path)
	if err != nil || loaded.ProductionCapacityRevision == nil || len(loaded.ProductionAuthorityHistory) != 1 {
		t.Fatal("public preview cannot become an independently approved resource successor", err)
	}
	actual := f.validator.approval
	actual.ConfigHash, oldApproval.ConfigHash = [32]byte{}, [32]byte{}
	if !reflect.DeepEqual(actual, oldApproval) || !reflect.DeepEqual(before, mainnetNamespaceTest(t, f.root)) {
		t.Fatal("capacity preview changed original economic scope or owned bytes")
	}
}

func TestValidatorCapacityPreviewRequiresDeclarationAndExactInput(t *testing.T) {
	f := newValidatorCapacityCommandFixture(t)
	before := mainnetNamespaceTest(t, f.root)
	args := f.args(t)
	var output, diagnostic bytes.Buffer
	if code := runMain(t.Context(), args, &output, &diagnostic); code == 0 || output.Len() != 0 || !strings.Contains(diagnostic.String(), "durable") {
		t.Fatal("actual command admitted absent physical authority", code, diagnostic.String())
	}
	args[len(args)-1] = "sha256:" + strings.Repeat("00", 32)
	diagnostic.Reset()
	if code := runMain(f.storage.Context, args, &output, &diagnostic); code == 0 || output.Len() != 0 || !strings.Contains(diagnostic.String(), "hash") && !strings.Contains(diagnostic.String(), "differ") {
		t.Fatal("actual command admitted unreviewed forecast bytes", code, diagnostic.String())
	}
	if !reflect.DeepEqual(before, mainnetNamespaceTest(t, f.root)) {
		t.Fatal("refused preview modified retained state")
	}
}

func TestValidatorCapacityPreviewRefusesMissingCheckpointAndOwner(t *testing.T) {
	f := newValidatorCapacityCommandFixture(t)
	before := mainnetNamespaceTest(t, f.root)
	complete := f.request.Sources
	f.request.Sources = complete[:1]
	var output, diagnostic bytes.Buffer
	if code := runMain(f.storage.Context, f.args(t), &output, &diagnostic); code == 0 || output.Len() != 0 || !strings.Contains(diagnostic.String(), "complete") {
		t.Fatal("preview admitted only a subset of configured owners", code, diagnostic.String())
	}
	f.request.Sources = complete
	operator := f.validator.config.Operators[1].StateDir
	if err := unix.Removexattr(operator, "user.urnetwork.attempt-ledger-custody"); err != nil {
		t.Fatal(err)
	}
	diagnostic.Reset()
	if code := runMain(f.storage.Context, f.args(t), &output, &diagnostic); code == 0 || output.Len() != 0 || !strings.Contains(diagnostic.String(), "original custody checkpoint") {
		t.Fatal("preview inferred fresh authority from missing original head", code, diagnostic.String())
	}
	if !reflect.DeepEqual(before, mainnetNamespaceTest(t, f.root)) {
		t.Fatal("refused preview rewrote original ledger members")
	}
	owner, err := durablepath.OpenVolume(f.storage.Context, f.root, durablevolume.ReadWrite)
	if err != nil {
		t.Fatal("failed joined preview leaked a snapshot lease", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestValidatorCapacityPreviewRefusesActiveOwnerCancellationAndShortOutput(t *testing.T) {
	f := newValidatorCapacityCommandFixture(t)
	args := f.args(t)
	before := mainnetNamespaceTest(t, f.root)
	owner, err := durablepath.OpenVolume(f.storage.Context, f.root, durablevolume.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	var output, diagnostic bytes.Buffer
	code := runMain(f.storage.Context, args, &output, &diagnostic)
	closeErr := owner.Close()
	if code == 0 || output.Len() != 0 || !strings.Contains(diagnostic.String(), "busy") || closeErr != nil {
		t.Fatal("capacity planning observed an active writer or leaked its lease", code, diagnostic.String(), closeErr)
	}
	canceled, cancel := context.WithCancel(f.storage.Context)
	cancel()
	diagnostic.Reset()
	if code := runMain(canceled, args, &output, &diagnostic); code == 0 || output.Len() != 0 || !strings.Contains(diagnostic.String(), "canceled") {
		t.Fatal("canceled planning emitted a usable draft", code, diagnostic.String())
	}
	var short storageInspectionShortWriter
	diagnostic.Reset()
	if code := runMain(f.storage.Context, args, &short, &diagnostic); code != 1 || short.Len() == 0 || !strings.Contains(diagnostic.String(), "short write") {
		t.Fatal("incomplete output was treated as approved capacity", code, diagnostic.String())
	}
	if !reflect.DeepEqual(before, mainnetNamespaceTest(t, f.root)) {
		t.Fatal("canceled or truncated preview changed original owners")
	}
}
