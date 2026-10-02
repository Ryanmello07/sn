package miner

// Reviewed catalogs select only artifacts consumed by an operation. The local
// endpoint supplies observations, never approval; no test broadcasts a write.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/urfoundation/sn/crv4"
)

// Raw JSON keeps the old-source causal test compile-valid before v2 exists.
func writeFleetRuntimeCatalogFixture(t *testing.T, fixture *fleetMainnetTestFixture) {
	t.Helper()
	old, current := fixture.version, fixture.version
	current.SpecVersion++
	entries := []any{}
	for _, version := range []crv4.RuntimeVersionIdentity{old, current} {
		entries = append(entries, map[string]any{
			"runtime_source_commit": fixture.authority.RuntimeSourceCommit, "runtime_review_sha256": fixture.authority.RuntimeReviewSha256,
			"runtime_version": version, "runtime_code_hash": fixture.authority.RuntimeCodeHash, "runtime_metadata_hash": fixture.authority.RuntimeMetadataHash,
			"purposes": []string{"fleet-commitment-read-v1", "fleet-frontier-read-v1"},
		})
	}
	raw, err := json.Marshal(map[string]any{"schema": "urnetwork-mainnet-fleet-runtime-authority-v2", "native_chain": fixture.authority.NativeChain, "genesis_hash": fixture.authority.GenesisHash, "evm_chain_id": fixture.authority.EvmChainId, "netuid": fixture.authority.Netuid, "coordinator": fixture.authority.Coordinator, "runtime_catalog": entries})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(fleetOpt(fixture.opts, "--mainnet-runtime-authority"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	fixture.opts["--mainnet-runtime-authority-sha256"] = hex.EncodeToString(digest[:])
	fixture.stateLock.Lock()
	fixture.historicalVersions[fixture.head.Hex()] = old
	fixture.version = current
	fixture.finalizedNumber = 103
	fixture.stateLock.Unlock()
}

func TestFleetRuntimeCatalogCurrentAndHistoricalReads(t *testing.T) {
	fixture := newFleetMainnetTestFixture(t)
	writeFleetRuntimeCatalogFixture(t, fixture)
	authority, err := loadFleetMainnetRuntimeAuthority(fixture.opts, fixture.manifest)
	if err != nil {
		t.Fatal("reviewed multi-artifact read authority was refused", err)
	}
	chain, err := crv4.DialChainContext(t.Context(), fixture.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.API.Client.Close()
	previousMeta, previousRuntime := chain.Meta, chain.Runtime
	current, block, err := authority.finalizedView(t.Context(), chain)
	if err != nil || block != fixture.nativeBlocks[103] || current.Runtime.SpecVersion != 8002 {
		t.Fatal("reviewed current selection failed", block, err)
	}
	old, err := authority.viewAt(t.Context(), chain, fixture.head)
	if err != nil || old.Runtime.SpecVersion != 8001 {
		t.Fatal("reviewed historical selection failed", err)
	}
	if chain.Meta != previousMeta || chain.Runtime != previousRuntime || current.Runtime.SpecVersion != 8002 {
		t.Fatal("historical selection mutated a concurrent current view")
	}
	if _, err = authority.commitmentFinalized(t.Context(), chain, fixture.manifest.Netuid, fixture.manifest.Hotkey); err != nil {
		t.Fatal("current readable capability did not continue", err)
	}
	if fixture.count("author_submitAndWatchExtrinsic") != 0 || fixture.count("eth_sendRawTransaction") != 0 {
		t.Fatal("read-only catalog test sent a transaction")
	}
}

func TestFleetRuntimeCatalogPublicStatusHonorsCancellation(t *testing.T) {
	fixture := newFleetMainnetTestFixture(t)
	ctx, cancel := context.WithCancel(fixture.durable.Context)
	cancel()
	err := fleetStatus(ctx, fixture.opts, fixture.manifest)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("public fleet status discarded caller cancellation", err)
	}
	fixture.stateLock.Lock()
	defer fixture.stateLock.Unlock()
	if len(fixture.calls) != 0 {
		t.Fatal("canceled public status reached the endpoint", fixture.calls)
	}
}
