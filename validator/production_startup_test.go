//go:build linux || darwin

// The public root opens real dormant disk stores, authenticates signed
// activation/history, starts actual operator sessions and reconciles native work.
package validator

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

// An unrelated latest metadata/current EVM outage cannot strand an immortal
// original signature. Both receipt and application use actual canonical rows;
// a later preparation request is blocked before it can sign anything new.
func TestProductionStartupRunReleaseReconcilesBeforeCurrentPreparation(t *testing.T) {
	fixture := newProductionStartupTestFixture(t)
	continuation := fixture.continuation
	pending := continuation.beginAndLoseAcknowledgement(t, fixture.native)
	fixture.native.receiptNumber, fixture.native.applied = 103, true
	continuation.production.head = max(uint64(104), pending.Prepared.RevealBlock)
	continuation.production.extrinsicsKVs = map[uint64][]string{103: {pending.Prepared.ExtrinsicHex}}
	continuation.production.epoch++
	continuation.production.operator.blocks[110] = [32]byte{0xa2}
	continuation.production.operator.finalized = 110
	fixture.closePreparation(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunRelease(ctx, fixture.configPath) }()
	var result error
	select {
	case result = <-done:
		t.Fatalf("actual RunRelease stopped before the completed original intent reached current preparation: %v", result)
	case <-fixture.latestRead:
		cancel()
		result = <-done
		t.Fatalf("actual RunRelease queried unrelated latest metadata before retained intent recovery: %v", result)
	case <-fixture.evm.freshRead:
		cancel()
		result = <-done
	case <-t.Context().Done():
		cancel()
		result = <-done
		t.Fatalf("actual RunRelease did not reach its preparation barrier: %v", result)
	}
	if result != nil && !errors.Is(result, context.Canceled) {
		t.Fatalf("public root cancellation invented a terminal integrity failure: %v", result)
	}
	var stored steeringIntentFile
	if err := json.Unmarshal(fixture.storedIntentBytes(t), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Current == nil {
		t.Fatal("public root lost its durable nonempty intent")
	}
	applied := stored.Current
	if applied.Status != "applied" || applied.SubnetEpoch != pending.SubnetEpoch || applied.VectorHash != pending.VectorHash || applied.CreatedAt != pending.CreatedAt || applied.Prepared == nil || applied.Prepared.ExtrinsicHex != pending.Prepared.ExtrinsicHex || applied.FinalizedBlock != 103 || applied.ApplicationBlock == 0 || !slices.Equal(fixture.native.broadcasts, []string{pending.Prepared.ExtrinsicHex}) {
		t.Fatalf("public RunRelease lost original pending receipt/application or signed a replacement: status=%s finalized=%d application=%d sends=%d", applied.Status, applied.FinalizedBlock, applied.ApplicationBlock, len(fixture.native.broadcasts))
	}
	if fixture.latestReads != 0 {
		t.Fatal("historical recovery initialized against unrelated latest metadata")
	}
	for _, api := range fixture.origins {
		api.stateLock.Lock()
		sessions := api.sessions
		api.stateLock.Unlock()
		if sessions == 0 {
			t.Fatal("public root did not establish both actual operator session owners")
		}
	}
}
