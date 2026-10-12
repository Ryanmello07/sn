//go:build runtime_rehearsal

package rehearsal

import (
	"math/big"
	"strings"
	"testing"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/urfoundation/sn/merkle"
	"github.com/urfoundation/sn/miner/onchain"
)

// batchAccounts is the number of accounts R-7's root holds: enough for the
// single claims, the fixed batches and more than one block's batch.
const batchAccounts = 450

// claimItem is STClaimBatcher.Claim.
type claimItem struct {
	NoId      *big.Int
	RootIndex *big.Int
	Account   account
	Proof     []proofNode
}

// R-7. TestR7ClaimBatch measures the claim batcher (VERSION2-FINAL.md §10.2,
// §15.8) against single claims, on a ledger of 2^29 accounts: the block gas
// limit; for single claims the gas used, the gas limit eth_estimateGas gives
// and the least gas limit that pays; for batches of 1, 5, 10 and 20 claims
// and for the most one block holds, the gas used and the least gas limit at
// which the batch attempts every item. Each batch caps each claim at the
// tooling's default gas per claim, is sent with exactly its least limit, and
// pays every item.
func TestR7ClaimBatch(t *testing.T) {
	t.Parallel()
	n := startNode(t)
	s := deployStack(t, n, stackConfig{operators: 1})
	operator := s.operators[1]
	// Epochs stop (tempo 0), so no dividend reaches a position between reads.
	n.sudo(t, n.call(t, "AdminUtils.sudo_set_tempo", types.NewU16(s.netuid), types.NewU16(0)))
	relayer := newEVMKey(t)
	n.fundEVM(t, relayer.address, 10_000*tao)
	batcherArtifact := forgeArtifact(t, "src/STClaimBatcher.sol", "STClaimBatcher")
	batcher := n.deploy(t, relayer, batcherArtifact.creation)

	// Epoch 1 funds the accounts of root 0.
	n.stakeFor(t, alice, s.vaultColdkey, operator.pool, s.netuid, 1_000*tao)
	s.close(t, 1, 1)
	funded := s.vaultUint(t, "fundedThrough", id(1), id(1)).Uint64()
	accounts := make([]merkle.LedgerAccount, batchAccounts)
	for i := range accounts {
		share := funded / batchAccounts
		if i == len(accounts)-1 {
			share = funded - share*(batchAccounts-1)
		}
		accounts[i] = ledgerAccount(newAccount(t), share)
	}
	l := newLedger(t, accounts, deepLeafCount)
	committed := l.commitment("ledger header epoch 1")
	n.send(t, operator.rootSigner, s.settlement, s.settlementArtifact, nil, "commitDistribution", id(1), id(1), id(0), committed)
	n.send(t, s.auditor, s.settlement, s.settlementArtifact, nil, "approveDistribution", id(1), uint64(1), committed.Root, committed.LedgerHash)
	n.sealUntil(t, s.pending(t, 1).CommittedBlock+challengeBlocks)
	n.send(t, relayer, s.settlement, s.settlementArtifact, nil, "finalizeDistribution", id(1))

	header, err := n.eth.HeaderByNumber(n.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A transaction may name less gas than the block gas limit: the pool
	// refuses more than the normal dispatch class's extrinsic weight limit.
	// Found to the gas with value transfers, it must be at least the
	// tooling's onchain.ClaimBatchMaxGas, which sizes batches.
	accepted := func(gas uint64) bool {
		err := n.sendTx(t, relayer, &relayer.address, nil, gas)
		if err != nil && !strings.Contains(err.Error(), "exceeds block gas limit") {
			t.Fatalf("a transfer naming %d gas: %v", gas, err)
		}
		return err == nil
	}
	low, high := onchain.ClaimBatchMaxGas, header.GasLimit+1
	if !accepted(low) || accepted(high) {
		t.Fatalf("transfers naming %d and %d gas", low, high)
	}
	for high-low > 1 {
		if middle := (low + high) / 2; accepted(middle) {
			low = middle
		} else {
			high = middle
		}
	}
	maxGas := onchain.ClaimBatchMaxGas
	t.Logf("block gas limit %d; most gas one transaction may name %d; batches sized at %d", header.GasLimit, low, maxGas)

	// least returns the least gas limit, to the gas, at which a transaction
	// of data from the relayer to `to` satisfies ok in an eth_call.
	least := func(to common.Address, data []byte, high uint64, ok func([]byte, error) bool) uint64 {
		low := uint64(21_000)
		if !ok(n.eth.CallContract(n.ctx, ethereum.CallMsg{From: relayer.address, To: &to, Data: data, Gas: high}, nil)) {
			t.Fatalf("the call fails with %d gas", high)
		}
		for high-low > 1 {
			middle := (low + high) / 2
			if ok(n.eth.CallContract(n.ctx, ethereum.CallMsg{From: relayer.address, To: &to, Data: data, Gas: middle}, nil)) {
				high = middle
			} else {
				low = middle
			}
		}
		return high
	}
	attempted := func(out []byte, err error) uint64 {
		if err != nil {
			return 0
		}
		return batcherArtifact.unpack(t, "claimMany", out)[0].(*big.Int).Uint64()
	}
	// paid requires account index's claimed watermark at its cumulative.
	paid := func(t *testing.T, index uint64) {
		if got := s.vaultUint(t, "claimed", id(1), id(index)); got.Cmp(l.account(index).Cumulative) != 0 {
			t.Fatalf("account %d claimed %s of %s", index, got, l.account(index).Cumulative)
		}
	}
	next := uint64(0)

	// Single claims: eth_estimateGas's limit, the least limit that pays and
	// the gas used. The pool's first claim also writes its first claimed
	// and paid totals.
	var singleUsed []uint64
	var singleEstimate, singleLeast uint64
	for range 5 {
		a := l.account(next)
		data := s.vaultArtifact.pack(t, "claim", id(1), id(0), a, l.proof(next))
		estimate, err := n.estimate(relayer.address, s.vault, nil, data)
		if err != nil {
			t.Fatalf("estimate a claim: %s", reason(err))
		}
		exact := least(s.vault, data, estimate, func(_ []byte, err error) bool { return err == nil })
		receipt := n.transact(t, relayer, &s.vault, nil, data, exact)
		if receipt.Status != gethtypes.ReceiptStatusSuccessful {
			t.Fatalf("a claim sent with its least limit %d failed", exact)
		}
		paid(t, next)
		singleUsed, singleEstimate, singleLeast = append(singleUsed, receipt.GasUsed), max(singleEstimate, estimate), max(singleLeast, exact)
		next++
	}
	t.Logf("single claims: used %v, eth_estimateGas limit up to %d, least limit up to %d", singleUsed, singleEstimate, singleLeast)
	// The least limit covers the transaction's intrinsic gas besides the
	// claim's own, so a cap at the default leaves the claim at least as much.
	if singleLeast > onchain.ClaimBatchGasPerClaim {
		t.Fatalf("a single claim's least limit %d exceeds the default gas per claim %d", singleLeast, onchain.ClaimBatchGasPerClaim)
	}

	// batch sends the next count accounts with the least limit that attempts
	// all of them and requires each to pay.
	batch := func(t *testing.T, count uint64, high uint64) (used, limit uint64) {
		items := make([]claimItem, count)
		for i := range items {
			index := next + uint64(i)
			items[i] = claimItem{NoId: id(1), RootIndex: id(0), Account: l.account(index), Proof: l.proof(index)}
		}
		data := batcherArtifact.pack(t, "claimMany", s.vault, items, id(onchain.ClaimBatchGasPerClaim))
		limit = least(batcher, data, high, func(out []byte, err error) bool { return attempted(out, err) == count })
		receipt := n.transact(t, relayer, &batcher, nil, data, limit)
		if receipt.Status != gethtypes.ReceiptStatusSuccessful {
			t.Fatalf("a batch of %d sent with its least limit %d failed", count, limit)
		}
		paidEvent := batcherArtifact.abi.Events["ClaimPaid"]
		events := 0
		for _, log := range receipt.Logs {
			if log.Address != batcher {
				continue
			}
			if log.Topics[0] != paidEvent.ID {
				t.Fatalf("batch of %d: event %s", count, log.Topics[0])
			}
			values, err := paidEvent.Inputs.Unpack(log.Data)
			if err != nil || values[0].(*big.Int).Uint64() != uint64(events) || values[1].(*big.Int).Cmp(items[events].Account.Cumulative) != 0 {
				t.Fatalf("batch of %d: ClaimPaid %v (%v), want item %d", count, values, err, events)
			}
			events++
		}
		if events != int(count) {
			t.Fatalf("batch of %d: %d ClaimPaid events", count, events)
		}
		for i := range count {
			paid(t, next+i)
		}
		next += count
		s.requireConservation(t)
		return receipt.GasUsed, limit
	}
	for _, count := range []uint64{1, 5, 10, 20} {
		used, limit := batch(t, count, maxGas)
		t.Logf("batch of %d: used %d (%d per claim), least limit %d", count, used, used/count, limit)
	}

	// The most one transaction holds: every remaining account offered at the
	// most gas a transaction may name, then exactly the attempted prefix sent.
	remaining := batchAccounts - next
	items := make([]claimItem, remaining)
	for i := range items {
		index := next + uint64(i)
		items[i] = claimItem{NoId: id(1), RootIndex: id(0), Account: l.account(index), Proof: l.proof(index)}
	}
	most := attempted(n.eth.CallContract(n.ctx, ethereum.CallMsg{From: relayer.address, To: &batcher,
		Data: batcherArtifact.pack(t, "claimMany", s.vault, items, id(onchain.ClaimBatchGasPerClaim)), Gas: maxGas}, nil))
	if most == 0 || most >= remaining {
		t.Fatalf("one transaction holds a batch of %d of the %d accounts left", most, remaining)
	}
	used, limit := batch(t, most, maxGas)
	t.Logf("batch of %d, the most one transaction holds: used %d (%d per claim), least limit %d of %d", most, used, used/most, limit, maxGas)
}
