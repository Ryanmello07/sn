package onchain

// v2_check.go — `snclaim v2 check --all` (VERSION2-FINAL.md §9.3, §10.2,
// §4.2 G2): an independent watcher of a pool's pending root. It reads the
// pending root, the latest finalized root F and the covered epochs'
// fundedThrough from vault2 at the finalized block, fetches the published
// objects, and runs ledger.Verify (without head bindings, for which a
// watcher has no archive state). Every account of F is then reported as
// having no fault, a provable fault, or as unverifiable when objects are
// missing or do not reproduce the root: never "no fault" then. With
// --challenge it sends checkPath for a provable fault, which deletes the
// root; with --demand it sends the §9.3 demand set (ledger.DemandSet), each
// demand paying demandFeeWei with the account and its proof against F.

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"slices"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"

	docopt "github.com/docopt/docopt-go"

	"github.com/urfoundation/sn/ledger"
	"github.com/urfoundation/sn/merkle"
	"github.com/urfoundation/sn/stabi"
)

// v2LogChunkBlocks is the widest eth_getLogs span asked for: the public
// endpoint's enforced maximum (validator/chain.go getLogsChunkBlocks).
const v2LogChunkBlocks = 1_000

// v2State is a pool's vault2 state at the finalized block.
type v2State struct {
	firstEpoch uint64
	pending    stabi.ISTSettlementVault2Pending
	// previous is the latest finalized root F, at index previousIndex; nil
	// before the pool's first root.
	previous      *stabi.ISTSettlementVault2Root
	previousIndex *big.Int
	funded        map[uint64]uint256.Int
}

// readV2State reads the pool, its pending root, F and fundedThrough for
// the covered epochs p+1 … e. A range Verify refuses by its length alone
// is not read.
func readV2State(ctx context.Context, r *v2Reader, noID uint64) (*v2State, error) {
	n := new(big.Int).SetUint64(noID)
	pool, err := v2View(ctx, r, vault2.PackPools(n), vault2.UnpackPools)
	if err != nil {
		return nil, fmt.Errorf("pools(%d): %w", noID, err)
	}
	if !pool.Registered {
		return nil, fmt.Errorf("pool %d is not registered in vault2 %s", noID, r.vault.Hex())
	}
	pending, err := v2View(ctx, r, vault2.PackPending(n), vault2.UnpackPending)
	if err != nil {
		return nil, fmt.Errorf("pending(%d): %w", noID, err)
	}
	state := &v2State{firstEpoch: pool.FirstEpoch, pending: pending, funded: map[uint64]uint256.Int{}}
	if pending.Seq == 0 {
		return state, nil
	}
	count, err := v2View(ctx, r, vault2.PackRootCount(n), vault2.UnpackRootCount)
	if err != nil {
		return nil, fmt.Errorf("rootCount(%d): %w", noID, err)
	}
	p := pool.FirstEpoch - 1
	if count.Sign() > 0 {
		index := new(big.Int).Sub(count, common.Big1)
		root, err := v2View(ctx, r, vault2.PackRoots(n, index), vault2.UnpackRoots)
		if err != nil {
			return nil, fmt.Errorf("roots(%d,%s): %w", noID, index, err)
		}
		state.previous, state.previousIndex, p = &root, index, root.Epoch
	}
	if pending.Epoch > p && pending.Epoch-p <= ledger.MaxCoveredEpochs {
		for k := p + 1; k <= pending.Epoch; k++ {
			funded, err := v2View(ctx, r, vault2.PackFundedThrough(n, new(big.Int).SetUint64(k)), vault2.UnpackFundedThrough)
			if err != nil {
				return nil, fmt.Errorf("fundedThrough(%d,%d): %w", noID, k, err)
			}
			state.funded[k] = *uint256.MustFromBig(funded)
		}
	}
	return state, nil
}

// v2Objects fetches ledger objects from GET /sn/ledger/object.
type v2Objects struct{ api *url.URL }

func (o v2Objects) Fetch(ctx context.Context, ref ledger.Ref) (io.ReadCloser, error) {
	u := o.api.JoinPath("sn", "ledger", "object")
	u.RawQuery = url.Values{"hash": {ref.String()}}.Encode()
	return v2Get(ctx, u)
}

// cmdV2Check implements `snclaim v2 check --all`.
func cmdV2Check(ctx context.Context, opts docopt.Opts) error {
	vault, err := parseVault2(strOpt(opts, "--vault"))
	if err != nil {
		return err
	}
	noID, _, err := parseUint64Opt(opts, "--no_id")
	if err != nil {
		return err
	}
	api, err := parseAPIURL(strOpt(opts, "--api_url"))
	if err != nil {
		return err
	}
	wantChainID, gasLimit, err := parseChainAndGas(opts)
	if err != nil {
		return err
	}
	challenge, demand := boolOpt(opts, "--challenge"), boolOpt(opts, "--demand")
	var key *ecdsa.PrivateKey
	if challenge || demand {
		if !hasOpt(opts, "--key_file") {
			return errors.New("--challenge and --demand send transactions: --key_file is required")
		}
		if key, err = loadKeyFile(strOpt(opts, "--key_file")); err != nil {
			return err
		}
	}
	client, chainID, rpcURL, err := v2Connect(ctx, opts, wantChainID)
	if err != nil {
		return err
	}
	defer client.Close()

	head, err := ReadEVMBlockIdentity(ctx, client, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil {
		return fmt.Errorf("read the finalized EVM head: %w", err)
	}
	state, err := readV2State(ctx, &v2Reader{client: client, vault: vault, block: new(big.Int).SetUint64(head.Number)}, noID)
	if err != nil {
		return err
	}
	fmt.Printf("settlement v2 ledger check — vault %s, pool %d (chain id %s, rpc %s)\n", vault.Hex(), noID, chainID, rpcURL)
	fmt.Printf("  finalized block:   %d (%s)\n", head.Number, head.Hash)
	pending := &state.pending
	if pending.Seq == 0 {
		fmt.Printf("  pending root:      none; nothing to check\n")
		return nil
	}
	fmt.Printf("  pending root:      seq %d, epoch %d, committed at block %d, %d open demands\n", pending.Seq, pending.Epoch, pending.CommittedBlock, pending.OpenDemands)
	fmt.Printf("                     root 0x%x, total %s, %d accounts, ledger %v\n", pending.Root, formatAlpha(pending.Total), pending.LeafCount, ledger.Ref(pending.LedgerHash))
	in := ledger.VerifyInput{
		NoID: noID, FirstEpoch: state.firstEpoch, Seq: pending.Seq,
		Pending:       v2LedgerRoot(pending.Epoch, pending.Root, pending.Total, pending.LeafCount, pending.LedgerHash),
		FundedThrough: state.funded, Fetcher: v2Objects{api: api}, Custody: ledger.NewMemoryCustody(),
		MaxFetchBytes: ledger.DefaultMaxFetchBytes,
	}
	if previous := state.previous; previous != nil {
		fmt.Printf("  finalized root:    index %s, epoch %d, root 0x%x, total %s, %d accounts\n", state.previousIndex, previous.Epoch, previous.Root, formatAlpha(previous.Total), previous.LeafCount)
		root := v2LedgerRoot(previous.Epoch, previous.Root, previous.Total, previous.LeafCount, previous.LedgerHash)
		in.Previous = &root
	} else {
		fmt.Printf("  finalized root:    none; the pending root is the pool's first\n")
	}
	res, err := ledger.Verify(ctx, in)
	if err != nil {
		return fmt.Errorf("verify the pending root: %w", err)
	}
	reportV2Check(res)

	switch {
	case state.previous == nil:
		return nil
	case res.Previous == nil:
		return errors.New("the finalized root's objects are unavailable or do not rebuild it: its accounts cannot be checked, challenged or demanded")
	}
	sender := &v2Sender{client: client, chainID: chainID, rpcURL: rpcURL, vault: vault, key: key, gasLimit: gasLimit, dryRun: boolOpt(opts, "--dry-run")}
	if key != nil {
		sender.from = crypto.PubkeyToAddress(key.PublicKey)
	}
	switch res.Action() {
	case ledger.ActionChallenge:
		if !challenge {
			return fmt.Errorf("pending root seq %d faults %d accounts of the finalized root; --challenge deletes it", pending.Seq, len(res.Faults))
		}
		return v2Challenge(ctx, sender, noID, state, res)
	case ledger.ActionDemand:
		if !demand {
			return fmt.Errorf("pending root seq %d leaves the finalized root's accounts unverifiable; --demand sends the demand set", pending.Seq)
		}
		return v2Demand(ctx, sender, noID, state, res, head.Number)
	}
	return nil
}

// v2LedgerRoot is a vault2 root as ledger.Verify takes it.
func v2LedgerRoot(epoch uint64, root [32]byte, total *big.Int, leafCount uint64, ledgerHash [32]byte) ledger.Root {
	return ledger.Root{Epoch: epoch, Root: root, Total: *uint256.MustFromBig(total), LeafCount: leafCount, LedgerHash: ledgerHash}
}

// reportV2Check prints the audit and, for the finalized root's accounts,
// no fault, the provable faults, or unverifiable.
func reportV2Check(res *ledger.Result) {
	if res.Err == nil {
		fmt.Printf("  audit:             %v\n", res.Action())
	} else {
		fmt.Printf("  audit:             %v (%v check: %v)\n", res.Action(), res.Check, res.Err)
	}
	for _, ref := range res.Missing {
		fmt.Printf("  object missing:    %v\n", ref)
	}
	if res.Previous == nil {
		return
	}
	n := len(res.PreviousAccounts)
	switch res.Action() {
	case ledger.ActionDemand:
		fmt.Printf("  previous accounts: %d, unverifiable: the published objects are missing or do not reproduce the pending root\n", n)
		for _, fault := range res.Faults {
			fmt.Printf("    account %d:      the published row shows %v\n", fault.Index, fault.Kind)
		}
	case ledger.ActionChallenge:
		fmt.Printf("  previous accounts: %d, %d with a provable fault, %d with no fault\n", n, len(res.Faults), n-len(res.Faults))
		for _, fault := range res.Faults {
			fmt.Printf("    account %d:      %v\n", fault.Index, fault.Kind)
		}
	default:
		fmt.Printf("  previous accounts: %d, no fault\n", n)
	}
}

// v2PendingGone reads, at the latest block, whether pending root seq has
// left the pending slot: deleted, replaced or finalized.
func v2PendingGone(ctx context.Context, s *v2Sender, noID, seq uint64) (bool, error) {
	pending, err := v2View(ctx, &v2Reader{client: s.client, vault: s.vault}, vault2.PackPending(new(big.Int).SetUint64(noID)), vault2.UnpackPending)
	if err != nil {
		return false, fmt.Errorf("pending(%d): %w", noID, err)
	}
	return pending.Seq != seq, nil
}

// v2Challenge sends checkPath for the first provable fault: one deletes the
// root. A root that has left the pending slot, deleted by another
// challenger or replaced, is done (SPEC A6).
func v2Challenge(ctx context.Context, s *v2Sender, noID uint64, state *v2State, res *ledger.Result) error {
	fault := res.Faults[0]
	path, err := res.Tree.Path(fault.Index)
	if err != nil {
		return fmt.Errorf("the pending root has no account %d to open: %w", fault.Index, err)
	}
	// Faults index the finalized root's accounts, so each has a proof.
	proof, _ := res.Previous.Proof(fault.Index)
	account := &res.PreviousAccounts[fault.Index]
	seq := state.pending.Seq
	calldata := vault2.PackCheckPath(new(big.Int).SetUint64(noID), state.previousIndex, v2LedgerAccount(fault.Index, account), v2ProofNodes(proof), v2PathNodes(path))
	receipt, err := s.send(ctx, "challenge", calldata, nil, func() {
		fmt.Printf("  pending:     pool %d, seq %d, root 0x%x\n", noID, seq, state.pending.Root)
		fmt.Printf("  account:     %d of finalized root %s: coldkey 0x%x, cumulative %s\n", fault.Index, state.previousIndex, account.Coldkey, v2Alpha(&account.Cumulative))
		fmt.Printf("  fault:       %v\n", fault.Kind)
	})
	if err != nil {
		if !v2Reverted(err) {
			return err
		}
		gone, rerr := v2PendingGone(ctx, s, noID, seq)
		if rerr != nil {
			return errors.Join(err, rerr)
		}
		if !gone {
			return err
		}
		fmt.Printf("pending root seq %d is no longer pending (deleted or replaced); done\n", seq)
		return nil
	}
	if receipt == nil {
		return nil // dry run
	}
	for _, log := range receipt.Logs {
		if log.Address != s.vault {
			continue
		}
		event, err := vault2.UnpackDistributionDeletedEvent(log)
		if err != nil || event.Seq != seq {
			continue
		}
		fmt.Printf("DistributionDeleted: pool %s, seq %d\n", event.NoId, event.Seq)
		fmt.Printf("  account:     %s\n", event.Index)
		fmt.Printf("  fault:       %v at level %s\n", merkle.LedgerFault(event.Fault), event.Level)
		fmt.Printf("  challenger:  %s\n", event.Challenger.Hex())
		return nil
	}
	return fmt.Errorf("tx %s succeeded but %s emitted no DistributionDeleted for seq %d", receipt.TxHash, s.vault.Hex(), seq)
}

// v2History is what vault2's events since F's finalization say about the
// pending root and its predecessors over F.
type v2History struct {
	// doublings counts the earlier pending roots over F replaced or deleted
	// while a demand was open, and again holds their unanswered indexes.
	doublings uint
	again     []uint64
	// demanded holds the indexes demanded on the pending root, and ours
	// those this sender demanded.
	demanded map[uint64]bool
	ours     map[uint64]bool
}

// v2Event is a decoded vault2 event of the demand history.
type v2Event struct {
	name        string
	seq         uint64
	index       uint64
	replacedSeq uint64
	rootIndex   *big.Int
	sender      common.Address
}

// v2EventID returns the ID of a vault2 event. NewSTSettlementVault2 parsed
// the ABI when the package loaded, so parsing it again cannot fail.
func v2EventID(name string) common.Hash {
	parsed, _ := parsedVault2ABI()
	return parsed.Events[name].ID
}

// decodeV2Event decodes a log of the demand history.
func decodeV2Event(log *types.Log) (v2Event, error) {
	var topic common.Hash
	if len(log.Topics) > 0 {
		topic = log.Topics[0]
	}
	var ev v2Event
	var err error
	switch topic {
	case v2EventID("DistributionCommitted"):
		var e *stabi.STSettlementVault2DistributionCommitted
		if e, err = vault2.UnpackDistributionCommittedEvent(log); err == nil {
			ev = v2Event{name: "DistributionCommitted", seq: e.Seq, replacedSeq: e.ReplacedSeq}
		}
	case v2EventID("DistributionDeleted"):
		var e *stabi.STSettlementVault2DistributionDeleted
		if e, err = vault2.UnpackDistributionDeletedEvent(log); err == nil {
			ev = v2Event{name: "DistributionDeleted", seq: e.Seq}
		}
	case v2EventID("PathDemanded"):
		var e *stabi.STSettlementVault2PathDemanded
		if e, err = vault2.UnpackPathDemandedEvent(log); err == nil {
			ev = v2Event{name: "PathDemanded", seq: e.Seq, index: e.Index.Uint64(), sender: e.Demander}
		}
	case v2EventID("PathAnswered"):
		var e *stabi.STSettlementVault2PathAnswered
		if e, err = vault2.UnpackPathAnsweredEvent(log); err == nil {
			ev = v2Event{name: "PathAnswered", seq: e.Seq, index: e.Index.Uint64()}
		}
	case v2EventID("DistributionFinalized"):
		var e *stabi.STSettlementVault2DistributionFinalized
		if e, err = vault2.UnpackDistributionFinalizedEvent(log); err == nil {
			ev = v2Event{name: "DistributionFinalized", seq: e.Seq, rootIndex: e.RootIndex}
		}
	default:
		err = errors.New("not an event of the demand history")
	}
	if err != nil {
		return ev, fmt.Errorf("vault2 log %d of tx %s at block %d: %w", log.Index, log.TxHash, log.BlockNumber, err)
	}
	return ev, nil
}

// readV2History scans vault2's events of the pool backwards from the
// finalized head, in spans the public endpoint answers, to F's
// DistributionFinalized, and reads the history after it.
func readV2History(ctx context.Context, s *v2Sender, noID uint64, state *v2State, head uint64) (*v2History, error) {
	query := ethereum.FilterQuery{
		Addresses: []common.Address{s.vault},
		Topics: [][]common.Hash{
			{v2EventID("DistributionCommitted"), v2EventID("DistributionDeleted"), v2EventID("PathDemanded"), v2EventID("PathAnswered"), v2EventID("DistributionFinalized")},
			{common.BigToHash(new(big.Int).SetUint64(noID))},
		},
	}
	var events []v2Event
	for to := head; ; {
		from := to - min(to, v2LogChunkBlocks-1)
		query.FromBlock, query.ToBlock = new(big.Int).SetUint64(from), new(big.Int).SetUint64(to)
		logs, err := retryOnchainRead(ctx, func(ctx context.Context) ([]types.Log, error) {
			cctx, cancel := context.WithTimeout(ctx, callTimeout)
			defer cancel()
			return s.client.FilterLogs(cctx, query)
		})
		if err != nil {
			return nil, fmt.Errorf("eth_getLogs [%d,%d]: %w", from, to, err)
		}
		chunk := make([]v2Event, len(logs))
		start := -1
		for i := range logs {
			if chunk[i], err = decodeV2Event(&logs[i]); err != nil {
				return nil, err
			}
			if chunk[i].name == "DistributionFinalized" && chunk[i].rootIndex.Cmp(state.previousIndex) == 0 {
				start = i
			}
		}
		if start >= 0 {
			events = append(chunk[start+1:], events...)
			break
		}
		events = append(chunk, events...)
		if from == 0 {
			return nil, fmt.Errorf("vault2 logs up to block %d hold no DistributionFinalized of root %s of pool %d", head, state.previousIndex, noID)
		}
		to = from - 1
	}

	history := &v2History{demanded: map[uint64]bool{}, ours: map[uint64]bool{}}
	gone := map[uint64]bool{}
	open := map[uint64]map[uint64]bool{}
	for _, ev := range events {
		switch ev.name {
		case "DistributionCommitted":
			if ev.replacedSeq != 0 {
				gone[ev.replacedSeq] = true
			}
		case "DistributionDeleted":
			gone[ev.seq] = true
		case "PathDemanded":
			if open[ev.seq] == nil {
				open[ev.seq] = map[uint64]bool{}
			}
			open[ev.seq][ev.index] = true
			if ev.seq == state.pending.Seq {
				history.demanded[ev.index] = true
				if ev.sender == s.from {
					history.ours[ev.index] = true
				}
			}
		case "PathAnswered":
			delete(open[ev.seq], ev.index)
		}
	}
	for seq := range gone {
		if len(open[seq]) > 0 {
			history.doublings++
			for index := range open[seq] {
				history.again = append(history.again, index)
			}
		}
	}
	return history, nil
}

// v2Demand sends the §9.3 demand set. A sender draws its sample once per
// pending root: once it holds as many sampled demands on the root as the
// sample size, later runs demand only the forced indexes, so a watcher run
// again does not pay for a fresh sample. An index already demanded is done
// (SPEC A6); a pending root that leaves the slot ends the demands.
func v2Demand(ctx context.Context, s *v2Sender, noID uint64, state *v2State, res *ledger.Result, head uint64) error {
	history, err := readV2History(ctx, s, noID, state, head)
	if err != nil {
		return err
	}
	forced := slices.Clone(history.again)
	for _, fault := range res.Faults {
		forced = append(forced, fault.Index)
	}
	slices.Sort(forced)
	forced = slices.Compact(forced)
	sampled := 0
	for index := range history.ours {
		if !slices.Contains(forced, index) {
			sampled++
		}
	}
	set := forced
	if history.doublings >= 32 || uint64(sampled) < ledger.DemandSample<<history.doublings {
		if set, err = ledger.DemandSet(res.PreviousAccounts, res.Faults, history.doublings, history.again, v2Random); err != nil {
			return err
		}
	}
	fee, err := v2View(ctx, &v2Reader{client: s.client, vault: s.vault}, vault2.PackDemandFeeWei(), vault2.UnpackDemandFeeWei)
	if err != nil {
		return fmt.Errorf("demandFeeWei(): %w", err)
	}
	seq, n := state.pending.Seq, new(big.Int).SetUint64(noID)
	fmt.Printf("demand set: %d accounts (%d doublings, %d forced), %d already demanded\n", len(set), history.doublings, len(forced), len(history.demanded))
	for _, index := range set {
		if history.demanded[index] {
			continue
		}
		// The set indexes the finalized root's accounts, so each has a proof.
		proof, _ := res.Previous.Proof(index)
		account := &res.PreviousAccounts[index]
		receipt, err := s.send(ctx, "demand", vault2.PackDemandPath(n, state.previousIndex, v2LedgerAccount(index, account), v2ProofNodes(proof)), fee, func() {
			fmt.Printf("  pending:     pool %d, seq %d\n", noID, seq)
			fmt.Printf("  account:     %d of finalized root %s: coldkey 0x%x, cumulative %s\n", index, state.previousIndex, account.Coldkey, v2Alpha(&account.Cumulative))
			fmt.Printf("  fee:         %s wei\n", fee)
		})
		if err != nil {
			if !v2Reverted(err) {
				return err
			}
			gone, rerr := v2PendingGone(ctx, s, noID, seq)
			if rerr != nil {
				return errors.Join(err, rerr)
			}
			if gone {
				fmt.Printf("pending root seq %d is no longer pending (deleted or replaced); no more demands\n", seq)
				return nil
			}
			demanded, rerr := v2View(ctx, &v2Reader{client: s.client, vault: s.vault}, vault2.PackDemanded(n, seq, state.previousIndex, new(big.Int).SetUint64(index)), vault2.UnpackDemanded)
			if rerr != nil {
				return errors.Join(err, fmt.Errorf("demanded(%d,%d,%d): %w", noID, seq, index, rerr))
			}
			if demanded == 0 {
				return err
			}
			fmt.Printf("account %d: already demanded on seq %d; done\n", index, seq)
			continue
		}
		if receipt == nil {
			continue // dry run
		}
		if !v2Demanded(receipt, s.vault, seq, index) {
			return fmt.Errorf("tx %s succeeded but %s emitted no PathDemanded for seq %d account %d", receipt.TxHash, s.vault.Hex(), seq, index)
		}
		fmt.Printf("PathDemanded: pool %d, seq %d, account %d\n", noID, seq, index)
	}
	return nil
}

// v2Demanded reports a PathDemanded of seq and index in the receipt.
func v2Demanded(receipt *types.Receipt, vault common.Address, seq, index uint64) bool {
	for _, log := range receipt.Logs {
		if log.Address != vault {
			continue
		}
		if event, err := vault2.UnpackPathDemandedEvent(log); err == nil && event.Seq == seq && event.Index.IsUint64() && event.Index.Uint64() == index {
			return true
		}
	}
	return false
}
