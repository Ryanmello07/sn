// An independent observer must not copy a live sender's lock or nonce turn.
// Explicit ownership barriers reproduce the previous whole-struct copy.
package main

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
)

// Holding the original mutex makes an accidental copied mutex observable
// without a blocked goroutine, a sleep or an RPC request.
func TestEvmTxManagerReadCloneDoesNotCopyActiveNonceState(t *testing.T) {
	t.Parallel()
	originalClient := new(ethclient.Client)
	independentClient := new(ethclient.Client)
	manager := &EvmTxManager{
		client:       originalClient,
		chainID:      big.NewInt(17),
		deploymentID: "synthetic-read-view",
		stateDir:     t.TempDir(),
		journal:      &Journal{},
		key:          &ecdsa.PrivateKey{},
	}
	release, err := manager.acquireNonceTurn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	manager.stateLock.Lock()
	defer manager.stateLock.Unlock()

	observed := cloneReadManager(manager, independentClient)
	if observed == nil || observed == manager || observed.client != independentClient {
		t.Fatal("read view does not own its independent client reference")
	}
	if !observed.stateLock.TryLock() {
		t.Fatal("read view copied the active sender mutex")
	}
	observed.stateLock.Unlock()
	if observed.nonceTurn != nil || len(manager.nonceTurn) != 1 {
		t.Fatal("read view copied or changed the active sender nonce turn")
	}
	if observed.chainID != manager.chainID || observed.deploymentID != manager.deploymentID ||
		observed.stateDir != manager.stateDir || observed.journal != manager.journal || observed.key != manager.key {
		t.Fatal("read view lost immutable postcondition metadata")
	}
	if manager.client != originalClient {
		t.Fatal("read view replaced the sender transport")
	}
	if manager.stateLock.TryLock() {
		manager.stateLock.Unlock()
		t.Fatal("read view released the original sender mutex")
	}
}

// Every funded role and deposit reader follows the same ownership boundary,
// including while each original role has an outstanding account turn.
func TestEvmTxManagerIndependentReadExecutorKeepsAllAccountOwners(t *testing.T) {
	t.Parallel()
	originalClient := new(ethclient.Client)
	independentClient := new(ethclient.Client)
	newManager := func() *EvmTxManager {
		manager := &EvmTxManager{client: originalClient}
		release, err := manager.acquireNonceTurn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		manager.stateLock.Lock()
		t.Cleanup(func() {
			manager.stateLock.Unlock()
			release()
		})
		return manager
	}
	executor := &Executor{
		deployer:       newManager(),
		owner:          newManager(),
		guardian:       newManager(),
		oracle:         newManager(),
		keeper:         newManager(),
		deposits:       map[int]*EvmTxManager{1: newManager(), 2: newManager()},
		independentEVM: independentClient,
	}
	observed := executor.independentReadExecutor()
	for _, pair := range []struct {
		original *EvmTxManager
		observed *EvmTxManager
	}{
		{original: executor.deployer, observed: observed.deployer},
		{original: executor.owner, observed: observed.owner},
		{original: executor.guardian, observed: observed.guardian},
		{original: executor.oracle, observed: observed.oracle},
		{original: executor.keeper, observed: observed.keeper},
		{original: executor.deposits[1], observed: observed.deposits[1]},
		{original: executor.deposits[2], observed: observed.deposits[2]},
	} {
		if pair.observed == nil || pair.observed == pair.original || pair.observed.client != independentClient {
			t.Fatal("independent executor reused an account owner or its transport")
		}
		if !pair.observed.stateLock.TryLock() {
			t.Fatal("independent executor copied an active account mutex")
		}
		pair.observed.stateLock.Unlock()
		if pair.observed.nonceTurn != nil || len(pair.original.nonceTurn) != 1 || pair.original.client != originalClient {
			t.Fatal("independent executor copied or mutated active account state")
		}
	}
}

// Optional roles stay absent rather than becoming apparently usable readers.
func TestEvmTxManagerReadClonePreservesMissingRoles(t *testing.T) {
	t.Parallel()
	client := new(ethclient.Client)
	if cloneReadManager(nil, client) != nil {
		t.Fatal("missing role became a read manager")
	}
	executor := &Executor{independentEVM: client, deposits: map[int]*EvmTxManager{1: nil}}
	observed := executor.independentReadExecutor()
	if observed.deployer != nil || observed.owner != nil || observed.guardian != nil ||
		observed.oracle != nil || observed.keeper != nil || len(observed.deposits) != 1 || observed.deposits[1] != nil {
		t.Fatal("independent executor invented a missing account owner")
	}
}
