// The fixture executes retained vault creation/runtime bytecode in a local EVM.
// Only the runtime precompiles are synthetic; their stake lives in the real
// StateDB journal so an actual Solidity revert rolls back the complete move.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/holiman/uint256"
)

type economicCapturePrecompile struct {
	kind      string
	state     *state.StateDB
	pool      common.Hash
	escrow    common.Hash
	coldkey   common.Hash
	shortfall bool
}

func (self *economicCapturePrecompile) RequiredGas([]byte) uint64 { return 100 }

func (self *economicCapturePrecompile) stake(hotkey common.Hash) *big.Int {
	return self.state.GetState(common.HexToAddress("0x805"), crypto.Keccak256Hash(hotkey[:], self.coldkey[:])).Big()
}

func (self *economicCapturePrecompile) setStake(hotkey common.Hash, amount *big.Int) {
	self.state.SetState(common.HexToAddress("0x805"), crypto.Keccak256Hash(hotkey[:], self.coldkey[:]), common.BigToHash(amount))
}

func (self *economicCapturePrecompile) Run(input []byte) ([]byte, error) {
	if len(input) < 4 {
		return nil, errors.New("synthetic runtime selector absent")
	}
	selector := func(signature string) bool { return bytes.Equal(input[:4], crypto.Keccak256([]byte(signature))[:4]) }
	word := func(index int) common.Hash { return common.BytesToHash(input[4+index*32 : 4+(index+1)*32]) }
	switch {
	case self.kind == "price" && selector("getAlphaPrice(uint16)") && len(input) == 36 && word(0).Big().Cmp(big.NewInt(25)) == 0:
		return common.BigToHash(big.NewInt(1_000_000_000_000_000_000)).Bytes(), nil
	case self.kind == "neuron" && selector("registerLimit(uint16,bytes32,uint64)") && len(input) == 100:
		if word(0).Big().Cmp(big.NewInt(25)) != 0 || word(1) != self.pool && word(1) != self.escrow {
			return nil, errors.New("synthetic original registration identity differs")
		}
		return nil, nil
	case self.kind == "neuron" && selector("getUid(uint16,bytes32)") && len(input) == 68:
		if word(0).Big().Cmp(big.NewInt(25)) != 0 || word(1) != self.pool && word(1) != self.escrow {
			return nil, errors.New("synthetic original UID identity differs")
		}
		return append(common.BigToHash(big.NewInt(1)).Bytes(), common.Hash{}.Bytes()...), nil
	case self.kind == "stake" && selector("getStake(bytes32,bytes32,uint256)") && len(input) == 100:
		if word(1) != self.coldkey || word(2).Big().Cmp(big.NewInt(25)) != 0 || word(0) != self.pool && word(0) != self.escrow {
			return nil, errors.New("synthetic original stake query identity differs")
		}
		return common.BigToHash(self.stake(word(0))).Bytes(), nil
	case self.kind == "stake" && selector("moveStake(bytes32,bytes32,uint256,uint256,uint256)") && len(input) == 164:
		if word(0) != self.pool || word(1) != self.escrow || word(2).Big().Cmp(big.NewInt(25)) != 0 || word(3).Big().Cmp(big.NewInt(25)) != 0 || self.stake(self.pool).Cmp(word(4).Big()) < 0 {
			return nil, errors.New("synthetic original stake movement differs")
		}
		amount := word(4).Big()
		self.setStake(self.pool, new(big.Int).Sub(self.stake(self.pool), amount))
		if self.shortfall {
			amount.Sub(amount, big.NewInt(1))
		}
		self.setStake(self.escrow, new(big.Int).Add(self.stake(self.escrow), amount))
		return nil, nil
	default:
		return nil, errors.New("synthetic runtime call outside exact fixture census")
	}
}

// This fixture-local coordinator forwards the complete calldata to the real
// vault. A signed EOA transaction therefore retains an honest contract caller.
func economicCaptureForwarder(vault common.Address) []byte {
	code := []byte{0x36, 0x60, 0, 0x60, 0, 0x37, 0x60, 0, 0x60, 0, 0x36, 0x60, 0, 0x60, 0, 0x73}
	code = append(code, vault[:]...)
	code = append(code, 0x5a, 0xf1, 0x3d, 0x60, 0, 0x60, 0, 0x3e, 0x60, 0, 0x57, 0x3d, 0x60, 0, 0xfd)
	code[len(code)-6] = byte(len(code))
	return append(code, 0x5b, 0x3d, 0x60, 0, 0xf3)
}

type economicCaptureContractFixture struct {
	transaction *types.Transaction
	receipt     *types.Receipt
	poolAfter   string
	escrowAfter string
}

// Configure before public servers receive any requests. Snapshots and logs
// come from real contract calls, then the existing fixture builds exact tries.
func economicCaptureExecuteContract(t *testing.T, fixture *monitorEvmFixture, amount uint64, revert bool) *economicCaptureContractFixture {
	t.Helper()
	key, err := crypto.HexToECDSA(strings.Repeat("18", 32))
	if err != nil {
		t.Fatal(err)
	}
	origin := crypto.PubkeyToAddress(key.PublicKey)
	pool, escrow, coldkey := common.HexToHash("0x"+strings.Repeat("11", 32)), common.HexToHash("0x"+strings.Repeat("55", 32)), common.HexToHash("0x"+strings.Repeat("33", 32))
	coordinator := common.HexToAddress("0x4321")
	db, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	chain := *params.AllDevChainProtocolChanges
	chain.ChainID = big.NewInt(964)
	config := runtime.Config{ChainConfig: &chain, State: db, Origin: origin, BlockNumber: big.NewInt(10), GasLimit: 15_000_000, GasPrice: big.NewInt(2), Value: new(big.Int), BaseFee: big.NewInt(1)}
	contract := fixture.contract
	var creation []byte
	for _, artifact := range evmTestRelease(t).Artifacts {
		if artifact.Name == "STSettlementVault" {
			creation = common.FromHex(artifact.Creation)
		}
	}
	constructor, err := contract.Pack("", uint16(25), [32]byte(escrow), [32]byte(coldkey), uint64(10), uint64(1), origin)
	if err != nil || len(creation) == 0 {
		t.Fatal("original vault artifact/constructor absent", err)
	}
	code, address, _, err := runtime.Create(append(creation, constructor...), &config)
	if err != nil || len(code) == 0 {
		t.Fatal("actual original vault deployment failed", err)
	}
	db.SetCode(coordinator, economicCaptureForwarder(address), tracing.CodeChangeUnspecified)
	stake := &economicCapturePrecompile{kind: "stake", state: db, pool: pool, escrow: escrow, coldkey: coldkey, shortfall: revert}
	call := func(caller, target common.Address, data []byte, tx common.Hash) ([]byte, uint64, error) {
		config.Origin = caller
		env := runtime.NewEnv(&config)
		rules := chain.Rules(config.BlockNumber, config.Random != nil, config.Time)
		precompiles := vm.ActivePrecompiledContracts(rules)
		precompiles[common.HexToAddress("0x805")] = stake
		precompiles[common.HexToAddress("0x804")] = &economicCapturePrecompile{kind: "neuron", state: db, pool: pool, escrow: escrow, coldkey: coldkey}
		precompiles[common.HexToAddress("0x808")] = &economicCapturePrecompile{kind: "price", state: db, pool: pool, escrow: escrow, coldkey: coldkey}
		env.SetPrecompiles(precompiles)
		db.SetTxContext(tx, 0)
		db.Prepare(rules, caller, common.Address{}, &target, []common.Address{common.HexToAddress("0x804"), common.HexToAddress("0x805"), common.HexToAddress("0x808")}, nil)
		return env.Call(caller, target, data, config.GasLimit, uint256.NewInt(0))
	}
	pack := func(method string, values ...any) []byte {
		data, err := contract.Pack(method, values...)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	for index, setup := range []struct {
		caller common.Address
		data   []byte
	}{{caller: origin, data: pack("setCoordinatorOnce", coordinator)}, {caller: coordinator, data: pack("registerPool", big.NewInt(1), [32]byte(pool), uint64(1))}} {
		if _, _, err := call(setup.caller, address, setup.data, crypto.Keccak256Hash([]byte{byte(index)})); err != nil {
			t.Fatal("actual original vault setup failed", index, err)
		}
	}
	fixture.fixtureGetters = map[string][]any{}
	for _, method := range []string{"selfColdkey", "escrowHotkey", "pools"} {
		args := []any{}
		if method == "pools" {
			args = append(args, big.NewInt(1))
		}
		raw, _, err := call(origin, address, pack(method, args...), crypto.Keccak256Hash([]byte(method)))
		if err != nil {
			t.Fatal("actual original capture identity getter", method, err)
		}
		values, err := contract.Unpack(method, raw)
		if err != nil {
			t.Fatal(err)
		}
		fixture.fixtureGetters[method] = values
	}
	fixture.policy.CaptureIdentity = true
	stake.setStake(pool, new(big.Int).SetUint64(amount))
	readSnapshot := func() monitorEconomicEvmSnapshot {
		result := monitorEconomicEvmSnapshot{Counters: map[string]string{}, Pools: map[string]string{"1": "0"}, Credits: map[string]string{fixture.policy.Coldkeys[0]: "0"}}
		for _, name := range []string{"totalCaptured", "totalPaid", "pendingFunding", "outstandingLiability", "escrowAccounted", "liveEscrowStake"} {
			raw, _, err := call(origin, address, pack(name), crypto.Keccak256Hash([]byte(name)))
			if err != nil {
				t.Fatal("actual original vault getter failed", name, err)
			}
			values, err := contract.Unpack(name, raw)
			if err != nil || len(values) != 1 {
				t.Fatal("actual original getter output differs", name, err)
			}
			result.Counters[name] = values[0].(*big.Int).String()
		}
		return result
	}
	before := readSnapshot()
	beforeRoot := db.IntermediateRoot(true)
	input := pack("captureEmission", big.NewInt(2), big.NewInt(1))
	tx, err := types.SignNewTx(key, types.LatestSignerForChainID(big.NewInt(964)), &types.LegacyTx{Nonce: 11, GasPrice: big.NewInt(2), Gas: config.GasLimit, To: &coordinator, Value: new(big.Int), Data: input})
	if err != nil {
		t.Fatal(err)
	}
	config.BlockNumber = big.NewInt(11)
	_, gas, callErr := call(origin, coordinator, input, tx.Hash())
	if (callErr != nil) != revert {
		t.Fatal("actual original capture success/revert differs", callErr, revert)
	}
	status := uint64(types.ReceiptStatusSuccessful)
	if revert {
		status = types.ReceiptStatusFailed
	}
	logs := db.GetLogs(tx.Hash(), 11, common.Hash{}, 0)
	if logs == nil {
		logs = []*types.Log{}
	}
	receipt := &types.Receipt{Type: types.LegacyTxType, Status: status, CumulativeGasUsed: config.GasLimit - gas, Logs: logs, TxHash: tx.Hash(), GasUsed: config.GasLimit - gas, EffectiveGasPrice: big.NewInt(2), BlockNumber: big.NewInt(11), TransactionIndex: 0}
	after := readSnapshot()
	afterRoot := db.IntermediateRoot(true)
	for _, number := range []uint64{10, 11, 12, 13} {
		block := fixture.blocks[number]
		block.transactions, block.receipts, block.funded = types.Transactions{}, []map[string]any{}, map[string]string{}
		block.header.TxHash, block.header.ReceiptHash = types.EmptyTxsHash, types.EmptyReceiptsHash
		block.header.GasUsed, block.header.Bloom = 0, types.Bloom{}
		block.snapshot, block.header.Root = before.clone(), beforeRoot
		if number >= 11 {
			block.snapshot, block.header.Root = after.clone(), afterRoot
		}
		if number == 11 {
			block.transactions = types.Transactions{tx}
			block.header.TxHash = types.DeriveSha(block.transactions, trie.NewStackTrie(nil))
			block.header.GasUsed = receipt.GasUsed
			block.header.GasLimit = config.GasLimit
			raw, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			fields["from"], fields["to"], fields["contractAddress"] = origin.Hex(), coordinator.Hex(), nil
			block.receipts = []map[string]any{fields}
		}
	}
	fixture.policy.Address, fixture.policy.CodeHash, fixture.code = address.Hex(), crypto.Keccak256Hash(code).Hex(), code
	fixture.policy.FeePayers = []string{origin.Hex()}
	economicConservationTestEvmRehash(t, fixture, nil)
	return &economicCaptureContractFixture{transaction: tx, receipt: receipt, poolAfter: stake.stake(pool).String(), escrowAfter: stake.stake(escrow).String()}
}
