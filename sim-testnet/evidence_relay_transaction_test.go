//go:build linux || darwin

// Exercise actual funded sends, journal reopen and permissionless race proofs.
// Explicit endpoint transitions establish ordering, not negative sleep windows.
package main

import (
	"bytes"
	"context"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// The first network broadcast is observed only after both original custody
// records exist. Successful readback exercises actual finality and contract abi.
func TestEvidenceRelayTransactionFreshSendRetainsExactCustody(t *testing.T) {
	fixture := newEvidenceRelayRpcFixture(t, "success")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := fixture.manager.relayValidatorEvidenceTransaction(ctx, fixture.chain, fixture.planHash, fixture.action, fixture.expected)
	if err != nil || result == nil {
		t.Fatalf("actual funded relay: %v", err)
	}
	if result.LostPublicationRace || result.Winner == nil || result.OwnReceipt == nil || result.OwnReceipt.TxHash != fixture.transaction.Hash() || result.Winner.Receipt.TxHash != fixture.transaction.Hash() || !bytes.Equal(result.Winner.SignedTransaction, fixture.transactionBytes) || result.Winner.Publication.Header != fixture.expected.Evidence.Header {
		t.Fatal("fresh send lost exact ownership or publication")
	}
	if fixture.requestCount("eth_sendRawTransaction") != 1 || fixture.requestCount("pending-nonce") != 1 || fixture.requestCount("eth_estimateGas") != 1 || fixture.requestCount("eth_getTransactionByBlockHashAndIndex") == 0 || fixture.requestCount("eth_getCode") == 0 {
		t.Fatal("fresh send skipped funded or independent proof transport")
	}
	entries := fixture.manager.journal.Entries()
	if len(entries) != 4 || entries[0].Stage != StageIntent || entries[1].Stage != StageBroadcast || entries[2].Stage != StageIncluded || entries[3].Stage != StageFinalized {
		t.Fatalf("actual sender custody stages: %+v", entries)
	}
	if entries[1].Signer != fixture.expected.Relayer.Hex() || entries[1].Nonce != "4" || entries[2].Signer != "" || entries[3].Nonce != "" {
		t.Fatal("fixture failed to exercise metadata-free later rows")
	}
	fixture.reopen(t)
	entries = fixture.manager.journal.Entries()
	if len(entries) != 4 || entries[3].TransactionHash != fixture.transaction.Hash().Hex() {
		t.Fatal("disk journal did not retain exact finalized transaction")
	}
}

// The first server accepts the signed request but returns an error. A newly
// opened journal and nonce owner must resend identical bytes without allocation.
func TestEvidenceRelayTransactionRestartResendsOriginalBytes(t *testing.T) {
	fixture := newEvidenceRelayRpcFixture(t, "send-error")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := fixture.manager.relayValidatorEvidenceTransaction(ctx, fixture.chain, fixture.planHash, fixture.action, fixture.expected)
	if err == nil || result != nil || !strings.Contains(err.Error(), "deterministic transport interruption") || fixture.requestCount("eth_sendRawTransaction") != 1 {
		t.Fatalf("first durable send did not interrupt deterministically: %v", err)
	}
	fixture.reopen(t)
	func() { fixture.stateLock.Lock(); defer fixture.stateLock.Unlock(); fixture.mode = "success" }()
	result, err = fixture.manager.relayValidatorEvidenceTransaction(ctx, fixture.chain, fixture.planHash, fixture.action, fixture.expected)
	if err != nil || result == nil || result.Winner.Receipt.TxHash != fixture.transaction.Hash() || result.OwnReceipt == nil {
		t.Fatalf("exact restart: %v", err)
	}
	if fixture.requestCount("eth_sendRawTransaction") != 2 || fixture.requestCount("pending-nonce") != 1 || fixture.requestCount("eth_estimateGas") != 1 {
		t.Fatal("recovery allocated or re-estimated another transaction")
	}
	func() {
		fixture.stateLock.Lock()
		defer fixture.stateLock.Unlock()
		if len(fixture.sentBytes) != 2 || !bytes.Equal(fixture.sentBytes[0], fixture.sentBytes[1]) || !bytes.Equal(fixture.sentBytes[0], fixture.transactionBytes) {
			t.Fatal("restart changed original bytes")
		}
	}()
	broadcasts := 0
	for _, entry := range fixture.manager.journal.Entries() {
		if entry.Stage == StageBroadcast {
			broadcasts++
		}
	}
	if broadcasts != 1 {
		t.Fatalf("restart manufactured %d original broadcasts", broadcasts)
	}
}

// A genuine independent slot winner needs no local transaction or action row.
func TestEvidenceRelayTransactionThirdPartyFirstDoesNotSend(t *testing.T) {
	fixture := newEvidenceRelayRpcFixture(t, "third-party")
	fixture.installPublication(t, "third-party")
	expected := fixture.expected
	expected.Relayer = common.Address{0x99}
	expected.SignedTransaction = []byte{0xff}
	result, err := fixture.manager.relayValidatorEvidenceTransaction(t.Context(), fixture.chain, fixture.planHash, fixture.action, expected)
	if err != nil || result == nil || result.Winner == nil || result.Winner.Receipt.TxHash != fixture.thirdPartyTransaction.Hash() || result.OwnReceipt != nil || result.LostPublicationRace {
		t.Fatalf("real third-party-first proof: %v", err)
	}
	if fixture.requestCount("eth_sendRawTransaction") != 0 || fixture.requestCount("pending-nonce") != 0 || fixture.requestCount("eth_estimateGas") != 0 || len(fixture.manager.journal.Entries()) != 0 || fixture.requestCount("eth_getLogs") == 0 || fixture.requestCount("eth_getTransactionByHash") == 0 {
		t.Fatal("third-party-first allocated local custody or skipped actual winner discovery")
	}
	if _, err := os.Stat(filepath.Join(fixture.stateDir, "transactions")); !os.IsNotExist(err) {
		t.Fatalf("third-party-first created transaction storage: %v", err)
	}
}

// Included, finalized and failed rows deliberately contain no signer/nonce.
// The original durable broadcast remains authoritative after a process restart.
func TestEvidenceRelayTransactionRecoveryUsesOriginalBroadcast(t *testing.T) {
	for _, stage := range []JournalStage{StageIncluded, StageFinalized, StageFailed} {
		fixture := newEvidenceRelayRpcFixture(t, "success")
		fixture.retain(t, fixture.transaction, fixture.expected.Relayer.Hex(), "4", stage)
		fixture.reopen(t)
		fixture.installPublication(t, "success")
		before := len(fixture.manager.journal.Entries())
		result, err := fixture.manager.relayValidatorEvidenceTransaction(t.Context(), fixture.chain, fixture.planHash, fixture.action, fixture.expected)
		if err != nil || result == nil || result.OwnReceipt == nil || result.OwnReceipt.TxHash != fixture.transaction.Hash() || result.Winner.Receipt.TxHash != fixture.transaction.Hash() {
			t.Fatalf("%s recovery rejected original broadcast identity: %v", stage, err)
		}
		if fixture.requestCount("eth_sendRawTransaction") != 0 || fixture.requestCount("pending-nonce") != 0 || len(fixture.manager.journal.Entries()) != before {
			t.Fatalf("%s confirmed restart wrote or sent again", stage)
		}
	}
}

// Our actual broadcast loses to a different account between absence observation
// and inclusion. The reverted transaction and its paid cost remain explicit.
func TestEvidenceRelayTransactionCanonicalPublicationRace(t *testing.T) {
	fixture := newEvidenceRelayRpcFixture(t, "race")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := fixture.manager.relayValidatorEvidenceTransaction(ctx, fixture.chain, fixture.planHash, fixture.action, fixture.expected)
	if err != nil || result == nil || !result.LostPublicationRace || result.OwnReceipt == nil || result.OwnReceipt.Status != types.ReceiptStatusFailed || result.OwnReceipt.TxHash != fixture.transaction.Hash() || result.Winner == nil || result.Winner.Receipt.TxHash != fixture.thirdPartyTransaction.Hash() {
		t.Fatalf("canonical lost publication race: %v", err)
	}
	if result.OwnReceipt.GasUsed != 80000 || result.OwnReceipt.EffectiveGasPrice.Cmp(big.NewInt(50)) != 0 || fixture.requestCount("eth_sendRawTransaction") != 1 || fixture.requestCount("pending-nonce") != 1 {
		t.Fatal("race hid our actual cost or allocated another nonce")
	}
	last, ok := fixture.manager.journal.LastStage(fixture.action.ID, fixture.action.IntentHash, fixture.planHash)
	if !ok || last.Stage != StageFailed || last.TransactionHash != fixture.transaction.Hash().Hex() || !strings.Contains(last.Error, fixture.thirdPartyTransaction.Hash().Hex()) {
		t.Fatal("race did not retain our failed custody separately from the winner")
	}
	fixture.reopen(t)
	result, err = fixture.manager.relayValidatorEvidenceTransaction(ctx, fixture.chain, fixture.planHash, fixture.action, fixture.expected)
	if err != nil || result == nil || !result.LostPublicationRace || result.OwnReceipt.TxHash != fixture.transaction.Hash() || fixture.requestCount("eth_sendRawTransaction") != 1 || fixture.requestCount("pending-nonce") != 1 {
		t.Fatalf("failed-stage restart changed or resent race custody: %v", err)
	}
}

// A real other-account publication cannot make our noncanonical, malformed or
// unbound failed receipt canonical. All faults alter endpoint bytes only.
func TestEvidenceRelayTransactionRejectsUnprovenPublicationRaces(t *testing.T) {
	for _, fault := range []string{"race-canonical-error", "race-body", "race-cost", "race-logs", "race-no-winner"} {
		fixture := newEvidenceRelayRpcFixture(t, fault)
		fixture.retain(t, fixture.transaction, fixture.expected.Relayer.Hex(), "4", "")
		fixture.installPublication(t, fault)
		result, err := fixture.manager.relayValidatorEvidenceTransaction(t.Context(), fixture.chain, fixture.planHash, fixture.action, fixture.expected)
		if err == nil || result != nil || fixture.requestCount("eth_sendRawTransaction") != 0 || fixture.requestCount("pending-nonce") != 0 {
			t.Fatalf("%s admitted unproven race or changed original transaction: %v", fault, err)
		}
		for _, entry := range fixture.manager.journal.Entries() {
			if entry.Stage == StageFailed && strings.Contains(entry.Error, "canonical publication race:") {
				t.Fatalf("%s was durably mislabeled a canonical race", fault)
			}
		}
	}
}

// The real journal allows later rows without broadcast metadata. It must not
// allow those rows to invent signer/nonce or select a transaction by themselves.
func TestEvidenceRelayTransactionRejectsMissingOriginalBroadcastBeforeRpc(t *testing.T) {
	fixture := newEvidenceRelayRpcFixture(t, "success")
	entry := JournalEntry{DeploymentID: fixture.manager.deploymentID, PlanHash: fixture.planHash, ActionID: fixture.action.ID, IntentHash: fixture.action.IntentHash, Stage: StageIntent}
	if err := fixture.manager.journal.Append(entry); err != nil {
		t.Fatal(err)
	}
	entry.Stage, entry.TransactionHash, entry.BlockNumber, entry.BlockHash = StageIncluded, fixture.transaction.Hash().Hex(), 1201, (common.Hash{0xb1}).Hex()
	if err := fixture.manager.journal.Append(entry); err != nil {
		t.Fatal(err)
	}
	fixture.reopen(t)
	before := fixture.requestCount("")
	result, err := fixture.manager.relayValidatorEvidenceTransaction(t.Context(), fixture.chain, fixture.planHash, fixture.action, fixture.expected)
	if err == nil || result != nil || fixture.requestCount("") != before {
		t.Fatalf("missing original broadcast reached network: %v", err)
	}
}

// Changed signed body, signer, nonce and chain all fail against actual persisted
// bytes before even asking the network whether a third party already won.
func TestEvidenceRelayTransactionRejectsForeignRecoveryBeforeRpc(t *testing.T) {
	for _, fault := range []string{"foreign-signer", "signer-metadata", "nonce-metadata", "noncanonical-nonce", "wrong-chain", "wrong-target", "wrong-calldata", "nonzero-value", "excess-gas", "excess-fee", "corrupt-bytes", "suffixed-bytes", "missing-bytes", "symlink-bytes", "wrong-deployment", "wrong-intent"} {
		fixture := newEvidenceRelayRpcFixture(t, "third-party")
		fixture.installPublication(t, "third-party")
		transaction, signer, nonce := fixture.transaction, fixture.expected.Relayer.Hex(), "4"
		unsigned := &types.DynamicFeeTx{ChainID: big.NewInt(945), Nonce: 4, GasTipCap: big.NewInt(2), GasFeeCap: big.NewInt(100), Gas: 100000, To: &fixture.expected.Journal, Value: new(big.Int), Data: bytes.Clone(fixture.calldata)}
		switch fault {
		case "foreign-signer":
			transaction = fixture.thirdPartyTransaction
		case "signer-metadata":
			signer = common.Address{0x99}.Hex()
		case "nonce-metadata":
			nonce = "5"
		case "noncanonical-nonce":
			nonce = "04"
		case "wrong-chain":
			unsigned.ChainID = big.NewInt(946)
		case "wrong-target":
			target := common.Address{0x99}
			unsigned.To = &target
		case "wrong-calldata":
			unsigned.Data = append(unsigned.Data, 0)
		case "nonzero-value":
			unsigned.Value = big.NewInt(1)
		case "excess-gas":
			unsigned.Gas++
		case "excess-fee":
			unsigned.GasFeeCap = big.NewInt(101)
		}
		if fault == "wrong-chain" || fault == "wrong-target" || fault == "wrong-calldata" || fault == "nonzero-value" || fault == "excess-gas" || fault == "excess-fee" {
			var err error
			transaction, err = types.SignTx(types.NewTx(unsigned), types.LatestSignerForChainID(unsigned.ChainID), fixture.key)
			if err != nil {
				t.Fatal(err)
			}
		}
		fixture.retain(t, transaction, signer, nonce, StageIncluded)
		path := filepath.Join(fixture.stateDir, "transactions", stringsTrim0x(transaction.Hash().Hex())+".rlp")
		switch fault {
		case "corrupt-bytes":
			if err := os.WriteFile(path, []byte{0xc0}, 0o600); err != nil {
				t.Fatal(err)
			}
		case "suffixed-bytes":
			raw, err := transaction.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(raw, 0), 0o600); err != nil {
				t.Fatal(err)
			}
		case "missing-bytes", "symlink-bytes":
			if err := os.Rename(path, path+".retained"); err != nil {
				t.Fatal(err)
			}
			if fault == "symlink-bytes" {
				if err := os.Symlink(path+".retained", path); err != nil {
					t.Fatal(err)
				}
			}
		}
		fixture.reopen(t)
		if fault == "wrong-deployment" {
			fixture.manager.deploymentID = "another-deployment"
		}
		if fault == "wrong-intent" {
			fixture.action.Description += " changed"
			var err error
			fixture.action.IntentHash, err = actionIntentHash(fixture.action)
			if err != nil {
				t.Fatal(err)
			}
		}
		before := fixture.requestCount("")
		result, err := fixture.manager.relayValidatorEvidenceTransaction(t.Context(), fixture.chain, fixture.planHash, fixture.action, fixture.expected)
		if err == nil || result != nil || fixture.requestCount("") != before {
			t.Fatalf("%s recovery reached network or admitted foreign bytes: %v", fault, err)
		}
	}
}

// Current action identity, exact public consent and finite spend bounds are
// admission inputs. A valid independent winner never widens that authority.
func TestEvidenceRelayTransactionRejectsInvalidActionBeforeRpc(t *testing.T) {
	for _, fault := range []string{"slot", "header", "target", "value", "gas", "fee", "intent", "vpk-consent", "hotkey-consent", "chain", "raw-bound", "log-bound"} {
		fixture := newEvidenceRelayRpcFixture(t, "third-party")
		fixture.installPublication(t, "third-party")
		switch fault {
		case "slot":
			fixture.action.Parameters["validator_evidence_slot"] = common.Hash{0x99}.Hex()
		case "header":
			fixture.action.Parameters["validator_evidence_header_hash"] = common.Hash{0x99}.Hex()
		case "target":
			fixture.action.Target = common.Address{0x99}.Hex()
		case "value":
			fixture.action.Spend.TAORao = 1
		case "gas":
			fixture.action.Parameters[evmMaximumGasUnitsParameter] = "0"
		case "fee":
			fixture.action.Parameters[evmMaximumFeePerGasParameter] = "0"
		case "vpk-consent":
			fixture.expected.Evidence.VPKSignature[0] ^= 1
		case "hotkey-consent":
			fixture.expected.Evidence.HotkeySignature[0] ^= 1
		case "chain":
			fixture.manager.chainID = big.NewInt(946)
		case "raw-bound":
			fixture.expected.MaxTransactionBytes = 0
		case "log-bound":
			fixture.expected.MaxReceiptLogs = 0
		}
		var err error
		fixture.action.IntentHash, err = actionIntentHash(fixture.action)
		if err != nil {
			t.Fatal(err)
		}
		if fault == "intent" {
			fixture.action.IntentHash = crypto.Keccak256Hash([]byte("another intent")).Hex()
		}
		before := fixture.requestCount("")
		result, err := fixture.manager.relayValidatorEvidenceTransaction(t.Context(), fixture.chain, fixture.planHash, fixture.action, fixture.expected)
		if err == nil || result != nil || fixture.requestCount("") != before || len(fixture.manager.journal.Entries()) != 0 {
			t.Fatalf("%s action reached custody or network: %v", fault, err)
		}
	}
}

// Capture umask must not provide or remove state authority. The actual journal
// refuses public roots before creating a lock or a transaction history file.
func TestEvidenceRelayTransactionStateDirectoryCustody(t *testing.T) {
	t.Parallel()
	for _, mode := range []os.FileMode{0o755, 0o770} {
		stateDir := t.TempDir()
		if err := os.Chmod(stateDir, mode); err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(stateDir)
		if err != nil || !info.IsDir() || info.Mode().Perm() != mode {
			t.Fatalf("public relay root control has the wrong actual mode: %v", err)
		}
		journal, err := OpenJournal(stateDir)
		if journal != nil {
			if closeErr := journal.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			t.Fatal("public relay root acquired a journal owner")
		}
		if err == nil || !strings.Contains(err.Error(), "accessible by group/other") {
			t.Fatalf("public relay root did not fail its actual custody admission: %v", err)
		}
		entries, err := os.ReadDir(stateDir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("refused public relay root acquired files: %v", err)
		}
	}
	stateDir := t.TempDir()
	if err := os.Chmod(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	journal, err := OpenJournal(stateDir)
	if err != nil || journal == nil {
		t.Fatalf("explicit private relay root did not acquire the real journal: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}
