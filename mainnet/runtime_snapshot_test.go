// Snapshot tests use synthetic local RPC evidence and never contact a chain.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/blake2b"
)

// Serves exact pinned artifact bytes, with a controlled identity or hash fault.
func runtimeSnapshotTestServer(t *testing.T, evmChainId string, wrongCodeHash, changedGenesis bool) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	code := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	metadata := []byte{109, 101, 116, 97, 255, 0}
	codeDigest := blake2b.Sum256(code)
	codeHash := "0x" + hex.EncodeToString(codeDigest[:])
	if wrongCodeHash {
		codeHash = testGenesisHash
	}
	artifactReads := &atomic.Int64{}
	genesisReads := &atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			JsonRpc string `json:"jsonrpc"`
			Id      int    `json:"id"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
		}
		if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&call) != nil || call.JsonRpc != "2.0" || call.Id != 1 {
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		var result any
		switch call.Method {
		case "system_chain":
			result = "Bittensor"
		case "system_version":
			result = "synthetic node"
		case "eth_chainId":
			result = evmChainId
		case "chain_getFinalizedHead":
			result = testFinalizedHash
		case "chain_getHeader":
			if len(call.Params) != 1 || call.Params[0] != testFinalizedHash {
				t.Errorf("header was not pinned to finalized hash: %v", call.Params)
			}
			result = map[string]any{"number": "0x7b852f"}
		case "chain_getBlockHash":
			if len(call.Params) != 1 {
				t.Errorf("block lookup has invalid params: %v", call.Params)
			}
			if call.Params[0] == float64(0) {
				result = testGenesisHash
				if changedGenesis && genesisReads.Add(1) > 1 {
					result = testFinalizedHash
				}
			} else if call.Params[0] == float64(0x7b852f) {
				result = testFinalizedHash
			} else {
				t.Errorf("unexpected block lookup: %v", call.Params)
			}
		case "state_getRuntimeVersion":
			result = map[string]any{"specName": "node-subtensor", "specVersion": 460, "transactionVersion": 1, "stateVersion": 1}
		case "state_getStorageHash":
			artifactReads.Add(1)
			if len(call.Params) != 2 || call.Params[0] != runtimeCodeStorageKey || call.Params[1] != testFinalizedHash {
				t.Errorf("code hash read was not pinned: %v", call.Params)
			}
			result = codeHash
		case "state_getStorage":
			artifactReads.Add(1)
			if len(call.Params) != 2 || call.Params[0] != runtimeCodeStorageKey || call.Params[1] != testFinalizedHash {
				t.Errorf("code read was not pinned: %v", call.Params)
			}
			result = "0x" + strings.ToUpper(hex.EncodeToString(code))
		case "state_getMetadata":
			artifactReads.Add(1)
			if len(call.Params) != 1 || call.Params[0] != testFinalizedHash {
				t.Errorf("metadata read was not pinned: %v", call.Params)
			}
			result = "0x" + strings.ToUpper(hex.EncodeToString(metadata))
		default:
			t.Errorf("unexpected RPC method %q", call.Method)
			http.Error(writer, "unexpected method", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result}); err != nil {
			t.Errorf("encode synthetic RPC reply: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server, artifactReads
}

// The raw bytes, their independent hashes and the enclosing digest must agree.
func TestRuntimeSnapshotBindsOneFinalizedArtifact(t *testing.T) {
	server, artifactReads := runtimeSnapshotTestServer(t, "0x3c4", false, false)
	var stdout, stderr bytes.Buffer
	args := []string{"runtime-snapshot", "--rpc", server.URL, "--expected-chain", "Bittensor", "--expected-genesis", testGenesisHash, "--expected-evm-chain-id", "964"}
	if code := runMain(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("runtime snapshot exit %d: %s", code, stderr.String())
	}
	var envelope runtimeSnapshotEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	snapshot := envelope.Snapshot
	if snapshot.Schema != runtimeSnapshotSchema || snapshot.Admission != "unapproved_observation" || snapshot.Identity.FinalizedHash != testFinalizedHash || snapshot.Identity.EvmChainId != 964 || snapshot.Version.StateVersion != 1 || artifactReads.Load() != 3 {
		t.Fatalf("runtime snapshot lost pinned scope: %+v reads=%d", snapshot, artifactReads.Load())
	}
	code, err := hex.DecodeString(snapshot.CodeHex[2:])
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := hex.DecodeString(snapshot.MetadataHex[2:])
	if err != nil {
		t.Fatal(err)
	}
	codeDigest := blake2b.Sum256(code)
	metadataDigest := blake2b.Sum256(metadata)
	if snapshot.CodeHash != "0x"+hex.EncodeToString(codeDigest[:]) || snapshot.MetadataHash != "0x"+hex.EncodeToString(metadataDigest[:]) {
		t.Fatal("snapshot hashes do not reproduce from retained artifact bytes")
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(append([]byte(runtimeSnapshotSchema+"\x00"), encoded...))
	if envelope.ContentHash != "sha256:"+hex.EncodeToString(digest[:]) {
		t.Fatal("snapshot content hash does not reproduce")
	}
}

// A testnet route cannot spend time downloading artifacts under mainnet pins.
func TestRuntimeSnapshotRejectsWrongChainBeforeArtifactReads(t *testing.T) {
	server, artifactReads := runtimeSnapshotTestServer(t, "0x3b1", false, false)
	var stdout, stderr bytes.Buffer
	args := []string{"runtime-snapshot", "--rpc", server.URL, "--expected-chain", "Bittensor", "--expected-genesis", testGenesisHash, "--expected-evm-chain-id", "964"}
	if code := runMain(context.Background(), args, &stdout, &stderr); code != 3 || stdout.Len() != 0 || artifactReads.Load() != 0 {
		t.Fatalf("wrong chain got exit %d, output %d bytes, artifact reads %d: %s", code, stdout.Len(), artifactReads.Load(), stderr.String())
	}
}

// A server-reported code hash cannot certify different raw Wasm bytes.
func TestRuntimeSnapshotRejectsCodeHashMismatch(t *testing.T) {
	server, _ := runtimeSnapshotTestServer(t, "0x3c4", true, false)
	var stdout, stderr bytes.Buffer
	if code := runMain(context.Background(), []string{"runtime-snapshot", "--rpc", server.URL}, &stdout, &stderr); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "code bytes differ") {
		t.Fatalf("code mismatch got exit %d and output %q: %s", code, stdout.String(), stderr.String())
	}
}

// A proxy retarget after artifact reads cannot publish the old route's bytes.
func TestRuntimeSnapshotRejectsRetargetedGenesis(t *testing.T) {
	server, _ := runtimeSnapshotTestServer(t, "0x3c4", false, true)
	var stdout, stderr bytes.Buffer
	if code := runMain(context.Background(), []string{"runtime-snapshot", "--rpc", server.URL}, &stdout, &stderr); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "canonical block changed") {
		t.Fatalf("route retarget got exit %d and output %q: %s", code, stdout.String(), stderr.String())
	}
}

// Observation retains unknown metadata for review instead of claiming that an
// old decoder can authorize a new runtime.
func TestRuntimeSnapshotRetainsUnknownMetadataForReview(t *testing.T) {
	server, _ := runtimeSnapshotTestServer(t, "0x3c4", false, false)
	var stdout, stderr bytes.Buffer
	if code := runMain(context.Background(), []string{"runtime-snapshot", "--rpc", server.URL}, &stdout, &stderr); code != 0 {
		t.Fatalf("unknown metadata was discarded: %d %s", code, stderr.String())
	}
	var envelope runtimeSnapshotEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope.Snapshot.MetadataHex != "0x6d657461ff00" {
		t.Fatalf("unknown metadata did not survive raw capture: %v %q", err, envelope.Snapshot.MetadataHex)
	}
}
