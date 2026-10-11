//go:build linux || darwin

// The settlement v2 ledger audit (VERSION2-FINAL.md §9.2, §9.3). For every
// configured operator whose pool vault2 registered, the validator reads the
// pending root, the latest finalized root F, fundedThrough for the covered
// epochs and vault2's challenge window at a finalized, hash-pinned block. It
// fetches the ledger objects it does not hold from the operator's object
// endpoint into durable custody, within --ledger_max_bytes per root over all
// attempts, runs ledger.Verify with head bindings read from the archive, and
// decides once per root:
//
//   - approve a root that passes every check;
//   - challenge the first fault the published tree proves against F;
//   - refuse a root whose objects could not be obtained (missing, or beyond
//     the byte budget) by commit + CHALLENGE_BLOCKS/2 (7,200 blocks), or that
//     failed any other check; when the rows are missing or do not reproduce
//     the root and F exists, also demand the §9.3 sample of F's accounts.
//
// Demands and challenges prove accounts against the latest finalized root
// whose accounts custody holds and verified: F, which check 2 verifies, or
// when F's objects cannot be obtained or do not rebuild it, the latest older
// root whose objects custody holds and that they rebuild. Against an older
// root the audit only demands, since only F's rows are compared with the
// pending root's. An audit that could not obtain its objects is retried
// until that block.
// The decision and the demand set, drawn once after the commit was seen, are
// recorded before anything is sent; the auditor sender is idempotent, so a
// restart sends exactly the same transactions. Without an auditor key the
// audit records its decisions and sends nothing. It never changes a pool's
// weight (VA-5), and its failures are reported and retried, never fatal.
package validator

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"time"

	"github.com/holiman/uint256"

	"github.com/urfoundation/sn/ledger"
	"github.com/urfoundation/sn/merkle"
	"github.com/urfoundation/sn/stabi"
)

// settlementV2AuditInterval spaces the audit's passes. A pass that holds its
// decision reads only the pending roots; one that waits for objects verifies
// again from custody.
const settlementV2AuditInterval = time.Minute

// The audit's decisions, as recorded.
const (
	settlementV2DecisionApprove   = "approve"
	settlementV2DecisionChallenge = "challenge"
	settlementV2DecisionDemand    = "demand"
	settlementV2DecisionRefuse    = "refuse"
)

// settlementV2PoolState is what the audit read about one pool at a pinned
// finalized block.
type settlementV2PoolState struct {
	noID       uint64
	block      uint64
	blockHash  [32]byte
	registered bool
	firstEpoch uint64
	pending    stabi.ISTSettlementVault2Pending
	// previous is the latest finalized root F, at previousIndex; nil before
	// the pool's first root.
	previous      *stabi.ISTSettlementVault2Root
	previousIndex uint64
	// funded holds fundedThrough for the covered epochs, read only for a
	// range Verify can audit at all.
	funded map[uint64]uint256.Int
	// challengeBlocks is vault2's CHALLENGE_BLOCKS.
	challengeBlocks uint64
}

// settlementV2AuditReader is what the audit reads from chain.
type settlementV2AuditReader interface {
	// poolState reads the pool at the finalized head; pending and the rest
	// only when it is registered.
	poolState(ctx context.Context, noID uint64) (*settlementV2PoolState, error)
	// finalizedRoot reads the pool's finalized root rootIndex at state's
	// pinned block.
	finalizedRoot(ctx context.Context, state *settlementV2PoolState, rootIndex uint64) (stabi.ISTSettlementVault2Root, error)
	// headOracle decides check 6 at state's pinned block.
	headOracle(state *settlementV2PoolState) ledger.HeadOracle
	// demandHistory reads, from vault2's events between F's finalization and
	// the pending commit, the earlier pending roots over F that were replaced
	// or deleted while a demand was open, and the indexes of those demands.
	demandHistory(ctx context.Context, state *settlementV2PoolState) (doublings uint, again []uint64, err error)
}

// settlementV2AuditSender sends the auditor's transactions;
// *SettlementV2Sender implements it.
type settlementV2AuditSender interface {
	Approve(ctx context.Context, noID uint64, pending stabi.ISTSettlementVault2Pending) error
	Challenge(ctx context.Context, noID uint64, pending stabi.ISTSettlementVault2Pending, rootIndex uint64, account stabi.LedgerTreeAccount, oldProof []stabi.LedgerTreeProofNode, newPath []stabi.LedgerTreePathNode) error
	Demand(ctx context.Context, noID uint64, pending stabi.ISTSettlementVault2Pending, rootIndex uint64, account stabi.LedgerTreeAccount, oldProof []stabi.LedgerTreeProofNode) error
}

// settlementV2Anchor is the finalized root rootIndex whose accounts the audit
// demands or challenges, with their tree.
type settlementV2Anchor struct {
	index    uint64
	accounts []merkle.LedgerAccount
	tree     *merkle.LedgerSumTree
}

// settlementV2AuditPool is one audited pool and its operator's objects.
type settlementV2AuditPool struct {
	noID    uint64
	objects ledger.Fetcher
}

// settlementV2Audit audits the pending roots of its pools.
type settlementV2Audit struct {
	reader  settlementV2AuditReader
	custody *settlementV2Custody
	// sender is nil without --ledger_auditor_key_file.
	sender   settlementV2AuditSender
	pools    []settlementV2AuditPool
	maxBytes uint64
	// random is crypto/rand.Reader; tests fix it.
	random io.Reader
}

func newSettlementV2Audit(reader settlementV2AuditReader, custody *settlementV2Custody, sender settlementV2AuditSender, pools []settlementV2AuditPool, maxBytes uint64) *settlementV2Audit {
	return &settlementV2Audit{reader: reader, custody: custody, sender: sender, pools: pools, maxBytes: maxBytes, random: rand.Reader}
}

// newReleaseSettlementV2Audit audits the configured operators' pools when the
// configuration names settlement v2, and returns nil otherwise. Custody and
// the sender's records live under state_dir; the flags come from `validator
// run` (SettlementV2RuntimeOptionsFrom).
func newReleaseSettlementV2Audit(ctx context.Context, cfg *ReleaseConfig, chain *ChainClient) (*settlementV2Audit, error) {
	addresses := releaseSettlementV2Addresses(cfg)
	if !addresses.configured() {
		return nil, nil
	}
	custody, err := openSettlementV2Custody(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	options := SettlementV2RuntimeOptionsFrom(ctx)
	var sender settlementV2AuditSender
	if options.AuditorKeyFile != "" {
		if sender, err = NewSettlementV2Sender(chain, cfg.RPC, cfg.ChainID, addresses, options.AuditorKeyFile, cfg.StateDir); err != nil {
			return nil, err
		}
	}
	pools := make([]settlementV2AuditPool, len(cfg.Operators))
	for i, operator := range cfg.Operators {
		objects, err := newSettlementV2Objects(operator.APIURL)
		if err != nil {
			return nil, err
		}
		pools[i] = settlementV2AuditPool{noID: operator.NoID, objects: objects}
	}
	reader := &settlementV2AuditChain{chain: chain, vault: addresses.Vault, deployBlock: cfg.DeployBlock}
	return newSettlementV2Audit(reader, custody, sender, pools, options.LedgerMaxBytes), nil
}

// worker is the audit's run loop, or nil without an audit.
func (self *settlementV2Audit) worker() func(context.Context) error {
	if self == nil {
		return nil
	}
	return self.run
}

// run audits every pool, then waits an interval, until ctx ends. A pool's
// failure is reported and retried at the next pass.
func (self *settlementV2Audit) run(ctx context.Context) error {
	for {
		for _, pool := range self.pools {
			if err := self.audit(ctx, pool); err != nil && ctx.Err() == nil {
				releaseDiagnostic(ctx, "operator", "ledger_audit_retrying", 0, false, 0, releaseDiagnosticFacts{cause: releaseDiagnosticReadCause(err), operatorId: pool.noID, operatorKnown: true})
			}
		}
		if err := waitReleaseSnapshotRetry(ctx, settlementV2AuditInterval); err != nil {
			return err
		}
	}
}

// audit takes one step on the pool's pending root: it audits the root until
// the audit decides, then sends what the decision calls for until nothing is
// left to send.
func (self *settlementV2Audit) audit(ctx context.Context, pool settlementV2AuditPool) error {
	state, err := self.reader.poolState(ctx, pool.noID)
	if err != nil {
		return err
	}
	if !state.registered || state.pending.Seq == 0 {
		return nil
	}
	record, err := self.custody.auditRecord(state.noID, state.pending)
	if err != nil || record.Done {
		return err
	}
	var res *ledger.Result
	if record.Decision == "" || record.Decision == settlementV2DecisionChallenge || record.Decision == settlementV2DecisionDemand {
		// A challenge or demand needs the trees again; custody holds them.
		if res, err = self.verify(ctx, pool, state, record); err != nil {
			return err
		}
	}
	if record.Decision == "" {
		if err := self.decide(ctx, state, record, res); err != nil || record.Decision == "" {
			return err
		}
	}
	if self.sender == nil {
		return nil
	}
	outcome, err := self.send(ctx, state, record, res)
	if err != nil {
		return err
	}
	record.Done, record.Outcome = true, outcome
	return self.custody.keepAuditRecord(record)
}

// verify runs ledger.Verify with what is left of the root's byte budget and
// records what it fetched.
func (self *settlementV2Audit) verify(ctx context.Context, pool settlementV2AuditPool, state *settlementV2PoolState, record *settlementV2AuditRecord) (*ledger.Result, error) {
	pending := &state.pending
	in := ledger.VerifyInput{
		NoID: state.noID, FirstEpoch: state.firstEpoch, Seq: pending.Seq,
		Pending:       settlementV2LedgerRoot(pending.Epoch, pending.Root, pending.Total, pending.LeafCount, pending.LedgerHash),
		FundedThrough: state.funded, Fetcher: pool.objects, Custody: self.custody, Heads: self.reader.headOracle(state),
		MaxFetchBytes: self.maxBytes - min(record.FetchedBytes, self.maxBytes),
	}
	if previous := state.previous; previous != nil {
		root := settlementV2LedgerRoot(previous.Epoch, previous.Root, previous.Total, previous.LeafCount, previous.LedgerHash)
		in.Previous = &root
	}
	res, err := ledger.Verify(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("settlement v2 audit of pool %d seq %d: %w", state.noID, pending.Seq, err)
	}
	record.FetchedBytes += res.FetchedBytes
	return res, nil
}

// decide records the audit's finding and, unless the audit waits for its
// objects, its decision. A demand draws its set now, over the anchor's
// accounts: F's, or when F could not be verified, the latest older finalized
// root's that custody holds.
func (self *settlementV2Audit) decide(ctx context.Context, state *settlementV2PoolState, record *settlementV2AuditRecord, res *ledger.Result) error {
	record.observe(state.block, res)
	action := res.Action()
	unobtained := len(res.Missing) > 0 || errors.Is(res.Err, ledger.ErrByteBudget)
	if unobtained && (action == ledger.ActionDemand || action == ledger.ActionRefuse) && state.block < state.pending.CommittedBlock+state.challengeBlocks/2 {
		return self.custody.keepAuditRecord(record)
	}
	var anchor *settlementV2Anchor
	var err error
	switch {
	case res.Previous != nil:
		anchor = &settlementV2Anchor{index: state.previousIndex, accounts: res.PreviousAccounts, tree: res.Previous}
	case state.previous != nil:
		for rootIndex := state.previousIndex; anchor == nil && rootIndex > 0; {
			rootIndex--
			if anchor, err = self.anchor(ctx, state, res, rootIndex); err != nil {
				return err
			}
		}
		if anchor != nil {
			action = ledger.ActionDemand
		}
	}
	record.Decision = action.String()
	switch action {
	case ledger.ActionChallenge:
		record.RootIndex = anchor.index
	case ledger.ActionDemand:
		doublings, again, err := self.reader.demandHistory(ctx, state)
		if err != nil {
			return err
		}
		// An older anchor holds fewer accounts than F, and its rows were not
		// compared with the pending root's.
		again = slices.DeleteFunc(again, func(index uint64) bool { return index >= uint64(len(anchor.accounts)) })
		faults := res.Faults
		if res.Previous == nil {
			faults = nil
		}
		demands, err := ledger.DemandSet(anchor.accounts, faults, doublings, again, self.random)
		if err != nil {
			return fmt.Errorf("settlement v2 demand set of pool %d seq %d: %w", state.noID, state.pending.Seq, err)
		}
		record.RootIndex, record.Doublings, record.Demands = anchor.index, doublings, demands
	}
	if err := self.custody.keepAuditRecord(record); err != nil {
		return err
	}
	releaseDiagnostic(ctx, "operator", "ledger_audit_"+record.Decision, 0, false, 0, releaseDiagnosticFacts{operatorId: state.noID, operatorKnown: true})
	return nil
}

// anchor returns the finalized root rootIndex's accounts and tree: from
// check 2 for F, otherwise from custody alone; nil when custody does not hold
// objects that rebuild that root.
func (self *settlementV2Audit) anchor(ctx context.Context, state *settlementV2PoolState, res *ledger.Result, rootIndex uint64) (*settlementV2Anchor, error) {
	if rootIndex == state.previousIndex && res.Previous != nil {
		return &settlementV2Anchor{index: rootIndex, accounts: res.PreviousAccounts, tree: res.Previous}, nil
	}
	root, err := self.reader.finalizedRoot(ctx, state, rootIndex)
	if err != nil {
		return nil, err
	}
	accounts, tree, err := self.custody.accounts(ctx, settlementV2LedgerRoot(root.Epoch, root.Root, root.Total, root.LeafCount, root.LedgerHash))
	if err != nil || tree == nil {
		return nil, err
	}
	return &settlementV2Anchor{index: rootIndex, accounts: accounts, tree: tree}, nil
}

// send carries out the decision. It returns the outcome once nothing is left
// to send; an error is retried at the next pass. A root that left the
// pending slot, an approval past end(e+1), a demand past the challenge
// window, a finalized root and an answered opening each end the decision.
func (self *settlementV2Audit) send(ctx context.Context, state *settlementV2PoolState, record *settlementV2AuditRecord, res *ledger.Result) (string, error) {
	noID, pending := state.noID, state.pending
	var anchor *settlementV2Anchor
	var err error
	if record.Decision == settlementV2DecisionChallenge || record.Decision == settlementV2DecisionDemand {
		if anchor, err = self.anchor(ctx, state, res, record.RootIndex); err != nil {
			return "", err
		}
		if anchor == nil || record.Decision == settlementV2DecisionChallenge && !res.Reproduced {
			return "", fmt.Errorf("settlement v2 audit of pool %d seq %d no longer rebuilds the trees of its %s against root %d", noID, pending.Seq, record.Decision, record.RootIndex)
		}
	}
	switch record.Decision {
	case settlementV2DecisionApprove:
		err = self.sender.Approve(ctx, noID, pending)
	case settlementV2DecisionChallenge:
		// A fault indexes an account of F, and a tree that reproduces the
		// root has at least F's leaves, so the opening exists.
		index := record.Faults[0].Index
		proof, _ := anchor.tree.Proof(index)
		path, _ := res.Tree.Path(index)
		err = self.sender.Challenge(ctx, noID, pending, anchor.index, settlementV2Account(index, &anchor.accounts[index]), settlementV2ProofNodes(proof), settlementV2PathNodes(path))
	case settlementV2DecisionDemand:
		// DemandSet drew every index from the anchor's accounts.
		for _, index := range record.Demands {
			proof, _ := anchor.tree.Proof(index)
			if err = self.sender.Demand(ctx, noID, pending, anchor.index, settlementV2Account(index, &anchor.accounts[index]), settlementV2ProofNodes(proof)); err != nil {
				break
			}
		}
	default:
		return "refused", nil
	}
	switch {
	case err == nil:
		return "sent", nil
	case errors.Is(err, ErrSettlementV2RootNotPending), errors.Is(err, ErrSettlementV2ApprovalClosed), errors.Is(err, ErrSettlementV2ChallengeWindowClosed),
		errors.Is(err, ErrSettlementV2RootFinalized), errors.Is(err, ErrSettlementV2ChallengeAnswered):
		return err.Error(), nil
	}
	return "", err
}

// settlementV2LedgerRoot is a vault2 root as ledger.Verify takes it.
func settlementV2LedgerRoot(epoch uint64, root [32]byte, total *big.Int, leafCount uint64, ledgerHash [32]byte) ledger.Root {
	return ledger.Root{Epoch: epoch, Root: root, Total: *uint256.MustFromBig(total), LeafCount: leafCount, LedgerHash: ledgerHash}
}

func settlementV2Account(index uint64, account *merkle.LedgerAccount) stabi.LedgerTreeAccount {
	return stabi.LedgerTreeAccount{Index: new(big.Int).SetUint64(index), Coldkey: account.Coldkey, Cumulative: account.Cumulative.ToBig()}
}

func settlementV2ProofNodes(nodes []merkle.LedgerNode) []stabi.LedgerTreeProofNode {
	out := make([]stabi.LedgerTreeProofNode, len(nodes))
	for k := range nodes {
		out[k] = stabi.LedgerTreeProofNode{Hash: nodes[k].Hash, Sum: nodes[k].Sum.ToBig()}
	}
	return out
}

func settlementV2PathNodes(nodes []merkle.LedgerPathNode) []stabi.LedgerTreePathNode {
	out := make([]stabi.LedgerTreePathNode, len(nodes))
	for k := range nodes {
		out[k] = stabi.LedgerTreePathNode{LeftHash: nodes[k].LeftHash, LeftSum: nodes[k].LeftSum.ToBig(), RightHash: nodes[k].RightHash, RightSum: nodes[k].RightSum.ToBig()}
	}
	return out
}
