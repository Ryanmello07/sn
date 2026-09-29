// Concrete adapter regressions use only local synthetic native RPC responses.
package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// The independently encoded native headers commit the actual trim signature.
// Subnet storage is deliberately unavailable to exercise financial retention.
func newOwnerTrimReceiptTestFixture(t *testing.T) (*ownerTrimCanonicalChain, *rootReceiptFixture, []byte) {
	t.Helper()
	native, fixture := newRootReceiptFixture(t, 2, false)
	f := newOwnerTrimActionTestFixture(t)
	f.config.Action.BirthHash = fixture.byHeight[f.config.Action.BirthBlock]
	var err error
	f.config.Action, err = prepareOwnerTrimAction(f.config.Action, f.metadata)
	if err != nil {
		t.Fatal(err)
	}
	f.config.Route.RpcUrl = native.client.url
	f.approve()
	payload, _ := hex.DecodeString(f.config.Action.Payload[2:])
	signature, err := f.pair.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := f.config.Action.signed(signature)
	if err != nil {
		t.Fatal(err)
	}
	action := f.config.Action
	parent := action.BirthHash
	for height := action.BirthBlock + 1; height <= action.BirthBlock+2; height++ {
		body := [][]byte{}
		if height == action.BirthBlock+1 {
			body = append(body, []byte{8, 4, 0}, raw)
		}
		header, hash := rootReceiptHeaderFixture(t, parent, height, body, false)
		fixture.headers[hash], fixture.byHeight[height], fixture.bodies[hash] = header, hash, []string{}
		for _, extrinsic := range body {
			fixture.bodies[hash] = append(fixture.bodies[hash], "0x"+hex.EncodeToString(extrinsic))
		}
		parent = hash
	}
	fixture.finalized = parent
	payer, _ := hex.DecodeString(action.Coldkey[2:])
	events := append([]byte{8}, rootReceiptEventFixture(t, fixture.metadata, "TransactionPayment.TransactionFeePaid", 1, payer, binary.LittleEndian.AppendUint64(nil, 17), make([]byte, 8))...)
	events = append(events, rootReceiptEventFixture(t, fixture.metadata, "System.ExtrinsicSuccess", 1)...)
	key, err := types.CreateStorageKey(fixture.metadata, "System", "Events")
	if err != nil {
		t.Fatal(err)
	}
	fixture.storageKVs[key.Hex()] = "0x" + hex.EncodeToString(events)
	fixture.fault = func(method string, _ []json.RawMessage, _ int) (any, bool) {
		if method == "system_version" {
			return "synthetic-receipt-node", true
		}
		return nil, false
	}
	_, _, policy := newSubnetFixture(t)
	policy.NativeChain, policy.GenesisHash, policy.EvmChainId = action.Network.NativeChain, action.Network.GenesisHash, action.Network.EvmChainId
	policy.RuntimeSourceCommit, policy.RuntimeVersion, policy.RuntimeCodeHash, policy.RuntimeMetadataHash = action.Runtime.RuntimeSourceCommit, action.Runtime.RuntimeVersion, action.Runtime.RuntimeCodeHash, action.Runtime.RuntimeMetadataHash
	policy.SubnetOwnerColdkey, policy.SubnetRegistrationBlock, policy.SubnetGeneration = action.Coldkey, new(action.SubnetRegistrationBlock), new(action.SubnetGeneration)
	chain, err := newOwnerTrimCanonicalChain(f.config, f.key, policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	return chain, fixture, raw
}

// Census loss must retain exact fee/index/dispatch and refuse all network writes.
func TestOwnerTrimReceiptCanonicalFinancialOutcomeSurvivesCensusGap(t *testing.T) {
	chain, fixture, raw := newOwnerTrimReceiptTestFixture(t)
	result, err := chain.reconcile(t.Context(), chain.config.Action, raw)
	if err != nil || result.Receipt == nil || !result.Receipt.Success || result.Receipt.ExtrinsicIndex != 1 || result.Receipt.ActualFeeRao != 17 ||
		result.Census == nil || result.Census.Issue == "" || result.Census.Correspondence != nil {
		t.Fatalf("exact trim receipt was lost or invented post-state: %+v %v", result, err)
	}
	if err := chain.submit(t.Context(), chain.config, raw); err == nil || fixture.counts["author_submitExtrinsic"] != 0 {
		t.Fatal("read-only receipt adapter inferred live submission authority")
	}
}

// A changed body, event phase or canonical anchor cannot settle this action.
func TestOwnerTrimReceiptRejectsBodyEventAndCanonicalConflicts(t *testing.T) {
	for _, kind := range []string{"body", "phase", "canonical"} {
		chain, fixture, raw := newOwnerTrimReceiptTestFixture(t)
		switch kind {
		case "body":
			fixture.bodies[fixture.byHeight[chain.config.Action.BirthBlock+1]] = []string{}
		case "phase":
			key, err := types.CreateStorageKey(fixture.metadata, "System", "Events")
			if err != nil {
				t.Fatal(err)
			}
			events, _ := hex.DecodeString(fixture.storageKVs[key.Hex()][2:])
			binary.LittleEndian.PutUint32(events[2:6], 0)
			fixture.storageKVs[key.Hex()] = "0x" + hex.EncodeToString(events)
		case "canonical":
			fixture.fault = func(method string, params []json.RawMessage, _ int) (any, bool) {
				if method == "system_version" {
					return "synthetic-receipt-node", true
				}
				if method == "chain_getBlockHash" && string(params[0]) != "0" {
					return "0x" + strings.Repeat("d9", 32), true
				}
				return nil, false
			}
		}
		if result, err := chain.reconcile(t.Context(), chain.config.Action, raw); err == nil || result.Receipt != nil {
			t.Fatalf("%s became a canonical receipt: %+v %v", kind, result, err)
		}
	}
}

// Complete original census and bounded predicates are observed through the same
// constructor-owned local route. No mainnet identity or external key is present.
func newOwnerTrimWindowTestFixture(t *testing.T) (*ownerTrimCanonicalChain, *rootRpcFixture, ownerTrimObservation) {
	t.Helper()
	_, fixture, policy, window := newOwnerTrimBoundedFixture(t)
	fixture.set(t, "NetworkImmunityPeriod", binary.LittleEndian.AppendUint64(nil, 98))
	server := bootstrapReadinessTestServer(t, fixture)
	f := newOwnerTrimActionTestFixture(t)
	a := f.config.Action
	a.Network = planNetwork{NativeChain: policy.NativeChain, GenesisHash: policy.GenesisHash, EvmChainId: policy.EvmChainId}
	a.Runtime = rootReceiptProfile{RuntimeSourceCommit: policy.RuntimeSourceCommit, RuntimeVersion: policy.RuntimeVersion, RuntimeCodeHash: policy.RuntimeCodeHash, RuntimeMetadataHash: policy.RuntimeMetadataHash}
	a.Coldkey, a.SubnetRegistrationBlock, a.SubnetGeneration, a.PolicyHash = policy.SubnetOwnerColdkey, *policy.SubnetRegistrationBlock, *policy.SubnetGeneration, window.PolicyHash
	a.Nonce, a.BirthBlock, a.BirthHash, a.Period = 0, window.FinalizedNumber, window.FinalizedHash, window.MortalPeriod
	var err error
	f.config.Action, err = prepareOwnerTrimAction(a, fixture.metadataHex)
	if err != nil {
		t.Fatal(err)
	}
	f.config.Route.RpcUrl = server.URL
	f.approve()
	chain, err := newOwnerTrimCanonicalChain(f.config, f.key, policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	census, err := chain.client.readSubnetPreview(t.Context(), policy, window.PolicyHash)
	if err != nil {
		t.Fatal(err)
	}
	sealed, _ := sealSubnetPreview(census)
	return chain, fixture, ownerTrimObservation{FinalizedNumber: 100, FinalizedHash: testFinalizedHash, AccountNonce: new(uint32(0)), Census: &sealed}
}

// Immunity uses a strict exclusive bound: expiry at death is safe, expiry at
// the last possible inclusion block is not. Neither grants privileged authority.
func TestOwnerTrimAuthorityCurrentPruningBoundaryNeverGrantsEnforcement(t *testing.T) {
	chain, fixture, observation := newOwnerTrimWindowTestFixture(t)
	window, err := chain.readCurrentWindow(t.Context(), chain.config.Action, observation)
	if err != nil || !window.Qualification.ConditionalSafeSet || !window.PublicPruningFenced || !window.OwnerProxiesAbsent || len(window.Blockers) != 0 || window.CurrentAuthorityVerified || len(window.RequiredEnforcement) == 0 {
		t.Fatalf("reviewed current predicates differ: %+v %v", window, err)
	}
	action := chain.config.Action
	authority := ownerTrimCurrentAuthority{chain: chain, baseline: observation.Census.Observation}
	evidence := ownerTrimActionReconciliation{Observation: observation, AnchorHash: action.BirthHash, CheckedFrom: action.BirthBlock + 1, CheckedThrough: action.BirthBlock}
	if err := authority.authorize(t.Context(), chain.config, evidence); err == nil {
		t.Fatal("conditional window predicates promoted into execution authority")
	}
	fixture.set(t, "NetworkImmunityPeriod", binary.LittleEndian.AppendUint64(nil, 97))
	window, err = chain.readCurrentWindow(t.Context(), action, observation)
	if err != nil || window.PublicPruningFenced || len(window.Blockers) == 0 || window.NetworkImmuneUntil != 107 {
		t.Fatalf("last inclusion boundary allowed public subnet pruning: %+v %v", window, err)
	}
}

// Same-hash contradictions and nonce drift are integrity failures, not new plans.
func TestOwnerTrimAuthorityRejectsSemanticCensusAndNonceConflicts(t *testing.T) {
	for _, kind := range []string{"generation", "nonce", "proxy"} {
		chain, fixture, observation := newOwnerTrimWindowTestFixture(t)
		switch kind {
		case "generation":
			fixture.set(t, "BlockAtRegistration", binary.LittleEndian.AppendUint64(nil, 91), []byte{25, 0}, []byte{2, 0})
		case "nonce":
			owner, _ := hex.DecodeString(chain.config.Action.Coldkey[2:])
			key, err := types.CreateStorageKey(fixture.metadata, "System", "Account", owner)
			if err != nil {
				t.Fatal(err)
			}
			row := make([]byte, 56)
			binary.LittleEndian.PutUint32(row, 1)
			fixture.storageKVs[key.Hex()] = "0x" + hex.EncodeToString(row)
		case "proxy":
			owner, _ := hex.DecodeString(chain.config.Action.Coldkey[2:])
			key, err := types.CreateStorageKey(fixture.metadata, "Proxy", "Proxies", owner)
			if err != nil {
				t.Fatal(err)
			}
			fixture.storageKVs[key.Hex()] = "0x04" + strings.Repeat("11", 32) + "0000000000" + strings.Repeat("00", 8)
		}
		window, err := chain.readCurrentWindow(t.Context(), chain.config.Action, observation)
		if kind == "proxy" {
			if err != nil || window.OwnerProxiesAbsent || len(window.Blockers) == 0 {
				t.Fatalf("active delegation was not an explicit blocker: %+v %v", window, err)
			}
		} else if err == nil {
			t.Fatalf("%s conflict admitted current authority", kind)
		}
	}
}

// Cancellation or absent authority never resolves unknown chain facts positively.
func TestOwnerTrimAuthorityCancellationAndOwnedRouteRefusal(t *testing.T) {
	chain, _, observation := newOwnerTrimWindowTestFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := chain.readCurrentWindow(ctx, chain.config.Action, observation); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled observation did not remain unresolved", err)
	}
	changed := chain.config
	changed.Route.RpcUrl = "http://192.0.2.3:9944"
	if err := chain.submit(t.Context(), changed, []byte{1}); err == nil {
		t.Fatal("unapproved owned route accepted a direct submission")
	}
}
