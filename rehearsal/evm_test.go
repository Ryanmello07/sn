//go:build runtime_rehearsal

package rehearsal

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/urfoundation/sn/ss58"
)

var (
	neuronAddress  = common.HexToAddress("0x0000000000000000000000000000000000000804")
	stakingAddress = common.HexToAddress("0x0000000000000000000000000000000000000805")
	alphaAddress   = common.HexToAddress("0x0000000000000000000000000000000000000808")
)

// evmKey is a fresh EVM account.
type evmKey struct {
	private *ecdsa.PrivateKey
	address common.Address
}

func newEVMKey(t *testing.T) evmKey {
	t.Helper()
	private, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return evmKey{private: private, address: crypto.PubkeyToAddress(private.PublicKey)}
}

// mirror is the coldkey the runtime acts for when address calls a precompile.
func mirror(address common.Address) [32]byte {
	return ss58.EvmMirrorPubkey(address)
}

// fundEVM funds address's mapped account; its EVM balance is rao × 10^9 wei.
func (self *node) fundEVM(t *testing.T, address common.Address, rao uint64) {
	t.Helper()
	self.fund(t, mirror(address), rao)
}

// artifact is a contract compiled by forge with the pinned toolchain of
// evm/foundry.toml; creation is empty for an interface.
type artifact struct {
	abi      abi.ABI
	creation []byte
}

// forgeArtifact builds source (relative to evm/) with forge and loads the
// artifact of contract name from evm/out.
func forgeArtifact(t *testing.T, source, name string) artifact {
	t.Helper()
	evm, err := filepath.Abs(filepath.Join("..", "evm"))
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command("forge", "build", source)
	build.Dir = evm
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("forge build %s: %v\n%s", source, err, output)
	}
	raw, err := os.ReadFile(filepath.Join(evm, "out", filepath.Base(source), name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		ABI      json.RawMessage `json:"abi"`
		Bytecode struct {
			Object string `json:"object"`
		} `json:"bytecode"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	contract, err := abi.JSON(bytes.NewReader(parsed.ABI))
	if err != nil {
		t.Fatal(err)
	}
	creation, err := hexutil.Decode(parsed.Bytecode.Object)
	if err != nil {
		t.Fatalf("%s bytecode: %v", name, err)
	}
	return artifact{abi: contract, creation: creation}
}

// pack ABI-encodes a call to method.
func (self artifact) pack(t *testing.T, method string, args ...any) []byte {
	t.Helper()
	data, err := self.abi.Pack(method, args...)
	if err != nil {
		t.Fatalf("pack %s: %v", method, err)
	}
	return data
}

// unpack decodes method's return values.
func (self artifact) unpack(t *testing.T, method string, data []byte) []any {
	t.Helper()
	values, err := self.abi.Unpack(method, data)
	if err != nil {
		t.Fatalf("unpack %s: %v", method, err)
	}
	return values
}

// transact sends a transaction, seals it into its own block and returns its
// receipt, which may report a failure. value may be nil; gas zero takes
// eth_estimateGas.
func (self *node) transact(t *testing.T, from evmKey, to *common.Address, value *big.Int, data []byte, gas uint64) *gethtypes.Receipt {
	t.Helper()
	nonce, err := self.eth.NonceAt(self.ctx, from.address, nil)
	if err != nil {
		t.Fatal(err)
	}
	price, err := self.eth.SuggestGasPrice(self.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if gas == 0 {
		if gas, err = self.eth.EstimateGas(self.ctx, ethereum.CallMsg{From: from.address, To: to, Value: value, Data: data}); err != nil {
			t.Fatalf("estimate gas: %v", err)
		}
	}
	tx, err := gethtypes.SignNewTx(from.private, gethtypes.LatestSignerForChainID(big.NewInt(evmChainID)),
		&gethtypes.LegacyTx{Nonce: nonce, GasPrice: price, Gas: gas, To: to, Value: value, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	if err := self.eth.SendTransaction(self.ctx, tx); err != nil {
		t.Fatalf("send transaction: %v", err)
	}
	self.seal(t)
	// The node indexes Ethereum blocks just after import.
	deadline := time.Now().Add(30 * time.Second)
	for {
		receipt, err := self.eth.TransactionReceipt(self.ctx, tx.Hash())
		if err == nil {
			return receipt
		}
		if !errors.Is(err, ethereum.NotFound) || time.Now().After(deadline) {
			t.Fatalf("receipt of %s: %v", tx.Hash(), err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// deploy runs creation bytecode (with any constructor arguments appended)
// and returns the new contract's address.
func (self *node) deploy(t *testing.T, from evmKey, creation []byte) common.Address {
	t.Helper()
	receipt := self.transact(t, from, nil, nil, creation, 0)
	if receipt.Status != gethtypes.ReceiptStatusSuccessful {
		t.Fatalf("deployment %s failed", receipt.TxHash)
	}
	return receipt.ContractAddress
}

// ethCall runs data against to at the latest block without a transaction.
// eth_call skips EIP-3607, so from may be a contract: a call to a precompile
// from a contract's address then runs as that contract's mapped coldkey,
// which reports the runtime's reason for a refusal.
func (self *node) ethCall(from, to common.Address, data []byte) ([]byte, error) {
	return self.eth.CallContract(self.ctx, ethereum.CallMsg{From: from, To: &to, Data: data}, nil)
}

// stake reads coldkey's alpha on hotkey through the staking precompile's getStake.
func (self *node) stake(t *testing.T, hotkey, coldkey [32]byte, netuid uint16) uint64 {
	t.Helper()
	out, err := self.ethCall(common.Address{}, stakingAddress, self.staking.pack(t, "getStake", hotkey, coldkey, big.NewInt(int64(netuid))))
	if err != nil {
		t.Fatalf("getStake: %v", err)
	}
	stake := self.staking.unpack(t, "getStake", out)[0].(*big.Int)
	if !stake.IsUint64() {
		t.Fatalf("getStake returned %s", stake)
	}
	return stake.Uint64()
}
