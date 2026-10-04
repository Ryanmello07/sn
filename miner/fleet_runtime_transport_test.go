// Real local Rpc calls force replacement around production identity and
// closing reads. Runtime and wallet inputs remain original fixture authority.
package miner

import (
	"context"
	"sync/atomic"
	"testing"

	gsrpcclient "github.com/centrifuge/go-substrate-rpc-client/v4/client"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/urfoundation/sn/crv4"
)

// The adapter changes only the transport generation after an actual Rpc read.
// It cannot supply an artifact, decoded response, signing result or verdict.
type fleetRuntimeTransportTestClient struct {
	gsrpcclient.Client
	generation atomic.Uint64
	after      func(string, []any)
}

// Reads the same independent generation contract as the production transport.
func (self *fleetRuntimeTransportTestClient) TransportGeneration() uint64 {
	return self.generation.Load()
}

// The owned calling goroutine observes the exact completed-response boundary.
func (self *fleetRuntimeTransportTestClient) CallContext(ctx context.Context, target any, method string, args ...any) error {
	err := self.Client.CallContext(ctx, target, method, args...)
	if err == nil && self.after != nil {
		self.after(method, args)
	}
	return err
}

// A reconnect inside the artifact read repeats the enclosing network census;
// matching runtime code cannot authenticate a replacement's different genesis.
func TestFleetRuntimeReconnectCannotBorrowEarlierNetworkIdentity(t *testing.T) {
	fixture := newFleetMainnetTestFixture(t)
	native, err := crv4.DialChainContext(t.Context(), fixture.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(native.API.Client.Close)
	client := &fleetRuntimeTransportTestClient{Client: native.API.Client}
	client.generation.Store(1)
	native.API.Client = client
	faulted, networks := false, 0
	client.after = func(method string, _ []any) {
		if method == "system_chain" {
			networks++
		}
		if method == "state_getStorageHash" && !faulted {
			faulted = true
			client.generation.Add(1)
			fixture.stateLock.Lock()
			fixture.genesis = types.Hash{0xf2}
			fixture.stateLock.Unlock()
		}
	}
	metadata, runtime := native.Meta, native.Runtime
	artifact, err := fixture.authority.authenticateAt(t.Context(), native, fixture.head)
	if err == nil || artifact.Metadata != nil || !faulted || networks != 2 || native.Meta != metadata || native.Runtime != runtime {
		t.Fatalf("fleet replacement borrowed the earlier network identity: networks=%d artifact=%+v err=%v", networks, artifact, err)
	}
}

// The late reconnect used to return an expired artifact or make binding fail
// without retrying the role's earlier identity reads. It now returns one view.
func TestFleetRuntimeClosingReconnectReturnsFreshOwnedView(t *testing.T) {
	fixture := newFleetMainnetTestFixture(t)
	native, err := crv4.DialChainContext(t.Context(), fixture.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(native.API.Client.Close)
	client := &fleetRuntimeTransportTestClient{Client: native.API.Client}
	client.generation.Store(1)
	native.API.Client = client
	canonicalReads, networks := 0, 0
	client.after = func(method string, args []any) {
		if method == "system_chain" {
			networks++
		}
		if method == "chain_getBlockHash" && args[0] != uint64(0) {
			canonicalReads++
			if canonicalReads == 2 {
				client.generation.Add(1)
			}
		}
	}
	metadata, runtime := native.Meta, native.Runtime
	view, err := fixture.authority.viewAt(t.Context(), native, fixture.head)
	if err != nil || view == nil || view == native || networks != 2 || canonicalReads != 4 ||
		view.Runtime.SpecVersion != types.U32(fixture.authority.RuntimeVersion.SpecVersion) || native.Meta != metadata || native.Runtime != runtime {
		t.Fatalf("fleet closing reconnect failed the complete owned read: networks=%d canonical=%d view=%+v err=%v", networks, canonicalReads, view, err)
	}
}
