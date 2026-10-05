package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/urfoundation/sn/nativefee"
)

// This test-only export lets the Server public model test consume an actual
// owned verifier outcome without exposing a production constructor. The nested
// runtime peer is synthetic; signed receipt/GRANDPA verification is real.
// Qualification must provide the explicitly selected private output directory.
func TestNativeFeeOutcomeExportSettlementFixtures(t *testing.T) {
	directory := os.Getenv("URNETWORK_NATIVE_FEE_EXPORT_DIRECTORY")
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		t.Fatal("set URNETWORK_NATIVE_FEE_EXPORT_DIRECTORY to the selected private fixture output directory")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		t.Fatal("native fee fixture output must already be a private directory", err)
	}
	for _, mode := range []string{"pair", "missing"} {
		exportNativeFeeSettlementFixture(t, directory, mode)
	}
}

type nativeFeeSettlementFixtureManifest struct {
	Schema            string              `json:"schema"`
	Authority         nativefee.Authority `json:"authority"`
	Request           nativefee.Reference `json:"request"`
	TransactionHash   string              `json:"transaction_hash"`
	RawTransaction    []byte              `json:"raw_transaction"`
	Sender            string              `json:"sender"`
	Nonce             uint64              `json:"nonce"`
	RuntimeCodeSha256 string              `json:"runtime_code_sha256"`
	NativeBlockNumber uint64              `json:"native_block_number"`
	ReceiptStatus     uint64              `json:"receipt_status"`
}

func exportNativeFeeSettlementFixture(t *testing.T, directory, mode string) {
	t.Helper()
	contextRequest, fixture, job := historicalFeeContextTestFixture(t, "success", mode)
	request, approval, key := economicNativeFeeTestRequestForContext(t, contextRequest, job)
	root := filepath.Join(directory, mode)
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		name      string
		reference *planFileReference
	}{
		{"archive.json", &request.Context.Archive}, {"collection.json", &request.Context.Collection}, {"checkpoint.json", &request.Context.Checkpoint}, {"finality.json", &request.Context.FinalityProof}, {"job.json", &request.Context.Job},
	} {
		raw, err := os.ReadFile(input.reference.Path)
		if err != nil {
			t.Fatal(err)
		}
		if monitorReadDigest(raw) != input.reference.Sha256 {
			t.Fatal("fixture original changed before export")
		}
		input.reference.Path = filepath.Join(root, input.name)
		if err := os.WriteFile(input.reference.Path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	approval.ContextRequestHash = rootObjectHash(request.Context)
	economicNativeFeeTestSign(t, &request, &approval, key)
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(root, "request.json")
	if err := os.WriteFile(requestPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := nativeFeeSettlementFixtureManifest{Schema: "urnetwork-native-fee-settlement-test-fixture-v1", Authority: nativefee.Authority{Verifier: nativefee.Reference(request.Context.Engine), NativePolicy: nativefee.NativePolicy(request.Policy)}, Request: nativefee.Reference{Path: requestPath, Sha256: monitorReadDigest(raw)}, RuntimeCodeSha256: economicNativeFeeSha(job.RuntimeCodeSha256), NativeBlockNumber: fixture.Expected.Blocks[0].NativeContexts[0].NativeBlock.Number}
	for _, transaction := range fixture.Expected.Transactions {
		if transaction.Receipt != nil {
			manifest.TransactionHash, manifest.Sender, manifest.Nonce, manifest.ReceiptStatus = transaction.Hash, transaction.Sender, transaction.Nonce, transaction.Receipt.Status
			for _, original := range fixture.Archive.Transactions {
				if original.Hash == transaction.Hash {
					manifest.RawTransaction = original.Raw
				}
			}
			break
		}
	}
	if len(manifest.RawTransaction) == 0 {
		t.Fatal("export lost original signed transaction")
	}
	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, mode+".json"), manifestRaw, 0600); err != nil {
		t.Fatal(err)
	}
}
