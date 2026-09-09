// Exact receipt identity must survive the real ethclient JSON decoder before
// canonical height/hash checks may admit a transaction as finalized.
package main

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Only transport data changes; the production finality reader remains real.
func evmReceiptIdentityTestReader(t *testing.T, fault string) (ethEVMReceiptFinalityReader, common.Hash, *atomic.Uint32) {
	t.Helper()
	hash := common.Hash{0x11}
	receipt := &types.Receipt{Status: types.ReceiptStatusSuccessful, TxHash: hash, BlockHash: common.Hash{0x20}, BlockNumber: big.NewInt(20), Logs: []*types.Log{}}
	switch fault {
	case "transaction":
		receipt.TxHash[0] ^= 1
	case "zero-transaction":
		receipt.TxHash = common.Hash{}
	case "block":
		receipt.BlockHash = common.Hash{}
	case "overflow":
		receipt.BlockNumber = new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(20))
	case "zero-number":
		receipt.BlockNumber = new(big.Int)
	case "missing-number":
		receipt.BlockNumber = nil
	}
	count := new(atomic.Uint32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var call struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&call) != nil {
			http.Error(w, "invalid fixture request", 400)
			return
		}
		var result any
		switch call.Method {
		case "eth_getBlockByNumber":
			var selector string
			if len(call.Params) != 2 || json.Unmarshal(call.Params[0], &selector) != nil {
				http.Error(w, "invalid block request", 400)
				return
			}
			if selector == "finalized" {
				result = map[string]any{"number": "0x15", "hash": common.Hash{0x21}}
			} else if selector == "0x14" {
				result = map[string]any{"number": "0x14", "hash": common.Hash{0x20}}
			} else {
				http.Error(w, "unexpected block", 400)
				return
			}
		case "eth_getTransactionReceipt":
			var requested common.Hash
			if len(call.Params) != 1 || json.Unmarshal(call.Params[0], &requested) != nil || requested != hash {
				http.Error(w, "wrong requested hash", 400)
				return
			}
			count.Add(1)
			result = receipt
		default:
			http.Error(w, "unexpected method", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result})
	}))
	t.Cleanup(server.Close)
	client, err := ethclient.Dial(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return ethEVMReceiptFinalityReader{client: client}, hash, count
}

// A wrong returned hash was previously accepted when its block was canonical.
func TestEVMFinalityAuthenticatesRequestedTransactionIdentity(t *testing.T) {
	for _, fault := range []string{"transaction", "zero-transaction"} {
		reader, hash, count := evmReceiptIdentityTestReader(t, fault)
		observed, ready, err := observeEVMReceiptFinality(t.Context(), reader, hash)
		if err == nil || observed != nil || ready || count.Load() != 1 || !strings.Contains(err.Error(), "requested transaction") {
			t.Fatalf("canonical receipt for another transaction was admitted: fault%s ready%t error%v", fault, ready, err)
		}
	}
}

// Full-width inclusion identity is validated before numerical conversion.
func TestEVMFinalityReceiptIdentityRejectsMalformedInclusion(t *testing.T) {
	for _, fault := range []string{"block", "overflow", "zero-number", "missing-number"} {
		reader, hash, count := evmReceiptIdentityTestReader(t, fault)
		observed, ready, err := observeEVMReceiptFinality(t.Context(), reader, hash)
		if err == nil || observed != nil || ready || count.Load() != 1 {
			t.Fatalf("malformed receipt inclusion was admitted: fault%s ready%t error%v", fault, ready, err)
		}
	}
}

// The real adapter must still admit the exact transaction, not blanket-refuse.
func TestEVMFinalityReceiptIdentityPreservesCanonicalSuccess(t *testing.T) {
	reader, hash, count := evmReceiptIdentityTestReader(t, "")
	observed, ready, err := observeEVMReceiptFinality(t.Context(), reader, hash)
	if err != nil || !ready || observed == nil || observed.TxHash != hash || count.Load() != 1 {
		t.Fatalf("exact canonical receipt refused: %v", err)
	}
}
