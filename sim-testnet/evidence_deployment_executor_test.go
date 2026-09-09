// Exercises real Executor admission and signed transaction recovery through
// the existing journal, HTTP RPC client and canonical synthetic EVM hash domain.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// Construction freezes domains and responses. Only transaction inclusion is
// mutable, under stateLock; inclusion advances from block 100 to block 101.
type validatorEvidenceExecutorRPC struct {
	base          *validatorEvidenceInstallRPC
	owner         common.Address
	nonce         uint64
	initialAnchor common.Address
	stateLock     sync.Mutex
	transaction   *types.Transaction
	committed     bool
	sendHook      func()
	rpcCalls      atomic.Uint64
	nonceCalls    atomic.Uint64
	receiptCalls  atomic.Uint64
	sendCalls     atomic.Uint64
}

// Both block hashes are explicit RPC identities, not Header.Hash substitutes.
func (self *validatorEvidenceExecutorRPC) block(selector finalEVMBlockSelector) (uint64, error) {
	if !selector.RequireCanonical {
		return 0, errors.New("noncanonical evidence executor selector")
	}
	if selector.BlockHash == self.base.head.Hash {
		return self.base.head.Number, nil
	}
	self.stateLock.Lock()
	committed := self.committed
	self.stateLock.Unlock()
	inclusion := testEVMHead(101, 0xef)
	if committed && selector.BlockHash == inclusion.Hash {
		return inclusion.Number, nil
	}
	return 0, errors.New("foreign evidence executor block hash")
}

func (self *validatorEvidenceExecutorRPC) GetCode(_ context.Context, address common.Address, selector finalEVMBlockSelector) (hexutil.Bytes, error) {
	self.rpcCalls.Add(1)
	if _, err := self.block(selector); err != nil {
		return nil, err
	}
	if address != self.base.address {
		return nil, errors.New("foreign evidence code address")
	}
	return bytes.Clone(self.base.code), nil
}

func (self *validatorEvidenceExecutorRPC) Call(_ context.Context, message ValidatorEvidenceInstallRPCMessage, selector finalEVMBlockSelector) (hexutil.Bytes, error) {
	self.rpcCalls.Add(1)
	block, err := self.block(selector)
	if err != nil {
		return nil, err
	}
	key := validatorEvidenceInstallCall{address: message.To, data: hexutil.Encode(message.Data)}
	if key == self.base.fail {
		return nil, errors.New("fixture exact getter failure")
	}
	value, exists := self.base.responses[key]
	if !exists {
		return nil, errors.New("foreign evidence getter")
	}
	if key.data == hexutil.Encode(crypto.Keccak256([]byte("validatorEvidence()"))[:4]) {
		anchor := self.initialAnchor
		if block == 101 {
			anchor = self.base.address
		}
		return abiWordAddress(anchor), nil
	}
	return bytes.Clone(value), nil
}

func (self *validatorEvidenceExecutorRPC) GetBlockByNumber(_ context.Context, selector string, full bool) (any, error) {
	self.rpcCalls.Add(1)
	if full {
		return nil, errors.New("unexpected full block request")
	}
	if selector == "latest" {
		return &types.Header{Number: big.NewInt(100), Difficulty: new(big.Int), BaseFee: big.NewInt(1), GasLimit: 30_000_000}, nil
	}
	self.stateLock.Lock()
	committed := self.committed
	self.stateLock.Unlock()
	head := self.base.head
	if selector == "finalized" && committed || selector == "0x65" && committed {
		head = testEVMHead(101, 0xef)
	} else if selector != "finalized" && selector != "0x64" {
		return nil, errors.New("foreign numbered block request")
	}
	return &evmRPCBlock{Number: hexutil.EncodeUint64(head.Number), Hash: head.Hash}, nil
}

func (self *validatorEvidenceExecutorRPC) GetTransactionCount(_ context.Context, address common.Address, selector string) (hexutil.Uint64, error) {
	self.rpcCalls.Add(1)
	self.nonceCalls.Add(1)
	if address != self.owner || selector != "pending" && selector != "0x64" {
		return 0, errors.New("foreign nonce owner or block")
	}
	return hexutil.Uint64(self.nonce), nil
}

func (self *validatorEvidenceExecutorRPC) MaxPriorityFeePerGas(context.Context) (*hexutil.Big, error) {
	self.rpcCalls.Add(1)
	return (*hexutil.Big)(big.NewInt(1)), nil
}

func (self *validatorEvidenceExecutorRPC) EstimateGas(_ context.Context, _ json.RawMessage) (hexutil.Uint64, error) {
	self.rpcCalls.Add(1)
	return 50_000, nil
}

func (self *validatorEvidenceExecutorRPC) GetBalance(_ context.Context, address common.Address, selector string) (*hexutil.Big, error) {
	self.rpcCalls.Add(1)
	if address != self.owner || selector != "latest" {
		return nil, errors.New("foreign balance owner")
	}
	return (*hexutil.Big)(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)), nil
}

func (self *validatorEvidenceExecutorRPC) SendRawTransaction(_ context.Context, wire hexutil.Bytes) (common.Hash, error) {
	self.rpcCalls.Add(1)
	self.sendCalls.Add(1)
	var transaction types.Transaction
	if err := transaction.UnmarshalBinary(wire); err != nil {
		return common.Hash{}, err
	}
	self.stateLock.Lock()
	self.transaction = &transaction
	self.stateLock.Unlock()
	if self.sendHook != nil {
		self.sendHook()
	}
	return transaction.Hash(), nil
}

func (self *validatorEvidenceExecutorRPC) GetTransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	self.rpcCalls.Add(1)
	self.receiptCalls.Add(1)
	self.stateLock.Lock()
	defer self.stateLock.Unlock()
	if self.transaction == nil {
		return nil, nil
	}
	if self.transaction.Hash() != hash {
		return nil, errors.New("receipt queried for another transaction")
	}
	self.committed = true
	head := testEVMHead(101, 0xef)
	return &types.Receipt{Status: types.ReceiptStatusSuccessful, TxHash: hash, BlockNumber: new(big.Int).SetUint64(head.Number), BlockHash: common.HexToHash(head.Hash), GasUsed: 60_000, CumulativeGasUsed: 60_000, Logs: []*types.Log{}, EffectiveGasPrice: big.NewInt(3)}, nil
}

// Register the complete transport fixture, then close the client and join HTTP
// handlers before stopping its RPC server.
func (self *validatorEvidenceExecutorRPC) client(t *testing.T) *ethclient.Client {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("eth", self); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Stop)
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	client, err := ethclient.DialContext(context.Background(), httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

// Uses the actual generated plan, deterministic test owner key and private
// journal. No wallet, external RPC, transaction-manager stub or global hook.
func validatorEvidenceExecutorTest(t *testing.T, configure func(*validatorEvidenceExecutorRPC)) (*Executor, Action, *validatorEvidenceExecutorRPC) {
	t.Helper()
	cfg, secrets, payloads := validatorEvidenceInstallTest(t)
	roles, err := derivePublicRoles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	facts := *testSetupFacts()
	facts.DeployerNonce = payloads.Manifest.InitialNonce
	plan, err := buildPlan(cfg, &facts, roles, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePlanBudget(plan); err != nil {
		t.Fatal(err)
	}
	if err := validateValidatorEvidenceDeployment(plan.ValidatorEvidence, payloads.ValidatorEvidence); err != nil {
		t.Fatal(err)
	}
	role, err := secrets.EVMKey("testnet-owner")
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.HexToECDSA(role.PrivateKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &validatorEvidenceExecutorRPC{base: validatorEvidenceInstallRPCTest(t, payloads.ValidatorEvidence), owner: crypto.PubkeyToAddress(key.PublicKey), nonce: 7}
	if configure != nil {
		configure(fixture)
	}
	client := fixture.client(t)
	stateDir := filepath.Join(t.TempDir(), "owner")
	journal, err := OpenJournal(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := journal.Close(); err != nil {
			t.Error(err)
		}
	})
	manager := &EvmTxManager{client: client, chainID: new(big.Int).SetUint64(testnetChainID), deploymentID: plan.DeploymentID, stateDir: stateDir, journal: journal, key: key}
	executor := &Executor{cfg: cfg, roles: secrets, plan: plan, payloads: payloads, stateDir: stateDir, journal: journal, owner: manager, deployer: &EvmTxManager{client: client}}
	return executor, actionByID(t, plan, validatorEvidenceAnchorActionID), fixture
}

// Persists a real signed RLP and journal row, then closes/reopens the journal;
// recovered authority must come from the approved action, not that row's labels.
func persistValidatorEvidenceAnchorTest(t *testing.T, executor *Executor, action Action, transaction *types.Transaction) {
	t.Helper()
	wire, err := transaction.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(executor.stateDir, "transactions", stringsTrim0x(transaction.Hash().Hex())+".rlp")
	if err := atomicWrite(path, wire, 0o600); err != nil {
		t.Fatal(err)
	}
	var transactionSigner types.Signer = types.HomesteadSigner{}
	if transaction.Protected() {
		transactionSigner = types.LatestSignerForChainID(transaction.ChainId())
	}
	signer, err := types.Sender(transactionSigner, transaction)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.journal.Append(JournalEntry{DeploymentID: executor.plan.DeploymentID, PlanHash: executor.plan.PlanHash, ActionID: action.ID, IntentHash: action.IntentHash, Stage: StageBroadcast, Signer: signer.Hex(), Nonce: strconv.FormatUint(transaction.Nonce(), 10), TransactionHash: transaction.Hash().Hex(), RecoveryBlock: 100, RecoveryBlockHash: testEVMHead(100, 0xee).Hash}); err != nil {
		t.Fatal(err)
	}
	if err := executor.journal.Close(); err != nil {
		t.Fatal(err)
	}
	journal, err := OpenJournal(executor.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := journal.Close(); err != nil {
			t.Error(err)
		}
	})
	executor.journal, executor.owner.journal = journal, journal
}

// Signs exact bytes with independent test input mutations; the production
// transaction approval helper is deliberately not used to construct this wire.
func validatorEvidenceAnchorSignedTest(t *testing.T, executor *Executor, fixture *validatorEvidenceExecutorRPC, fault string) *types.Transaction {
	t.Helper()
	to := executor.payloads.Manifest.CoordinatorProxy
	data := bytes.Clone(executor.payloads.ValidatorEvidence.Anchor)
	value := new(big.Int)
	key := executor.owner.key
	chain := new(big.Int).Set(executor.owner.chainID)
	protected := true
	switch fault {
	case "owner":
		var err error
		key, err = crypto.ToECDSA(common.LeftPadBytes([]byte{9}, 32))
		if err != nil {
			t.Fatal(err)
		}
	case "target":
		to[19] ^= 1
	case "value":
		value.SetUint64(1)
	case "calldata":
		data[len(data)-1] ^= 1
	case "chain":
		chain.Add(chain, big.NewInt(1))
	case "unprotected":
		protected = false
	case "":
	default:
		t.Fatalf("unknown signed fixture mutation %s", fault)
	}
	unsigned := types.NewTx(&types.LegacyTx{Nonce: fixture.nonce, To: &to, Value: value, Data: data, Gas: 60_000, GasPrice: big.NewInt(3)})
	var signer types.Signer = types.HomesteadSigner{}
	if protected {
		signer = types.LatestSignerForChainID(chain)
	}
	signed, err := types.SignTx(unsigned, signer, key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestValidatorEvidenceInstallExecutorAnchorPublishesExactOwnerTransaction(t *testing.T) {
	executor, action, fixture := validatorEvidenceExecutorTest(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := executor.anchorValidatorEvidence(ctx, action); err != nil {
		t.Fatal(err)
	}
	fixture.stateLock.Lock()
	transaction, committed := fixture.transaction, fixture.committed
	fixture.stateLock.Unlock()
	if transaction == nil || !committed || fixture.sendCalls.Load() != 1 || transaction.Nonce() != 7 || !transaction.Protected() || transaction.ChainId().Cmp(executor.owner.chainID) != 0 {
		t.Fatal("owner transaction was not broadcast and finalized exactly once")
	}
	signer, err := types.Sender(types.LatestSignerForChainID(executor.owner.chainID), transaction)
	if err != nil || signer != fixture.owner || transaction.To() == nil || *transaction.To() != executor.payloads.Manifest.CoordinatorProxy || transaction.Value().Sign() != 0 || !bytes.Equal(transaction.Data(), executor.payloads.ValidatorEvidence.Anchor) {
		t.Fatalf("actual signed anchor authority differs: %v", err)
	}
	latest, exists := executor.journal.LatestTransaction(executor.plan.PlanHash, action.ID, action.IntentHash)
	if !exists || latest.Stage != StageFinalized || latest.TransactionHash != transaction.Hash().Hex() || latest.BlockHash != testEVMHead(101, 0xef).Hash {
		t.Fatalf("actual finality was not journalled: %+v", latest)
	}
}

func TestValidatorEvidenceInstallExecutorAnchorResumesExactSignedJournal(t *testing.T) {
	executor, action, fixture := validatorEvidenceExecutorTest(t, nil)
	signed := validatorEvidenceAnchorSignedTest(t, executor, fixture, "")
	persistValidatorEvidenceAnchorTest(t, executor, action, signed)
	fixture.stateLock.Lock()
	fixture.transaction = signed
	fixture.stateLock.Unlock()
	if err := executor.anchorValidatorEvidence(context.Background(), action); err != nil {
		t.Fatalf("exact durable owner recovery: %v", err)
	}
	if fixture.sendCalls.Load() != 0 || fixture.nonceCalls.Load() != 0 || fixture.receiptCalls.Load() == 0 {
		t.Fatal("mined recovery allocated a fresh nonce or rebroadcast")
	}
	latest, exists := executor.journal.LatestTransaction(executor.plan.PlanHash, action.ID, action.IntentHash)
	if !exists || latest.Stage != StageFinalized || latest.TransactionHash != signed.Hash().Hex() {
		t.Fatalf("exact recovered transaction was not finalized: %+v", latest)
	}
}

func TestValidatorEvidenceInstallExecutorAnchorRejectsPersistedAuthorityDrift(t *testing.T) {
	for _, fault := range []string{"owner", "target", "value", "calldata", "chain", "unprotected"} {
		executor, action, fixture := validatorEvidenceExecutorTest(t, nil)
		signed := validatorEvidenceAnchorSignedTest(t, executor, fixture, fault)
		if fault == "unprotected" && (signed.Protected() || signed.ChainId().Sign() != 0) {
			t.Fatal("unprotected refusal fixture is not a real chain-independent Homestead envelope")
		}
		if fault == "chain" && (!signed.Protected() || signed.ChainId().Sign() <= 0 || signed.ChainId().Cmp(executor.owner.chainID) == 0) {
			t.Fatal("wrong-chain refusal fixture is not a real protected foreign-chain envelope")
		}
		persistValidatorEvidenceAnchorTest(t, executor, action, signed)
		fixture.stateLock.Lock()
		fixture.transaction = signed
		fixture.stateLock.Unlock()
		before := executor.journal.Entries()
		err := executor.anchorValidatorEvidence(context.Background(), action)
		if err == nil || !strings.Contains(err.Error(), "persisted") || fixture.receiptCalls.Load() != 0 || fixture.nonceCalls.Load() != 0 || fixture.sendCalls.Load() != 0 {
			t.Fatalf("%s persisted anchor reached transaction recovery: %v receipts=%d nonces=%d sends=%d", fault, err, fixture.receiptCalls.Load(), fixture.nonceCalls.Load(), fixture.sendCalls.Load())
		}
		if len(executor.journal.Entries()) != len(before) {
			t.Fatalf("%s refusal changed the durable transaction history", fault)
		}
	}
}

func TestValidatorEvidenceInstallExecutorAnchorAlreadyLinkedIsReadOnly(t *testing.T) {
	executor, action, fixture := validatorEvidenceExecutorTest(t, func(fixture *validatorEvidenceExecutorRPC) { fixture.initialAnchor = fixture.base.address })
	if err := executor.anchorValidatorEvidence(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	if fixture.sendCalls.Load() != 0 || fixture.nonceCalls.Load() != 0 || fixture.receiptCalls.Load() != 0 || len(executor.journal.Entries()) != 0 {
		t.Fatal("already linked exact companion entered a transaction path")
	}
}

func TestValidatorEvidenceInstallExecutorAnchorRejectsForeignPrestateBeforeTransaction(t *testing.T) {
	for _, fault := range []string{"anchor", "runtime", "getter"} {
		executor, action, fixture := validatorEvidenceExecutorTest(t, func(fixture *validatorEvidenceExecutorRPC) {
			switch fault {
			case "anchor":
				fixture.initialAnchor = common.HexToAddress("0x1111111111111111111111111111111111111111")
			case "runtime":
				fixture.base.code[0] ^= 1
			case "getter":
				key := validatorEvidenceInstallCall{address: fixture.base.address, data: hexutil.Encode(crypto.Keccak256([]byte("netuid()"))[:4])}
				fixture.base.responses[key][31] ^= 1
			}
		})
		if err := executor.anchorValidatorEvidence(context.Background(), action); err == nil {
			t.Fatalf("%s prestate was accepted", fault)
		}
		if fixture.sendCalls.Load() != 0 || fixture.nonceCalls.Load() != 0 || fixture.receiptCalls.Load() != 0 || len(executor.journal.Entries()) != 0 {
			t.Fatalf("%s prestate reached a transaction boundary", fault)
		}
	}
}

func TestValidatorEvidenceInstallExecutorAnchorRejectsActionDriftBeforeRPC(t *testing.T) {
	for _, fault := range []string{"owner", "target", "calldata", "manifest", "key"} {
		executor, action, fixture := validatorEvidenceExecutorTest(t, nil)
		action.Parameters = cloneStrings(action.Parameters)
		switch fault {
		case "owner":
			action.Parameters["anchor_owner"] = executor.plan.Roles.Deployer
		case "target":
			action.Target = executor.payloads.Manifest.SettlementVault.Hex()
		case "calldata":
			action.Parameters["anchor_data_keccak256"] = common.Hash{}.Hex()
		case "manifest":
			changed := *executor.plan.ValidatorEvidence
			changed.Address[19] ^= 1
			executor.plan.ValidatorEvidence = &changed
		case "key":
			key, err := crypto.ToECDSA(common.LeftPadBytes([]byte{9}, 32))
			if err != nil {
				t.Fatal(err)
			}
			executor.owner.key = key
		}
		if err := executor.anchorValidatorEvidence(context.Background(), action); err == nil {
			t.Fatalf("%s action was accepted", fault)
		}
		if fixture.rpcCalls.Load() != 0 || len(executor.journal.Entries()) != 0 {
			t.Fatalf("%s action drift escaped pre-RPC admission", fault)
		}
	}
}

func TestValidatorEvidenceInstallExecutorAnchorCancellationAfterBroadcastRetainsOnlyRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executor, action, fixture := validatorEvidenceExecutorTest(t, func(fixture *validatorEvidenceExecutorRPC) { fixture.sendHook = cancel })
	err := executor.anchorValidatorEvidence(ctx, action)
	if !errors.Is(err, context.Canceled) || fixture.sendCalls.Load() != 1 {
		t.Fatalf("post-broadcast cancellation lost its boundary: error=%v sends=%d", err, fixture.sendCalls.Load())
	}
	fixture.stateLock.Lock()
	committed, signed := fixture.committed, fixture.transaction
	fixture.stateLock.Unlock()
	if committed || signed == nil {
		t.Fatal("cancellation fabricated finality or lost the actual signed transaction")
	}
	entries := executor.journal.Entries()
	if len(entries) != 1 || entries[0].Stage != StageBroadcast || entries[0].TransactionHash != signed.Hash().Hex() {
		t.Fatalf("cancellation did not retain exactly one recoverable broadcast: %+v", entries)
	}
	wire, err := os.ReadFile(filepath.Join(executor.stateDir, "transactions", stringsTrim0x(signed.Hash().Hex())+".rlp"))
	if err != nil {
		t.Fatal(err)
	}
	var recovered types.Transaction
	if err := recovered.UnmarshalBinary(wire); err != nil || recovered.Hash() != signed.Hash() {
		t.Fatalf("canceled broadcast RLP is not recoverable: %v", err)
	}
}
