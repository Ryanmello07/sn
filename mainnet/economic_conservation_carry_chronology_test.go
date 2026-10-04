// Carry availability comes from the original receipt order. A permissionless
// late finalizer can consume a newer epoch without relabelling its earnings.
package main

import (
	"bytes"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// The contract has one entitlement per epoch/pool, but no numeric ordering
// constraint between different finalizations. All amounts remain explicit.
func economicCarryChronologyState(t *testing.T, kind string) *economicConservationState {
	t.Helper()
	event := func(name string, block uint64, log uint64, values map[string]string) monitorEconomicEvmEvent {
		return monitorEconomicEvmEvent{
			Block:           economicEmissionBoundary{Number: block, Hash: common.BigToHash(new(big.Int).SetUint64(block)).Hex()},
			TransactionHash: common.BigToHash(new(big.Int).SetUint64(1000 + block)).Hex(),
			ReceiptHash:     monitorReadDigest([]byte(name)), LogIndex: log, Name: name, Values: values,
		}
	}
	capture := event("EmissionCaptured", 100, 0, map[string]string{"epoch": "4", "noId": "1", "amount": "20"})
	carry := event("RootMissed", 101, 1, map[string]string{"epoch": "4", "noId": "1", "carried": "20"})
	total := "20"
	source := economicConservationEntitlement{Id: "4/1", Epoch: "4", PoolId: "1", Funded: "20", Claimed: "0", Status: "root-missed", CarryEvent: &carry,
		Sources: []economicConservationBacking{{Kind: "capture", Id: rootObjectHash(capture), Amount: "20"}}}
	if kind == "expired-entitlement" {
		sourceTotal := "20"
		source.Total, source.Claimed, source.Status = &sourceTotal, "7", "carried"
		carry.Name, carry.Values = "EntitlementExpired", map[string]string{"epoch": "4", "noId": "1", "unclaimed": "13"}
		total = "13"
	}
	final := event("EntitlementFinalized", 102, 2, map[string]string{"epoch": "3", "noId": "1", "total": total})
	target := economicConservationEntitlement{Id: "3/1", Epoch: "3", PoolId: "1", Funded: "0", Claimed: "0", Status: "finalized", Total: &total, Finalization: &final,
		Sources: []economicConservationBacking{{Kind: kind, Id: source.Id, Amount: total}}}
	return &economicConservationState{Captures: []economicConservationCapture{{Id: rootObjectHash(capture), Event: capture}}, Entitlements: []economicConservationEntitlement{source, target}}
}

// Both missed roots and expired remainders may already be available when an
// older root finally executes. Their original epoch must survive the join.
func TestEconomicCarryChronologyKeepsNewerSourceForDelayedOlderRoot(t *testing.T) {
	for _, kind := range []string{"root-missed", "expired-entitlement"} {
		state := economicCarryChronologyState(t, kind)
		if err := state.reconcileEntitlementFunding(t.Context()); err != nil {
			t.Fatal("contract-valid delayed finalization rejected original receipt order", kind, err)
		}
		resolver, err := newEconomicFundingResolver(t.Context(), state)
		if err != nil {
			t.Fatal(err)
		}
		value, err := resolver.entitlement("3/1")
		if err != nil || value.Epoch != "3" || value.Pool != "1" || value.Funding.Amount != *state.Entitlements[1].Total || value.Funding.Complete || state.Entitlements[0].Epoch != "4" || state.Entitlements[1].Sources[0].Id != "4/1" {
			t.Fatal("delayed root lost original source identity or invented native income", kind, value, err)
		}
	}
}

// Global log order is authoritative within a block, including two calls in
// one transaction. Equal epoch amounts cannot authorize a future receipt.
func TestEconomicCarryChronologyChecksSameBlockTransactionAndLogOrder(t *testing.T) {
	for _, sameTransaction := range []bool{false, true} {
		state := economicCarryChronologyState(t, "root-missed")
		carry, final := state.Entitlements[0].CarryEvent, state.Entitlements[1].Finalization
		carry.Block = final.Block
		if sameTransaction {
			carry.TransactionHash, carry.ReceiptHash = final.TransactionHash, final.ReceiptHash
		} else {
			final.TransactionIndex = 1
		}
		if err := state.reconcileEntitlementFunding(t.Context()); err != nil {
			t.Fatal("original same-block carry order refused", sameTransaction, err)
		}
	}
	for _, change := range []struct {
		name   string
		mutate func(*monitorEconomicEvmEvent, *monitorEconomicEvmEvent)
	}{
		{name: "later block", mutate: func(carry, final *monitorEconomicEvmEvent) { carry.Block.Number = final.Block.Number + 1 }},
		{name: "different block hash", mutate: func(carry, final *monitorEconomicEvmEvent) { carry.Block.Hash = common.HexToHash("0xffff").Hex() }},
		{name: "same log", mutate: func(carry, final *monitorEconomicEvmEvent) { carry.LogIndex = final.LogIndex }},
		{name: "later log", mutate: func(carry, final *monitorEconomicEvmEvent) { carry.LogIndex = final.LogIndex + 1 }},
		{name: "later transaction", mutate: func(carry, final *monitorEconomicEvmEvent) { carry.TransactionIndex = final.TransactionIndex + 1 }},
		{name: "different same-transaction identity", mutate: func(carry, final *monitorEconomicEvmEvent) { carry.TransactionHash = common.HexToHash("0xffff").Hex() }},
		{name: "different same-transaction receipt", mutate: func(carry, final *monitorEconomicEvmEvent) {
			carry.ReceiptHash = monitorReadDigest([]byte("different-original-receipt"))
		}},
	} {
		state := economicCarryChronologyState(t, "root-missed")
		carry, final := state.Entitlements[0].CarryEvent, state.Entitlements[1].Finalization
		carry.Block, carry.TransactionHash, carry.ReceiptHash = final.Block, final.TransactionHash, final.ReceiptHash
		if err := state.reconcileEntitlementFunding(t.Context()); err != nil {
			t.Fatal("valid original receipt baseline failed before chronology control", change.name, err)
		}
		change.mutate(carry, final)
		if err := state.reconcileEntitlementFunding(t.Context()); err == nil || !strings.Contains(err.Error(), "before original finalization receipt") {
			t.Fatal("unavailable carry acquired original finalization authority", change.name, err)
		}
	}
}

// Use the actual public receipt, artifact, archive and restart owners. Only
// the original source epoch label changes; source amounts and target stay put.
func TestEconomicCarryChronologyPublicDelayedRootRetainsSourceThroughArchive(t *testing.T) {
	var entitlement *economicEntitlementFixture
	f := newEconomicConservationArchiveFixture(t, false, func(source *economicConservationFixture) {
		vault := source.vault
		economicConservationTestEvmRehash(t, vault, func(number uint64, receipt *types.Receipt) {
			if number != 11 {
				return
			}
			for _, index := range []int{0, 1} {
				if len(receipt.Logs[index].Topics) < 2 || receipt.Logs[index].Topics[1] != common.BigToHash(big.NewInt(2)) {
					t.Fatal("carry chronology fixture lost its original source epoch")
				}
				receipt.Logs[index].Topics[1] = common.BigToHash(big.NewInt(4))
			}
		})
		vault.blocks[11].funded["4/1"] = vault.blocks[11].funded["2/1"]
		delete(vault.blocks[11].funded, "2/1")
		entitlement = configureEconomicEntitlementFixture(t, source)
	})
	summary := f.sample(t, monitorServiceHooks{})
	record := economicEntitlementRecord(t, f.source)
	if record.Census == nil || record.Epoch != "3" || record.Funded != "0" || record.Total == nil || *record.Total != "50" || len(record.Sources) != 3 || record.Sources[2].Id != "4/1" || record.Sources[2].Amount != "20" || record.PayoutRoot != common.Hash(entitlement.committedRoot).Hex() || summary.Funding == nil || summary.Funding.OriginalCaptures != 1 || summary.Funding.OriginalClaims != 2 || summary.Funding.OriginalPayments != 1 || summary.TargetMet != nil {
		t.Fatal("public delayed root rejected, relabelled or recounted its original carry", summary, record)
	}
	state := f.source.state(t)
	if err := state.index(); err != nil {
		t.Fatal(err)
	}
	index, found := state.entitlementIds["4/1"]
	if !found || state.Entitlements[index].CarryEvent == nil || state.Entitlements[index].CarryEvent.Values["epoch"] != "4" || state.Entitlements[index].CarryEvent.Block.Number >= record.Finalization.Block.Number {
		t.Fatal("public delayed root lost its earlier receipt from the newer source epoch", state)
	}
	beforeCensus, beforeFunding := record.censusHash(), economicEntitlementFundingHash(record)
	f.reset(t)
	_, args := f.plan(t)
	if code, issue := f.apply(t, args, &bytes.Buffer{}, monitorServiceHooks{}); code != 0 {
		t.Fatal("public archive refused original out-of-order finalization", code, issue)
	}
	retired := f.source.state(t)
	if len(retired.Captures) != 0 || len(retired.Claims) != 0 || retired.Archive == nil || retired.Archive.Counts.Claims != 2 {
		t.Fatal("carry chronology control did not actually retire original source and claims", retired)
	}
	reopened := f.sample(t, monitorServiceHooks{})
	after := economicEntitlementRecord(t, f.source)
	if after.Census != nil || after.CensusReference == nil || after.censusHash() != beforeCensus || economicEntitlementFundingHash(after) != beforeFunding || after.Sources[2].Id != "4/1" || reopened.Funding == nil || reopened.Funding.Captured != summary.Funding.Captured || reopened.Funding.Accepted != summary.Funding.Accepted || reopened.Funding.Paid != summary.Funding.Paid || reopened.Funding.OriginalCaptures != 1 || reopened.Funding.OriginalClaims != 2 || reopened.Funding.OriginalPayments != 1 || reopened.TargetMet != nil {
		t.Fatal("cold delayed root changed original source or repeated economic effects", reopened, after)
	}
}
