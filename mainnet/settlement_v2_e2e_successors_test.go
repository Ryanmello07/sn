package main

import (
	"fmt"
	"math/big"
	"slices"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"

	"github.com/urfoundation/sn/ledger"
	"github.com/urfoundation/sn/merkle"
	"github.com/urfoundation/sn/protocol"
)

// demand sends demandPath for account index of the finalized tree, paying
// the fee, and checks PathDemanded.
func (f *e2eFixture) demand(actor *e2eActor, finalized *e2eTreeView, seq, index uint64) {
	f.t.Helper()
	account, proof := e2eProof(f.t, finalized.tree, finalized.accounts, index)
	receipt := f.must(actor, f.vault2, e2eRao(e2eDemandFeeWei), vault2Binding.PackDemandPath(e2eRao(e2eNoID), account, proof))
	f.requireLogs(receipt, f.vault2Log("PathDemanded", e2eRao(e2eNoID), seq, e2eRao(index), actor.address))
}

// open sends checkPath for account index of the finalized tree with path, a
// top-down opening of the pending root: an answer or a challenge.
func (f *e2eFixture) open(actor *e2eActor, finalized *e2eTreeView, index uint64, path []merkle.LedgerPathNode) *types.Receipt {
	f.t.Helper()
	account, proof := e2eProof(f.t, finalized.tree, finalized.accounts, index)
	return f.must(actor, f.vault2, nil, vault2Binding.PackCheckPath(e2eRao(e2eNoID), account, proof, e2ePath(path)))
}

// e2eRawTree is a ledger sum tree whose nodes can be misstated.
type e2eRawTree struct{ levels [][]merkle.LedgerNode }

func newE2ERawTree(t *testing.T, accounts []merkle.LedgerAccount) *e2eRawTree {
	t.Helper()
	depth, err := merkle.LedgerTreeDepth(uint64(len(accounts)))
	if err != nil {
		t.Fatal(err)
	}
	leaves := make([]merkle.LedgerNode, 1<<depth)
	for i := range accounts {
		leaves[i] = merkle.LedgerNode{Hash: merkle.LedgerLeafHash(accounts[i].Coldkey, &accounts[i].Cumulative), Sum: accounts[i].Cumulative}
	}
	tree := &e2eRawTree{levels: [][]merkle.LedgerNode{leaves}}
	for level := 1; level <= depth; level++ {
		tree.levels = append(tree.levels, make([]merkle.LedgerNode, len(tree.levels[level-1])/2))
		for j := range tree.levels[level] {
			tree.recompute(level, j)
		}
	}
	return tree
}

func (self *e2eRawTree) recompute(level, j int) {
	left, right := self.levels[level-1][2*j], self.levels[level-1][2*j+1]
	node := &self.levels[level][j]
	node.Hash = merkle.LedgerNodeHash(left.Hash, &left.Sum, right.Hash, &right.Sum)
	node.Sum.Add(&left.Sum, &right.Sum)
}

// misstate makes node j of level state sum over its unchanged children, and
// recomputes the nodes above it.
func (self *e2eRawTree) misstate(level, j int, sum uint256.Int) {
	self.levels[level][j].Sum = sum
	for level++; level < len(self.levels); level++ {
		j /= 2
		self.recompute(level, j)
	}
}

func (self *e2eRawTree) root() ([32]byte, uint256.Int) {
	top := self.levels[len(self.levels)-1][0]
	return top.Hash, top.Sum
}

// path returns the top-down preimages above index, as LedgerSumTree.Path.
func (self *e2eRawTree) path(index uint64) []merkle.LedgerPathNode {
	depth := len(self.levels) - 1
	path := make([]merkle.LedgerPathNode, depth)
	for i := range depth {
		level := depth - i
		j := index >> level
		left, right := self.levels[level-1][2*j], self.levels[level-1][2*j+1]
		path[i] = merkle.LedgerPathNode{LeftHash: left.Hash, LeftSum: left.Sum, RightHash: right.Hash, RightSum: right.Sum}
	}
	return path
}

// e2eSuccessor is the bad-successor setting: pool 1's finalized root F (root
// 0) covers its first epoch with four accounts, and the next epoch is closed,
// so its root signer can commit a successor over F; honest is the successor
// the operator would build, with three new accounts.
type e2eSuccessor struct {
	f         *e2eFixture
	finalized *e2eTreeView
	honest    *ledger.Ledger
	epoch     uint64
}

func newE2ESuccessor(t *testing.T) *e2eSuccessor {
	t.Helper()
	f := newE2EFixture(t, e2eDefaultFirst)
	root0 := f.epoch(f.firstEpoch, 1_000_000_003, []protocol.LedgerUsageRow{
		e2eRow(1, e2eKey("alice"), 300), e2eRow(2, e2eKey("alice"), 100), e2eRow(3, e2eKey("bob"), 200), e2eRow(4, [32]byte{}, 500),
		e2eRow(5, e2eKey("carol"), 400), e2eRow(6, e2eKey("dave"), 1),
	})
	epoch := f.firstEpoch + 1
	f.rows[epoch] = []protocol.LedgerUsageRow{
		e2eRow(1, e2eKey("alice"), 250), e2eRow(3, e2eKey("bob"), 250), e2eRow(5, e2eKey("carol"), 100), e2eRow(8, e2eKey("erin"), 300),
		e2eRow(10, [32]byte{}, 50), e2eRow(11, e2eKey("frank"), 500), e2eRow(13, e2eKey("gina"), 7),
	}
	f.emitV2(1_500_000_000)
	f.closeV2(epoch)
	honest, err := f.build(epoch)
	if err != nil {
		t.Fatal(err)
	}
	if len(root0.Accounts) != 4 || len(honest.Accounts) != 7 {
		t.Fatalf("the setting has %d and %d accounts", len(root0.Accounts), len(honest.Accounts))
	}
	return &e2eSuccessor{f: f, finalized: &e2eTreeView{accounts: root0.Accounts, tree: root0.Tree}, honest: honest, epoch: epoch}
}

// check runs UR's watcher, snclaim v2 check --all --challenge --demand.
func (s *e2eSuccessor) check(t *testing.T) (string, string, int) {
	return s.f.snclaim(s.f.snclaimArgs("v2", "check", "--all", "--no_id=1", "--challenge", "--demand", "--key_file="+s.f.watcher.keyFile(t))...)
}

// faults walks every account of F through the successor tree whose opening
// is path, as vault2's checkPath does, and returns each account's fault.
func (s *e2eSuccessor) faults(t *testing.T, root [32]byte, total uint256.Int, leafCount uint64, path func(uint64) []merkle.LedgerPathNode) []merkle.LedgerFault {
	t.Helper()
	faults := make([]merkle.LedgerFault, len(s.finalized.accounts))
	for i := range faults {
		account := s.finalized.accounts[i]
		fault, _, err := merkle.WalkLedgerPath(root, &total, leafCount, uint64(i), account.Coldkey, &account.Cumulative, path(uint64(i)))
		if err != nil {
			t.Fatal(err)
		}
		faults[i] = fault
	}
	return faults
}

// The bad successors whose account rows are a consistent tree, each with
// the accounts of F it faults; mutate keeps the total.
var e2eBadSuccessors = []struct {
	name   string
	mutate func(f, p []merkle.LedgerAccount) []merkle.LedgerAccount
}{
	{"omitted account", func(f, p []merkle.LedgerAccount) []merkle.LedgerAccount {
		attacker := merkle.LedgerAccount{Coldkey: e2eKey("mallory"), Cumulative: p[1].Cumulative}
		return append(slices.Delete(slices.Clone(p), 1, 2), attacker)
	}},
	{"lowered account", func(f, p []merkle.LedgerAccount) []merkle.LedgerAccount {
		p = slices.Clone(p)
		lowered := new(uint256.Int).SubUint64(&f[0].Cumulative, 1)
		p[6].Cumulative.Add(&p[6].Cumulative, new(uint256.Int).Sub(&p[0].Cumulative, lowered))
		p[0].Cumulative = *lowered
		return p
	}},
	{"garbage leaf", func(f, p []merkle.LedgerAccount) []merkle.LedgerAccount {
		p = slices.Clone(p)
		p[2].Coldkey = [32]byte{0xde, 0xad, 0xbe, 0xef}
		return p
	}},
	{"moved coldkey", func(f, p []merkle.LedgerAccount) []merkle.LedgerAccount {
		p = slices.Clone(p)
		p[1].Coldkey, p[3].Coldkey = p[3].Coldkey, p[1].Coldkey
		return p
	}},
}

// E-4, published: a bypassed admission commits each bad successor over F
// with its account rows published. snclaim v2 check finds the faults and
// challenges the first; vault2's checkPath, with F's account, its proof
// against F and the published tree's path, deletes the root and names the
// fault at the leaf.
func TestSettlementV2E2EBadSuccessorsPublished(t *testing.T) {
	for _, bad := range e2eBadSuccessors {
		t.Run(bad.name, func(t *testing.T) {
			s := newE2ESuccessor(t)
			f := s.f
			accounts := bad.mutate(s.finalized.accounts, s.honest.Accounts)
			commitment, objects, tree := f.forge(s.honest, accounts)
			f.publisher.publish(objects, accounts, tree)
			seq := f.commit(f.operator, s.epoch, commitment)
			faults := s.faults(t, commitment.Root, commitment.Total, commitment.LeafCount, func(i uint64) []merkle.LedgerPathNode {
				path, err := tree.Path(i)
				if err != nil {
					t.Fatal(err)
				}
				return path
			})
			first := slices.IndexFunc(faults, func(fault merkle.LedgerFault) bool { return fault != merkle.LedgerFaultNone })
			if first < 0 {
				t.Fatal("the bad successor faults no account of F")
			}
			output, stderr, code := s.check(t)
			want := fmt.Sprintf("DistributionDeleted: pool 1, seq %d\n  account:     %d\n  fault:       %v at level 0\n  challenger:  %s\n", seq, first, faults[first], f.watcher.address.Hex())
			if code != 0 || !strings.Contains(output, "  audit:             challenge (accounts check: ledger: a previous ") || !strings.HasSuffix(output, want) {
				t.Fatalf("watcher (exit %d):\n%s%s", code, output, stderr)
			}
			sent := f.chain.sentBy(f.watcher.address)
			if len(sent) != 1 {
				t.Fatalf("the watcher sent %d transactions", len(sent))
			}
			f.requireLogs(f.chain.receipt(sent[0].Hash()), f.vault2Log("DistributionDeleted", e2eRao(e2eNoID), seq, e2eRao(uint64(first)), uint8(faults[first]), e2eRao(0), f.watcher.address))
			if f.pending().Seq != 0 {
				t.Fatal("the challenged root is still pending")
			}
		})
	}
}

// inconsistent is the bad successor that hides an over-allocation in an
// inconsistent subtree: its published account rows give Gina hidden more
// than the honest root, and the node above F's four accounts states their
// sum less hidden, so the committed total is still the funding.
func (s *e2eSuccessor) inconsistent(t *testing.T, hidden uint64) (ledger.Root, ledger.Objects, []merkle.LedgerAccount, *e2eRawTree) {
	t.Helper()
	rows := slices.Clone(s.honest.Accounts)
	rows[6].Cumulative.AddUint64(&rows[6].Cumulative, hidden)
	committed := newE2ERawTree(t, rows)
	stated := committed.levels[2][0].Sum
	stated.SubUint64(&stated, hidden)
	committed.misstate(2, 0, stated)
	published, objects, _ := s.f.forge(s.honest, rows)
	root, total := committed.root()
	if total != s.honest.Commit.Total {
		t.Fatalf("the inconsistent root's total %s is not the funding", total.Dec())
	}
	return ledger.Root{Epoch: s.epoch, Root: root, Total: total, LeafCount: 7, LedgerHash: published.LedgerHash}, objects, rows, committed
}

// E-4, the over-allocation hidden in an inconsistent subtree: its account
// rows, published, never reproduce the committed root, so the watcher
// reports F's accounts unverifiable, never "no fault", and demands them all;
// any opening under the bad node, here the operator's own answer, shows the
// inconsistent node and deletes the root.
func TestSettlementV2E2EInconsistentSubtreePublished(t *testing.T) {
	s := newE2ESuccessor(t)
	f := s.f
	commitment, objects, rows, committed := s.inconsistent(t, 1_000)
	f.publisher.publish(objects, rows, nil)
	seq := f.commit(f.operator, s.epoch, commitment)
	output, stderr, code := s.check(t)
	if code != 0 || !strings.Contains(output, "  audit:             demand (tree check: ledger: account rows do not sum to the committed total: ") ||
		!strings.Contains(output, "  previous accounts: 4, unverifiable: the published objects are missing or do not reproduce the pending root\n") ||
		!strings.Contains(output, "demand set: 4 accounts (0 doublings, 0 forced), 0 already demanded\n") || strings.Contains(output, "no fault") {
		t.Fatalf("watcher (exit %d):\n%s%s", code, output, stderr)
	}
	s.requireDemanded(t, seq, []uint64{0, 1, 2, 3})
	receipt := f.open(f.operator, s.finalized, 2, committed.path(2))
	f.requireLogs(receipt, f.vault2Log("DistributionDeleted", e2eRao(e2eNoID), seq, e2eRao(2), uint8(merkle.LedgerFaultInconsistentNode), e2eRao(2), f.operator.address))
	if f.pending().Seq != 0 {
		t.Fatal("the inconsistent root is still pending")
	}
}

// requireDemanded checks that the watcher's transactions are exactly one
// demand of each index, each paying the fee, with PathDemanded's fields.
func (s *e2eSuccessor) requireDemanded(t *testing.T, seq uint64, indexes []uint64) {
	t.Helper()
	f := s.f
	sent := f.chain.sentBy(f.watcher.address)
	if len(sent) != len(indexes) {
		t.Fatalf("the watcher sent %d transactions for %d demands", len(sent), len(indexes))
	}
	for k, tx := range sent {
		account, proof := e2eProof(t, s.finalized.tree, s.finalized.accounts, indexes[k])
		if tx.Value().Cmp(e2eRao(e2eDemandFeeWei)) != 0 || !slices.Equal(tx.Data(), vault2Binding.PackDemandPath(e2eRao(e2eNoID), account, proof)) {
			t.Fatalf("demand %d is not account %d's, with the fee", k, indexes[k])
		}
		f.requireLogs(f.chain.receipt(tx.Hash()), f.vault2Log("PathDemanded", e2eRao(e2eNoID), seq, e2eRao(indexes[k]), f.watcher.address))
	}
}

// E-4, withheld: when the successor's tree is not published, the watcher
// demands every account of F (fewer than the 256 sample), each paying the
// fee with its proof against F. The attacker approves the root and answers
// every demand it can, the clean openings; the demand on a faulted account
// cannot be answered, so finalize is refused once the window has passed,
// and opening it deletes the root with its fault. The inconsistent subtree
// is blocked the same way, its every opening crossing the bad node.
func TestSettlementV2E2EBadSuccessorsWithheld(t *testing.T) {
	type successor struct {
		name   string
		commit func(*e2eSuccessor) (ledger.Root, func(uint64) []merkle.LedgerPathNode)
	}
	cases := []successor{}
	for _, bad := range e2eBadSuccessors {
		cases = append(cases, successor{bad.name, func(s *e2eSuccessor) (ledger.Root, func(uint64) []merkle.LedgerPathNode) {
			accounts := bad.mutate(s.finalized.accounts, s.honest.Accounts)
			commitment, _, tree := s.f.forge(s.honest, accounts)
			return commitment, func(i uint64) []merkle.LedgerPathNode {
				path, err := tree.Path(i)
				if err != nil {
					s.f.t.Fatal(err)
				}
				return path
			}
		}})
	}
	cases = append(cases, successor{"inconsistent subtree", func(s *e2eSuccessor) (ledger.Root, func(uint64) []merkle.LedgerPathNode) {
		commitment, _, _, committed := s.inconsistent(s.f.t, 1_000)
		return commitment, committed.path
	}})
	for _, bad := range cases {
		t.Run(bad.name, func(t *testing.T) {
			s := newE2ESuccessor(t)
			f := s.f
			commitment, path := bad.commit(s)
			seq := f.commit(f.operator, s.epoch, commitment)
			output, stderr, code := s.check(t)
			if code != 0 || !strings.Contains(output, "  object missing:    "+ledger.Ref(commitment.LedgerHash).String()+"\n") ||
				!strings.Contains(output, "  previous accounts: 4, unverifiable") || !strings.Contains(output, "demand set: 4 accounts (0 doublings, 0 forced), 0 already demanded\n") {
				t.Fatalf("watcher (exit %d):\n%s%s", code, output, stderr)
			}
			s.requireDemanded(t, seq, []uint64{0, 1, 2, 3})
			// The bypassed auditor approves; the attacker answers what it can.
			f.approveBlind(f.auditor)
			faults := s.faults(t, commitment.Root, commitment.Total, commitment.LeafCount, path)
			faulted := -1
			for i, fault := range faults {
				if fault != merkle.LedgerFaultNone {
					faulted = i
					continue
				}
				receipt := f.open(f.operator, s.finalized, uint64(i), path(uint64(i)))
				f.requireLogs(receipt, f.vault2Log("PathAnswered", e2eRao(e2eNoID), seq, e2eRao(uint64(i)), f.operator.address))
			}
			open := uint64(0)
			for _, fault := range faults {
				if fault != merkle.LedgerFaultNone {
					open++
				}
			}
			if faulted < 0 || f.pending().OpenDemands != open {
				t.Fatalf("%d demands open, want %d", f.pending().OpenDemands, open)
			}
			// The demands block the root past its window.
			f.chain.rollTo(f.pending().CommittedBlock + e2eWindow)
			f.refused(f.anyone, f.settlement, nil, settlementBinding.PackFinalizeDistribution(e2eRao(e2eNoID)), "DemandOpen")
			// Opening a faulted account deletes the root.
			_, level, err := merkle.WalkLedgerPath(commitment.Root, &commitment.Total, commitment.LeafCount, uint64(faulted), s.finalized.accounts[faulted].Coldkey, &s.finalized.accounts[faulted].Cumulative, path(uint64(faulted)))
			if err != nil {
				t.Fatal(err)
			}
			receipt := f.open(f.anyone, s.finalized, uint64(faulted), path(uint64(faulted)))
			f.requireLogs(receipt, f.vault2Log("DistributionDeleted", e2eRao(e2eNoID), seq, e2eRao(uint64(faulted)), uint8(faults[faulted]), big.NewInt(int64(level)), f.anyone.address))
			if f.pending().Seq != 0 || f.rootCount() != 1 {
				t.Fatal("the faulted root survived")
			}
		})
	}
}

// watcherDemands returns the indexes the watcher's transactions after the
// first skip demanded on seq, checking that each paid the fee.
func (f *e2eFixture) watcherDemands(t *testing.T, skip int, seq uint64) []uint64 {
	t.Helper()
	var indexes []uint64
	for _, tx := range f.chain.sentBy(f.watcher.address)[skip:] {
		receipt := f.chain.receipt(tx.Hash())
		if tx.Value().Cmp(e2eRao(e2eDemandFeeWei)) != 0 || receipt == nil || len(receipt.Logs) != 1 {
			t.Fatalf("watcher transaction %s is not a paid demand", tx.Hash())
		}
		event, err := vault2Binding.UnpackPathDemandedEvent(receipt.Logs[0])
		if err != nil || event.NoId.Uint64() != e2eNoID || event.Seq != seq || event.Demander != f.watcher.address {
			t.Fatalf("PathDemanded %+v (%v), want seq %d by the watcher", event, err, seq)
		}
		f.requireLogs(receipt, f.vault2Log("PathDemanded", e2eRao(e2eNoID), seq, event.Index, f.watcher.address))
		indexes = append(indexes, event.Index.Uint64())
	}
	return indexes
}

// E-4, the re-roll drill (§4.2 G2, §9.3): F has 600 accounts and the victim
// holds nearly all of its value. A bypassed admission commits a successor
// that lowers the victim, with its tree withheld. The watcher demands a
// value-weighted sample of 256, which draws the victim; the attacker answers
// every other demand, then deletes its own root to draw a new sample and
// recommits at once, which vault2's commit spacing refuses. Its next root,
// in the next epoch, meets the watcher's doubled sample of 512 plus the
// unanswered index again, and stays blocked past its window.
func TestSettlementV2E2EReRollDrill(t *testing.T) {
	f := newE2EFixture(t, e2eDefaultFirst)
	rows := make([]protocol.LedgerUsageRow, 600)
	for i := range rows {
		usage := uint64(1_000)
		if i == 0 {
			usage = 99_000_000
		}
		rows[i] = e2eRow(uint64(i+1), e2eKey(fmt.Sprintf("provider-%d", i)), usage)
	}
	root0 := f.epoch(f.firstEpoch, 600_000_000_000, rows)
	finalized := &e2eTreeView{accounts: root0.Accounts, tree: root0.Tree}
	victim, mallory := e2eAccountOf(t, root0.Accounts, e2eKey("provider-0")), e2eKey("mallory")
	// Each successor moves the victim one rao below F to the attacker.
	attack := func(epoch uint64) (ledger.Root, *merkle.LedgerSumTree) {
		f.rows[epoch] = []protocol.LedgerUsageRow{e2eRow(1, e2eKey("provider-0"), 1_000), e2eRow(9_999, mallory, 1_000)}
		f.emitV2(10_000_000_000)
		f.closeV2(epoch)
		honest, err := f.build(epoch)
		if err != nil {
			t.Fatal(err)
		}
		accounts := slices.Clone(honest.Accounts)
		m := e2eAccountOf(t, accounts, mallory)
		lowered := new(uint256.Int).SubUint64(&finalized.accounts[victim].Cumulative, 1)
		accounts[m].Cumulative.Add(&accounts[m].Cumulative, new(uint256.Int).Sub(&accounts[victim].Cumulative, lowered))
		accounts[victim].Cumulative = *lowered
		commitment, _, tree := f.forge(honest, accounts)
		return commitment, tree
	}
	answer := func(seq uint64, tree *merkle.LedgerSumTree, demanded []uint64) {
		for _, i := range demanded {
			if i == victim {
				continue
			}
			path, err := tree.Path(i)
			if err != nil {
				t.Fatal(err)
			}
			f.requireLogs(f.open(f.operator, finalized, i, path), f.vault2Log("PathAnswered", e2eRao(e2eNoID), seq, e2eRao(i), f.operator.address))
		}
		if f.pending().OpenDemands != 1 {
			t.Fatalf("%d demands open, want the victim's", f.pending().OpenDemands)
		}
	}
	check := func() string {
		output, stderr, code := f.snclaim(f.snclaimArgs("v2", "check", "--all", "--no_id=1", "--challenge", "--demand", "--key_file="+f.watcher.keyFile(t))...)
		if code != 0 {
			t.Fatalf("watcher (exit %d):\n%s%s", code, output, stderr)
		}
		return output
	}

	e1 := f.firstEpoch + 1
	commitment, tree := attack(e1)
	seq := f.commit(f.operator, e1, commitment)
	if output := check(); !strings.Contains(output, "demand set: 256 accounts (0 doublings, 0 forced), 0 already demanded\n") {
		t.Fatalf("first sample:\n%s", output)
	}
	demanded := f.watcherDemands(t, 0, seq)
	if len(demanded) != 256 || !slices.Contains(demanded, victim) {
		t.Fatalf("the sample demanded %d accounts, the victim %t", len(demanded), slices.Contains(demanded, victim))
	}
	answer(seq, tree, demanded)
	// The re-roll: the attacker proves its own fault, deleting the root, and
	// recommits in the same window.
	path, err := tree.Path(victim)
	if err != nil {
		t.Fatal(err)
	}
	f.requireLogs(f.open(f.operator, finalized, victim, path), f.vault2Log("DistributionDeleted", e2eRao(e2eNoID), seq, e2eRao(victim), uint8(merkle.LedgerFaultLowerValue), e2eRao(0), f.operator.address))
	f.refused(f.operator, f.settlement, nil, f.commitData(e1, commitment), "CommitTooSoon")

	// The next epoch's root meets the doubled sample and the victim again.
	commitment, tree = attack(e1 + 1)
	seq = f.commit(f.operator, e1+1, commitment)
	if output := check(); !strings.Contains(output, "demand set: 513 accounts (1 doublings, 1 forced), 0 already demanded\n") {
		t.Fatalf("doubled sample:\n%s", output)
	}
	demanded = f.watcherDemands(t, 256, seq)
	if len(demanded) != 513 || !slices.Contains(demanded, victim) {
		t.Fatalf("the doubled sample demanded %d accounts, the victim %t", len(demanded), slices.Contains(demanded, victim))
	}
	answer(seq, tree, demanded)
	f.approveBlind(f.auditor)
	f.chain.rollTo(f.pending().CommittedBlock + e2eWindow)
	f.refused(f.anyone, f.settlement, nil, settlementBinding.PackFinalizeDistribution(e2eRao(e2eNoID)), "DemandOpen")
	if f.pending().Seq != seq || f.rootCount() != 1 {
		t.Fatal("the attacker's root left the pending slot")
	}
}
