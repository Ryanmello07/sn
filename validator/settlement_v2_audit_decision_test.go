//go:build linux || darwin

// The settlement v2 audit's §9.3 decision: a provable fault is challenged;
// missing or unreproducible objects lead to a refusal and the demand set;
// approvals, challenges and demands go through the auditor sender, once, and
// survive a restart.
package validator

import (
	"encoding/hex"
	"errors"
	"math/big"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"

	"github.com/urfoundation/sn/ledger"
	"github.com/urfoundation/sn/merkle"
	"github.com/urfoundation/sn/protocol"
	"github.com/urfoundation/sn/stabi"
)

// settlementV2AuditTestLargeF is a finalized root for epoch 3 of n accounts,
// coldkey i+1 at index i; index 0 holds about half of the value, the others
// value in proportion to their index. The funding is that of a root over it
// for epochs 4 and 5.
func settlementV2AuditTestLargeF(t *testing.T, n int) (*ledger.Ledger, map[uint64]uint256.Int) {
	t.Helper()
	rows := make([]protocol.LedgerUsageRow, n)
	var small uint64
	for i := 1; i < n; i++ {
		small += uint64(i + 1)
	}
	for i := range rows {
		usage := uint64(i + 1)
		if i == 0 {
			usage = small
		}
		rows[i] = settlementV2AuditTestRow(100+i, i+1, usage, false)
	}
	funded := map[uint64]uint256.Int{3: *uint256.NewInt(1_000_000_000_000), 4: *uint256.NewInt(1_000_001_600_000), 5: *uint256.NewInt(1_000_002_500_000)}
	return settlementV2AuditTestBuild(t, nil, []ledger.EpochInput{{Epoch: 3, Rows: rows}}, funded), funded
}

// useLargeScenario puts a large F on chain with a pending root over it whose
// objects are withheld, and a recording sender in place of the auditor's.
func (self *settlementV2AuditTestFixture) useLargeScenario(t *testing.T, n int) (*ledger.Ledger, *ledger.Ledger, *settlementV2AuditTestSender) {
	t.Helper()
	f, funded := settlementV2AuditTestLargeF(t, n)
	p := settlementV2AuditTestBuild(t, f, []ledger.EpochInput{{Epoch: 4, Rows: settlementV2AuditTestRows4()}, {Epoch: 5, Rows: settlementV2AuditTestRows5()}}, funded)
	self.useFunding(funded)
	sender := &settlementV2AuditTestSender{}
	self.audit.sender = sender
	return f, p, sender
}

// requireSentDemands requires that the recording sender was asked for exactly
// the demands of indexes, in order, each with its account of the finalized
// root rootIndex, f, and a proof that verifies against it.
func requireSettlementV2AuditDemands(t *testing.T, sender *settlementV2AuditTestSender, f *ledger.Ledger, rootIndex uint64, indexes []uint64) {
	t.Helper()
	if len(sender.demands) != len(indexes) || len(sender.approvals)+len(sender.challenges) != 0 {
		t.Fatalf("%d demands, %d approvals and %d challenges sent; want %d demands", len(sender.demands), len(sender.approvals), len(sender.challenges), len(indexes))
	}
	root, total := f.Tree.Root()
	for k, index := range indexes {
		account, want := sender.demands[k], f.Accounts[index]
		proof := make([]merkle.LedgerNode, len(sender.proofs[k]))
		for j, node := range sender.proofs[k] {
			proof[j] = merkle.LedgerNode{Hash: node.Hash, Sum: *uint256.MustFromBig(node.Sum)}
		}
		cumulative := uint256.MustFromBig(account.Cumulative)
		if sender.rootIndexes[k] != rootIndex || account.Index.Uint64() != index || account.Coldkey != want.Coldkey || !cumulative.Eq(&want.Cumulative) ||
			!merkle.VerifyLedgerProof(root, &total, f.Tree.LeafCount(), index, account.Coldkey, cumulative, proof) {
			t.Fatalf("demand %d is account %s, not account %d of F with its proof", k, account.Index, index)
		}
	}
}

// VA-4: a fault the published tree proves against F is challenged: checkPath
// carries F's account with its proof against F and the pending root's path,
// which shows the fault at the leaf. The challenge deletes the root, and a
// later pass finds nothing pending.
func TestSettlementV2AuditChallengesProvableFault(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f := settlementV2AuditTestF(t)
	p := settlementV2AuditTestP(t, f)
	pub := settlementV2AuditTestPublish(t, p)
	accounts := slices.Clone(p.Accounts)
	accounts[2].Coldkey = settlementV2AuditTestColdkey(99)
	pub.setAccounts(t, accounts)
	commit := pub.seal(t)
	fixture.useScenario(t, f, commit, pub.objects, true, 610)
	fixture.chain.faults[2] = true
	fixture.mustPass(t)
	calls := fixture.sentCalls(t)
	if len(calls) != 1 || calls[0].method != "checkPath" || calls[0].to != settlementV2TestVault || calls[0].value.Sign() != 0 {
		t.Fatalf("sent %d transactions, want one checkPath", len(calls))
	}
	requireSettlementV2AuditOpening(t, calls[0], f, 0, 2)
	account := calls[0].account()
	tree, err := merkle.BuildLedgerSumTree(t.Context(), accounts)
	if err != nil {
		t.Fatal(err)
	}
	root, total := tree.Root()
	fault, level, err := merkle.WalkLedgerPath(root, &total, tree.LeafCount(), 2, account.Coldkey, uint256.MustFromBig(account.Cumulative), calls[0].path())
	if err != nil || fault != merkle.LedgerFaultWrongLeaf || level != 0 {
		t.Fatalf("the path shows %v at level %d: %v", fault, level, err)
	}
	record := fixture.record(t, 2)
	if record.Decision != settlementV2DecisionChallenge || !record.Done || record.Outcome != "sent" || len(record.Faults) != 1 || record.Faults[0] != (settlementV2AuditFault{Index: 2, Kind: merkle.LedgerFaultWrongLeaf.String()}) {
		t.Fatalf("record %+v", record)
	}
	if fixture.chain.current().pending[settlementV2AuditTestNoID].Seq != 0 {
		t.Fatal("the challenge did not delete the root")
	}
	fixture.mustPass(t)
	if len(fixture.chain.accepted()) != 1 {
		t.Fatal("a deleted root was challenged again")
	}
}

// VA-4: objects the operator has not published are waited for until commit
// + CHALLENGE_BLOCKS/2 (7,200 blocks on mainnet); then the audit refuses
// approval and demands every account of a small F, each paying the demand
// fee with the account's proof against F.
func TestSettlementV2AuditWaitsForMissingObjectsThenDemands(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f := settlementV2AuditTestF(t)
	p := settlementV2AuditTestP(t, f)
	objects := ledger.Objects{}
	for ref, data := range p.Objects {
		if ref != p.Commit.LedgerHash {
			objects[ref] = data
		}
	}
	fixture.useScenario(t, f, p.Commit, objects, true, 610)
	for _, head := range []uint64{610, 629} {
		fixture.chain.advanceTo(head)
		fixture.mustPass(t)
		record := fixture.record(t, 2)
		if record.Decision != "" || record.Done || record.Block != head || record.Check != "fetch" || !strings.Contains(record.Error, ledger.ErrObjectUnavailable.Error()) ||
			!slices.Equal(record.Missing, []string{ledger.Ref(p.Commit.LedgerHash).String()}) || len(fixture.chain.accepted()) != 0 {
			t.Fatalf("at block %d: record %+v, %d sent", head, record, len(fixture.chain.accepted()))
		}
	}
	fixture.chain.advanceTo(630)
	fixture.mustPass(t)
	record := fixture.record(t, 2)
	if record.Decision != settlementV2DecisionDemand || !record.Done || record.Outcome != "sent" || record.Doublings != 0 || !slices.Equal(record.Demands, []uint64{0, 1, 2, 3}) {
		t.Fatalf("record %+v", record)
	}
	calls := fixture.sentCalls(t)
	if len(calls) != 4 {
		t.Fatalf("%d demands sent", len(calls))
	}
	for k, call := range calls {
		if call.method != "demandPath" || call.to != settlementV2TestVault || call.value.Cmp(settlementV2TestFee) != 0 {
			t.Fatalf("transaction %d is %s with value %s", k, call.method, call.value)
		}
		requireSettlementV2AuditOpening(t, call, f, 0, uint64(k))
	}
	if fixture.chain.current().pending[settlementV2AuditTestNoID].OpenDemands != 4 {
		t.Fatal("the demands are not open on chain")
	}
}

// VA-4: objects published before commit + CHALLENGE_BLOCKS/2 are audited as
// any other, and an honest root is approved.
func TestSettlementV2AuditApprovesObjectsPublishedInTime(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f := settlementV2AuditTestF(t)
	p := settlementV2AuditTestP(t, f)
	fixture.useScenario(t, f, p.Commit, ledger.Objects{}, true, 610)
	fixture.mustPass(t)
	if record := fixture.record(t, 2); record.Decision != "" || len(record.Missing) != 1 {
		t.Fatalf("record %+v", record)
	}
	fixture.objects.publish(p.Objects)
	fixture.chain.advanceTo(629)
	fixture.mustPass(t)
	fixture.requireApproved(t, fixture.sentCalls(t), 2, p.Commit)
	if record := fixture.record(t, 2); record.Decision != settlementV2DecisionApprove || record.Missing != nil {
		t.Fatalf("record %+v", record)
	}
}

// VA-4: objects that are all present but do not reproduce the root need no
// wait: the audit demands at once.
func TestSettlementV2AuditDemandsUnreproducibleRootAtOnce(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f := settlementV2AuditTestF(t)
	p := settlementV2AuditTestP(t, f)
	commit := p.Commit
	commit.Root[0] ^= 1
	fixture.useScenario(t, f, commit, p.Objects, true, 610)
	fixture.mustPass(t)
	record := fixture.record(t, 2)
	if record.Decision != settlementV2DecisionDemand || record.Check != "tree" || record.Missing != nil || len(fixture.sentCalls(t)) != 4 {
		t.Fatalf("record %+v, %d sent", record, len(fixture.chain.accepted()))
	}
}

// VA-4: the demand sample of a large F is 256 accounts drawn by value with
// randomness read after the commit was seen: exactly ledger.DemandSet over
// F's accounts with the audit's random stream, which always draws the
// account holding half of F's value.
func TestSettlementV2AuditDemandSampleIsWeightedFloor(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f, p, sender := fixture.useLargeScenario(t, 600)
	fixture.useScenario(t, f, p.Commit, ledger.Objects{}, true, 630)
	fixture.mustPass(t)
	want, err := ledger.DemandSet(f.Accounts, nil, 0, nil, settlementV2AuditTestRandom())
	if err != nil {
		t.Fatal(err)
	}
	record := fixture.record(t, 2)
	if len(want) != ledger.DemandSample || !slices.Equal(record.Demands, want) || !slices.Contains(want, 0) || record.Doublings != 0 || !record.Done {
		t.Fatalf("demand set of %d, record %v", len(want), record.Demands)
	}
	requireSettlementV2AuditDemands(t, sender, f, 0, want)
}

// VA-4: an F of 64 accounts is demanded whole: the sample takes every
// account when it holds fewer than 256.
func TestSettlementV2AuditDemandSampleTakesSmallRootWhole(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f, p, sender := fixture.useLargeScenario(t, 64)
	fixture.useScenario(t, f, p.Commit, ledger.Objects{}, true, 630)
	fixture.mustPass(t)
	all := make([]uint64, 64)
	for i := range all {
		all[i] = uint64(i)
	}
	if record := fixture.record(t, 2); !slices.Equal(record.Demands, all) {
		t.Fatalf("demanded %v", record.Demands)
	}
	requireSettlementV2AuditDemands(t, sender, f, 0, all)
}

// VA-4: each earlier pending root over the same F that was deleted or
// replaced while a demand was open doubles the sample, and its unanswered
// indexes are demanded again. A root replaced with every demand answered,
// events before F was finalized, and an answer of the same index proven
// against another root do not count.
func TestSettlementV2AuditDemandSampleDoublesAfterFailedRoot(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f, p, sender := fixture.useLargeScenario(t, 600)
	id, demander := big.NewInt(settlementV2AuditTestNoID), common.Address{0xd1}
	vault := func(name string, block uint64, arguments ...any) {
		fixture.chain.addLog(t, settlementV2VaultABI, settlementV2TestVault, name, block, append([]any{id}, arguments...)...)
	}
	// Before F's finalization at 460, in the same log span: a failed root
	// over an older root.
	vault("PathDemanded", 450, uint64(9), big.NewInt(0), big.NewInt(11), demander)
	vault("DistributionDeleted", 451, uint64(9), big.NewInt(0), big.NewInt(11), uint8(1), big.NewInt(0), demander)
	// Seq 2 is deleted with index 3 open; seq 3 is replaced by seq 4 with
	// every demand answered.
	fixture.committed(t, 605, 2, p.Commit, 0)
	vault("PathDemanded", 610, uint64(2), big.NewInt(0), big.NewInt(3), demander)
	vault("PathDemanded", 611, uint64(2), big.NewInt(0), big.NewInt(4), demander)
	vault("PathAnswered", 612, uint64(2), big.NewInt(0), big.NewInt(4), demander)
	// An opening proven against another root answers another demand.
	vault("PathAnswered", 615, uint64(2), big.NewInt(1), big.NewInt(3), demander)
	vault("DistributionDeleted", 620, uint64(2), big.NewInt(0), big.NewInt(4), uint8(2), big.NewInt(0), demander)
	fixture.committed(t, 1405, 3, p.Commit, 0)
	vault("PathDemanded", 1410, uint64(3), big.NewInt(0), big.NewInt(5), demander)
	vault("PathAnswered", 1411, uint64(3), big.NewInt(0), big.NewInt(5), demander)
	fixture.committed(t, 2405, 4, p.Commit, 3)
	fixture.useScenarioAt(t, f, p.Commit, ledger.Objects{}, true, 4, 2405, 2430)
	fixture.mustPass(t)
	want, err := ledger.DemandSet(f.Accounts, nil, 1, []uint64{3}, settlementV2AuditTestRandom())
	if err != nil {
		t.Fatal(err)
	}
	record := fixture.record(t, 4)
	if record.Doublings != 1 || !slices.Equal(record.Demands, want) || !slices.Contains(want, 3) || len(want) < 2*ledger.DemandSample {
		t.Fatalf("doublings %d, %d demanded", record.Doublings, len(record.Demands))
	}
	requireSettlementV2AuditDemands(t, sender, f, 0, want)
}

// VA-4: rows that show a fault are always demanded, whatever the sample
// draws: here a lowered account and a withheld one, under a commitment the
// published rows do not reproduce.
func TestSettlementV2AuditDemandsFaultRows(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f, p, sender := fixture.useLargeScenario(t, 600)
	pub := settlementV2AuditTestPublish(t, p)
	accounts := slices.Clone(p.Accounts[:599])
	accounts[598].Cumulative.SubUint64(&accounts[598].Cumulative, 1)
	chunks, err := ledger.EncodeAccountChunks(accounts)
	if err != nil {
		t.Fatal(err)
	}
	pub.header.Accounts = pub.put(chunks)
	commit := pub.seal(t)
	fixture.useScenario(t, f, commit, pub.objects, true, 610)
	fixture.mustPass(t)
	faults := []ledger.Fault{{Index: 598, Kind: merkle.LedgerFaultLowerValue}, {Index: 599, Kind: merkle.LedgerFaultWrongLeaf}}
	want, err := ledger.DemandSet(f.Accounts, faults, 0, nil, settlementV2AuditTestRandom())
	if err != nil {
		t.Fatal(err)
	}
	record := fixture.record(t, 2)
	if record.Decision != settlementV2DecisionDemand || !slices.Equal(record.Demands, want) || !slices.Contains(want, 598) || !slices.Contains(want, 599) || len(want) != ledger.DemandSample+2 ||
		len(record.Faults) != 2 || record.Faults[1] != (settlementV2AuditFault{Index: 599, Kind: merkle.LedgerFaultWrongLeaf.String()}) {
		t.Fatalf("record faults %v, %d demanded", record.Faults, len(record.Demands))
	}
	requireSettlementV2AuditDemands(t, sender, f, 0, want)
}

// VA-4: the demand set is drawn once. A pass whose sends fail keeps the
// decision; a restarted validator with fresh randomness sends exactly the
// recorded set.
func TestSettlementV2AuditDemandSetDrawnOnce(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f, p, sender := fixture.useLargeScenario(t, 600)
	fixture.useScenario(t, f, p.Commit, ledger.Objects{}, true, 630)
	sender.err = errors.New("synthetic send failure")
	if err := fixture.pass(t); !errors.Is(err, sender.err) {
		t.Fatalf("pass: %v", err)
	}
	record := fixture.record(t, 2)
	if record.Decision != settlementV2DecisionDemand || record.Done || len(record.Demands) != ledger.DemandSample {
		t.Fatalf("record %+v", record)
	}
	fixture.restart(t)
	fixture.audit.random = rand.NewChaCha8([32]byte{0x01})
	again := &settlementV2AuditTestSender{}
	fixture.audit.sender = again
	fixture.mustPass(t)
	requireSettlementV2AuditDemands(t, again, f, 0, record.Demands)
	if record := fixture.record(t, 2); !record.Done || record.Outcome != "sent" {
		t.Fatalf("record %+v", record)
	}
}

// VA-4: a sender's refusal that ends the decision is recorded as its
// outcome; any other failure leaves the decision to be sent again.
func TestSettlementV2AuditSendOutcomes(t *testing.T) {
	t.Parallel()
	for _, final := range []error{ErrSettlementV2RootNotPending, ErrSettlementV2ApprovalClosed, ErrSettlementV2ChallengeWindowClosed, ErrSettlementV2RootFinalized, ErrSettlementV2ChallengeAnswered} {
		t.Run(final.Error(), func(t *testing.T) {
			t.Parallel()
			fixture := newSettlementV2AuditTestFixture(t)
			f := settlementV2AuditTestF(t)
			p := settlementV2AuditTestP(t, f)
			fixture.useScenario(t, f, p.Commit, p.Objects, true, 610)
			sender := &settlementV2AuditTestSender{err: ErrSettlementV2SendPending}
			fixture.audit.sender = sender
			if err := fixture.pass(t); !errors.Is(err, ErrSettlementV2SendPending) {
				t.Fatalf("pass: %v", err)
			}
			if record := fixture.record(t, 2); record.Decision != settlementV2DecisionApprove || record.Done {
				t.Fatalf("record %+v", record)
			}
			sender.err = final
			fixture.mustPass(t)
			if record := fixture.record(t, 2); !record.Done || record.Outcome != final.Error() || len(sender.approvals) != 2 {
				t.Fatalf("record %+v after %d approvals", record, len(sender.approvals))
			}
		})
	}
}

// VA-4: without --ledger_auditor_key_file the audit decides and records,
// sends nothing and stays undone; a validator restarted with the key sends
// the recorded decision.
func TestSettlementV2AuditWithoutAuditorKey(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f := settlementV2AuditTestF(t)
	p := settlementV2AuditTestP(t, f)
	fixture.useScenario(t, f, p.Commit, p.Objects, true, 610)
	fixture.audit.sender = nil
	fixture.mustPass(t)
	if record := fixture.record(t, 2); record.Decision != settlementV2DecisionApprove || record.Done || len(fixture.chain.accepted()) != 0 {
		t.Fatalf("record %+v", record)
	}
	fixture.restart(t)
	fixture.mustPass(t)
	fixture.requireApproved(t, fixture.sentCalls(t), 2, p.Commit)
}

// VA-3: the first root and the next, each approved through the audit with
// exactly its commitment before its deadline: the first root has no
// previous root to protect, the next is audited against it.
func TestSettlementV2AuditApprovesEachRoot(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f := settlementV2AuditTestF(t)
	p := settlementV2AuditTestP(t, f)
	fixture.objects.publish(f.Objects)
	fixture.objects.publish(p.Objects)
	fixture.setPool(settlementV2AuditTestPending(1, f.Commit, 405))
	fixture.mustPass(t)
	fixture.requireApproved(t, fixture.sentCalls(t), 1, f.Commit)
	if record := fixture.record(t, 1); record.Decision != settlementV2DecisionApprove || !record.Done {
		t.Fatalf("record %+v", record)
	}
	fixture.chain.advanceTo(459)
	fixture.setPool(stabi.ISTSettlementVault2Pending{Total: new(big.Int)}, f.Commit)
	fixture.committed(t, 605, 2, p.Commit, 0)
	fixture.useScenarioAt(t, f, p.Commit, ledger.Objects{}, false, 2, 605, 610)
	fixed := fixture.objects.fetched()
	fixture.mustPass(t)
	calls := fixture.sentCalls(t)
	fixture.requireApproved(t, calls[1:], 2, p.Commit)
	// F's objects were audited with F: only P's are fetched now.
	if fetched := fixture.objects.fetched() - fixed; fetched != len(p.Objects) {
		t.Fatalf("fetched %d objects, want P's %d", fetched, len(p.Objects))
	}
}

// VA-3: an approval is idempotent across restarts: a validator stopped
// after signing reconciles the recorded transaction and never signs another.
func TestSettlementV2AuditApprovalIdempotentAcrossRestart(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f := settlementV2AuditTestF(t)
	p := settlementV2AuditTestP(t, f)
	fixture.useScenario(t, f, p.Commit, p.Objects, true, 610)
	fixture.chain.lock.Lock()
	fixture.chain.hold, fixture.chain.loseAck = true, true
	fixture.chain.lock.Unlock()
	if err := fixture.pass(t); err == nil {
		t.Fatal("an unacknowledged approval passed")
	}
	fixture.chain.lock.Lock()
	fixture.chain.loseAck = false
	fixture.chain.lock.Unlock()
	fixture.restart(t)
	if err := fixture.pass(t); !errors.Is(err, ErrSettlementV2SendPending) {
		t.Fatalf("restart while unmined: %v", err)
	}
	if record := fixture.record(t, 2); record.Decision != settlementV2DecisionApprove || record.Done {
		t.Fatalf("record %+v", record)
	}
	fixture.chain.mineHeld()
	fixture.restart(t)
	fixture.mustPass(t)
	fixture.requireApproved(t, fixture.sentCalls(t), 2, p.Commit)
	if record := fixture.record(t, 2); !record.Done || record.Outcome != "sent" {
		t.Fatalf("record %+v", record)
	}
	fixture.restart(t)
	fixture.mustPass(t)
	if len(fixture.chain.accepted()) != 1 {
		t.Fatal("a restart signed a second approval")
	}
}

// VA-3: an approval that can no longer land before end(e+1) is not sent;
// the decision ends with that outcome.
func TestSettlementV2AuditApprovalClosedAtDeadline(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f := settlementV2AuditTestF(t)
	p := settlementV2AuditTestP(t, f)
	fixture.useScenario(t, f, p.Commit, p.Objects, true, 699)
	fixture.mustPass(t)
	record := fixture.record(t, 2)
	if record.Decision != settlementV2DecisionApprove || !record.Done || record.Outcome != ErrSettlementV2ApprovalClosed.Error() || len(fixture.chain.accepted()) != 0 {
		t.Fatalf("record %+v, %d sent", record, len(fixture.chain.accepted()))
	}
}

// VA-8: a challenge is idempotent: a root another challenger deleted between
// the audit's read and its send counts as success, and nothing is sent.
func TestSettlementV2AuditChallengeOfDeletedRootSucceeds(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f := settlementV2AuditTestF(t)
	p := settlementV2AuditTestP(t, f)
	pub := settlementV2AuditTestPublish(t, p)
	accounts := slices.Clone(p.Accounts)
	accounts[0].Coldkey = settlementV2AuditTestColdkey(99)
	pub.setAccounts(t, accounts)
	commit := pub.seal(t)
	fixture.useScenario(t, f, commit, pub.objects, true, 610)
	fixture.chain.before = func(name string) {
		if name == "checkPath" {
			fixture.chain.before = nil
			fixture.setPool(stabi.ISTSettlementVault2Pending{Total: new(big.Int)}, f.Commit)
		}
	}
	fixture.mustPass(t)
	record := fixture.record(t, 2)
	if record.Decision != settlementV2DecisionChallenge || !record.Done || record.Outcome != "sent" || len(fixture.chain.accepted()) != 0 {
		t.Fatalf("record %+v, %d sent", record, len(fixture.chain.accepted()))
	}
}

// VA-4: a demand set's sends are idempotent: after a send the endpoint
// refused, a restarted validator rebroadcasts that exact transaction, sends
// the rest, and never sends a demand twice.
func TestSettlementV2AuditDemandsResumeAfterFailedSend(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	f := settlementV2AuditTestF(t)
	p := settlementV2AuditTestP(t, f)
	fixture.useScenario(t, f, p.Commit, ledger.Objects{}, true, 630)
	fixture.chain.lock.Lock()
	fixture.chain.failures["eth_sendRawTransaction"] = 3
	fixture.chain.lock.Unlock()
	if err := fixture.pass(t); err == nil {
		t.Fatal("a pass with a refused send succeeded")
	}
	if len(fixture.chain.accepted()) != 2 {
		t.Fatalf("%d demands accepted before the refusal", len(fixture.chain.accepted()))
	}
	fixture.chain.lock.Lock()
	delete(fixture.chain.failures, "eth_sendRawTransaction")
	fixture.chain.lock.Unlock()
	fixture.restart(t)
	for attempt := 0; ; attempt++ {
		err := fixture.pass(t)
		if err == nil {
			break
		}
		if !errors.Is(err, ErrSettlementV2SendPending) || attempt > 1 {
			t.Fatalf("pass %d: %v", attempt, err)
		}
	}
	calls := fixture.sentCalls(t)
	if len(calls) != 4 || fixture.chain.current().pending[settlementV2AuditTestNoID].OpenDemands != 4 {
		t.Fatalf("%d demands accepted", len(calls))
	}
	for k, call := range calls {
		requireSettlementV2AuditOpening(t, call, f, 0, uint64(k))
	}
}

// useOlderScenario puts two finalized roots on chain: R, root 0, for epoch 3
// (the standard F), and F, root 1, for epochs 4 and 5 (the standard P),
// finalized at block 660, with mutate applied to F as vault2 records it. The
// pending root P covers epoch 6 and is committed at block 705 as seq 3; the
// head is block 710. Custody holds R's objects when holdR; F's and P's are
// not published.
func (self *settlementV2AuditTestFixture) useOlderScenario(t *testing.T, holdR bool, mutate func(*ledger.Root)) (r, f, p *ledger.Ledger) {
	t.Helper()
	funded := settlementV2AuditTestFunding()
	funded[6] = *uint256.NewInt(3_000_000)
	self.useFunding(funded)
	r = settlementV2AuditTestF(t)
	f = settlementV2AuditTestP(t, r)
	p = settlementV2AuditTestBuild(t, f, []ledger.EpochInput{{Epoch: 6, Rows: []protocol.LedgerUsageRow{settlementV2AuditTestRow(3, 2, 100, false)}}}, funded)
	if holdR {
		self.holdF(t, r)
	}
	finalized := f.Commit
	if mutate != nil {
		mutate(&finalized)
	}
	self.chain.addLog(t, settlementV2VaultABI, settlementV2TestVault, "DistributionFinalized", 660, big.NewInt(settlementV2AuditTestNoID), uint64(2), big.NewInt(1), finalized.Epoch, finalized.Root, finalized.Total.ToBig(), finalized.LeafCount, [32]byte(finalized.LedgerHash))
	self.committed(t, 705, 3, p.Commit, 0)
	self.chain.advanceTo(709)
	self.setPool(settlementV2AuditTestPending(3, p.Commit, 705), r.Commit, finalized)
	return r, f, p
}

// G2, VA-4: when the latest finalized root's objects are withheld, the audit
// waits until commit + CHALLENGE_BLOCKS/2, then demands against the latest
// older finalized root whose objects custody holds: each demand names that
// root's index and proves its account against it.
func TestSettlementV2AuditDemandsAgainstOlderRootWhenLatestWithheld(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	r, f, _ := fixture.useOlderScenario(t, true, nil)
	fixture.mustPass(t)
	if record := fixture.record(t, 3); record.Decision != "" || record.Check != "fetch" || !slices.Equal(record.Missing, []string{ledger.Ref(f.Commit.LedgerHash).String()}) {
		t.Fatalf("record %+v", record)
	}
	fixture.chain.advanceTo(730)
	fixture.mustPass(t)
	record := fixture.record(t, 3)
	if record.Decision != settlementV2DecisionDemand || record.RootIndex != 0 || !record.Done || record.Outcome != "sent" || !slices.Equal(record.Demands, []uint64{0, 1, 2, 3}) {
		t.Fatalf("record %+v", record)
	}
	calls := fixture.sentCalls(t)
	if len(calls) != len(r.Accounts) {
		t.Fatalf("%d demands sent", len(calls))
	}
	for k, call := range calls {
		requireSettlementV2AuditOpening(t, call, r, 0, uint64(k))
	}
}

// G2, VA-4: a latest finalized root whose objects do not rebuild it is
// passed over at once for the older root custody holds.
func TestSettlementV2AuditDemandsAgainstOlderRootWhenLatestInvalid(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	r, f, _ := fixture.useOlderScenario(t, true, func(root *ledger.Root) { root.Total.AddUint64(&root.Total, 1) })
	for ref, data := range f.Objects {
		if err := fixture.audit.custody.Keep(t.Context(), ref, data); err != nil {
			t.Fatal(err)
		}
	}
	sender := &settlementV2AuditTestSender{}
	fixture.audit.sender = sender
	fixture.mustPass(t)
	record := fixture.record(t, 3)
	if record.Decision != settlementV2DecisionDemand || record.RootIndex != 0 || record.Check != "previous root" || !strings.Contains(record.Error, ledger.ErrPreviousNotTight.Error()) {
		t.Fatalf("record %+v", record)
	}
	requireSettlementV2AuditDemands(t, sender, r, 0, []uint64{0, 1, 2, 3})
}

// G2, VA-4: without a finalized root whose objects custody holds, a root
// whose previous root cannot be verified is refused, and nothing is sent.
func TestSettlementV2AuditRefusesWithoutVerifiedRoot(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	fixture.useOlderScenario(t, false, nil)
	fixture.chain.advanceTo(730)
	fixture.mustPass(t)
	record := fixture.record(t, 3)
	if record.Decision != settlementV2DecisionRefuse || !record.Done || record.Outcome != "refused" || len(fixture.chain.accepted()) != 0 {
		t.Fatalf("record %+v", record)
	}
}

// G2: a demand decided against an older root is sent from custody after a
// restart; if custody no longer holds that root, nothing is sent and the
// pass fails, to be retried.
func TestSettlementV2AuditOlderRootSentAfterRestart(t *testing.T) {
	t.Parallel()
	fixture := newSettlementV2AuditTestFixture(t)
	r, _, _ := fixture.useOlderScenario(t, true, nil)
	fixture.chain.advanceTo(730)
	failing := &settlementV2AuditTestSender{err: errors.New("synthetic send failure")}
	fixture.audit.sender = failing
	if err := fixture.pass(t); !errors.Is(err, failing.err) {
		t.Fatalf("pass: %v", err)
	}
	fixture.restart(t)
	again := &settlementV2AuditTestSender{}
	fixture.audit.sender = again
	fixture.mustPass(t)
	requireSettlementV2AuditDemands(t, again, r, 0, []uint64{0, 1, 2, 3})

	lost := newSettlementV2AuditTestFixture(t)
	r, _, _ = lost.useOlderScenario(t, true, nil)
	lost.chain.advanceTo(730)
	lost.audit.sender = failing
	if err := lost.pass(t); !errors.Is(err, failing.err) {
		t.Fatalf("pass: %v", err)
	}
	if err := os.Remove(lost.audit.custody.path("objects", hex.EncodeToString(r.Commit.LedgerHash[:]))); err != nil {
		t.Fatal(err)
	}
	if err := lost.pass(t); err == nil || !strings.Contains(err.Error(), "no longer rebuilds the trees of its demand against root 0") {
		t.Fatalf("pass without the root in custody: %v", err)
	}
}
