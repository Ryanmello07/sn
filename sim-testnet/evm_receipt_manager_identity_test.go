// Real signed manager Send/recovery paths keep malformed endpoint observations
// out of the durable journal; no finality verdict or storage call is mocked.
package main

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Results include independently reloaded hash-chain bytes, not only an in-memory
// projection. Panics are captured so original nil-height controls fail a test root.
type evmManagerReceiptIdentityResult struct {
	receipt      *types.Receipt
	err          error
	panicValue   any
	entries      []JournalEntry
	expectedHash common.Hash
	receiptCalls uint64
}

// A receipt may become visible between nonce admission and the first receipt
// lookup. Both fresh and resumed Send use real RLP persistence and the real journal.
func runEVMManagerReceiptIdentity(t *testing.T, fault string, resume bool) evmManagerReceiptIdentityResult {
	t.Helper()
	stateDir := filepath.Join(t.TempDir(), "state")
	journal, err := OpenJournal(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := journal.Close(); err != nil {
			t.Error(err)
		}
	})
	key, err := crypto.HexToECDSA(strings.Repeat("6", 64))
	if err != nil {
		t.Fatal(err)
	}
	to := common.Address{0x17}
	calldata := []byte{1, 2, 3}
	transaction, err := types.SignTx(types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(945), Nonce: 4, GasTipCap: big.NewInt(2), GasFeeCap: big.NewInt(10), Gas: 55000, To: &to, Value: new(big.Int), Data: calldata}), types.LatestSignerForChainID(big.NewInt(945)), key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := transaction.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	action := Action{ID: "receipt-identity", Kind: "evm-transaction", IntentHash: "receipt-identity-intent", Parameters: map[string]string{evmMaximumGasUnitsParameter: "100000", evmMaximumFeePerGasParameter: "10"}, Spend: Spend{EVMGasWei: DecimalUint("1000000")}}
	const deploymentID, planHash = "receipt-identity-deployment", "receipt-identity-plan"
	if err := journal.Append(JournalEntry{DeploymentID: deploymentID, PlanHash: planHash, ActionID: action.ID, IntentHash: action.IntentHash, Stage: StageIntent}); err != nil {
		t.Fatal(err)
	}
	rawPath := filepath.Join(stateDir, "transactions", strings.TrimPrefix(transaction.Hash().Hex(), "0x")+".rlp")
	if resume {
		if err := atomicWrite(rawPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := journal.Append(JournalEntry{DeploymentID: deploymentID, PlanHash: planHash, ActionID: action.ID, IntentHash: action.IntentHash, Stage: StageBroadcast, Signer: crypto.PubkeyToAddress(key.PublicKey).Hex(), Nonce: "4", TransactionHash: transaction.Hash().Hex(), RecoveryBlock: 21, RecoveryBlockHash: common.Hash{0x21}.Hex()}); err != nil {
			t.Fatal(err)
		}
		if err := journal.Close(); err != nil {
			t.Fatal(err)
		}
		journal, err = OpenJournal(stateDir)
		if err != nil {
			t.Fatal(err)
		}
	}
	receipt := &types.Receipt{Type: transaction.Type(), Status: types.ReceiptStatusSuccessful, TxHash: transaction.Hash(), BlockNumber: big.NewInt(20), BlockHash: common.Hash{0x20}, GasUsed: 40000, CumulativeGasUsed: 40000, EffectiveGasPrice: big.NewInt(6), Logs: []*types.Log{}}
	switch fault {
	case "transaction":
		receipt.TxHash[0] ^= 1
	case "overflow":
		receipt.BlockNumber = new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(20))
	case "missing-number":
		receipt.BlockNumber = nil
	case "zero-number":
		receipt.BlockNumber = new(big.Int)
	case "zero-block":
		receipt.BlockHash = common.Hash{}
	}
	var stateLock sync.Mutex
	var receiptCalls uint64
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var call struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&call) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		var result any
		switch call.Method {
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_maxPriorityFeePerGas":
			result = "0x2"
		case "eth_estimateGas":
			result = "0x61a8"
		case "eth_getBalance":
			result = hexutil.EncodeBig(big.NewInt(1000000000000000000))
		case "eth_getBlockByNumber":
			var selector string
			if len(call.Params) != 2 || json.Unmarshal(call.Params[0], &selector) != nil {
				http.Error(w, "invalid block request", 400)
				return
			}
			switch selector {
			case "latest":
				result = &types.Header{Number: big.NewInt(21), Difficulty: new(big.Int), GasLimit: 30000000, Time: 1700000021, BaseFee: big.NewInt(4), Extra: []byte{}}
			case "finalized":
				result = map[string]any{"number": "0x15", "hash": common.Hash{0x21}}
			case "0x14":
				result = map[string]any{"number": "0x14", "hash": common.Hash{0x20}}
			default:
				http.Error(w, "unexpected block", 400)
				return
			}
		case "eth_getTransactionReceipt":
			var hash common.Hash
			if len(call.Params) != 1 || json.Unmarshal(call.Params[0], &hash) != nil || (hash != transaction.Hash() && hash != receipt.TxHash) {
				http.Error(w, "unexpected transaction", 400)
				return
			}
			stateLock.Lock()
			receiptCalls++
			stateLock.Unlock()
			result = receipt
		default:
			http.Error(w, "unexpected method "+call.Method, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result})
	}))
	t.Cleanup(endpoint.Close)
	client, err := ethclient.Dial(endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	manager := &EvmTxManager{client: client, chainID: big.NewInt(945), deploymentID: deploymentID, stateDir: stateDir, journal: journal, key: key}
	result := evmManagerReceiptIdentityResult{expectedHash: transaction.Hash()}
	func() {
		defer func() { result.panicValue = recover() }()
		result.receipt, result.err = manager.Send(t.Context(), planHash, action, &to, new(big.Int), calldata)
	}()
	result.entries, err = readJournalEntries(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	durable, err := os.ReadFile(rawPath)
	if err != nil || !bytes.Equal(durable, raw) {
		t.Fatalf("manager did not preserve the real signed transaction: %v", err)
	}
	stateLock.Lock()
	result.receiptCalls = receiptCalls
	stateLock.Unlock()
	return result
}

// The journal already has an independent hash fence. The manager must also
// refuse malformed authority before returning an endpoint-supplied receipt.
func TestEVMManagerFreshAndRecoveredWrongReceiptIsRefused(t *testing.T) {
	for _, resume := range []bool{false, true} {
		result := runEVMManagerReceiptIdentity(t, "transaction", resume)
		if result.panicValue != nil || result.err == nil || result.receipt != nil || result.receiptCalls != 1 || len(result.entries) != 2 || result.entries[1].Stage != StageBroadcast {
			t.Fatalf("manager exposed substituted receipt authority: resume%t panic%v error%v receipt%v entries%d", resume, result.panicValue, result.err, result.receipt, len(result.entries))
		}
	}
}

// A 65-bit height used to narrow into a durable Included entry before later
// refusal; a nil height could panic. Neither needs scheduling or polling luck.
func TestEVMManagerMalformedInclusionNeverMutatesDurableJournal(t *testing.T) {
	for _, resume := range []bool{false, true} {
		for _, fault := range []string{"overflow", "missing-number", "zero-number", "zero-block"} {
			result := runEVMManagerReceiptIdentity(t, fault, resume)
			if result.panicValue != nil || result.err == nil || result.receipt != nil || len(result.entries) != 2 || result.entries[1].Stage != StageBroadcast {
				t.Fatalf("malformed manager receipt reached durable journal: resume%t fault%s panic%v error%v entries%d", resume, fault, result.panicValue, result.err, len(result.entries))
			}
		}
	}
}

// Real first-send preparation and exact-RLP restart both retain the canonical
// successful receipt and all four independently reloaded journal stages.
func TestEVMManagerCanonicalFreshAndRecoveredReceiptStillFinalizes(t *testing.T) {
	for _, resume := range []bool{false, true} {
		result := runEVMManagerReceiptIdentity(t, "", resume)
		if result.panicValue != nil || result.err != nil || result.receipt == nil || result.receipt.TxHash != result.expectedHash || result.receiptCalls != 2 || len(result.entries) != 4 {
			t.Fatalf("canonical manager receipt failed: resume%t panic%v error%v calls%d entries%d", resume, result.panicValue, result.err, result.receiptCalls, len(result.entries))
		}
		for index, stage := range []JournalStage{StageIntent, StageBroadcast, StageIncluded, StageFinalized} {
			entry := result.entries[index]
			if entry.Stage != stage || (index > 0 && entry.TransactionHash != result.expectedHash.Hex()) {
				t.Fatalf("canonical manager journal differs at %d", index)
			}
		}
	}
}
