// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {SafeCast} from "@openzeppelin/contracts/utils/math/SafeCast.sol";

import {ISTSettlementVault2} from "./interfaces/ISTSettlementVault2.sol";
import {IStaking, ISTAKING_ADDRESS} from "./interfaces/stakingV2.sol";
import {IStakingCollateral} from "./interfaces/stakingCollateral.sol";
import {INeuron, INeuron_ADDRESS} from "./interfaces/neuron.sol";
import {LedgerTree} from "./LedgerTree.sol";
import {NativeBalance} from "./NativeBalance.sol";

/// @title STSettlementVault2
/// @notice Immutable custody for settlement v2 (VERSION2-FINAL.md §7.1). It
/// captures each operator pool's emission into one escrow hotkey, keeps each
/// pool's pending root and every finalized root of its ledger sum tree, and
/// pays cumulative account balances against them. The coordinator decides when
/// epochs are captured and which roots are committed and finalized; whatever
/// it does, this vault enforces conservation, the funding binding of every
/// root, the challenge window, path challenges and demands, and the
/// destination of every payment (§4.2 G1-G6). Nothing here can be upgraded,
/// paused or redirected, and a valid claim cannot be stopped.
contract STSettlementVault2 is ISTSettlementVault2 {
    uint64 public constant CHALLENGE_BLOCKS = 14_400;

    // The staking precompile reads an amount of 2^64 − 1 or more as "the
    // whole position" (§7.3); every amount passed to it stays below this.
    uint256 private constant WHOLE_POSITION = type(uint64).max;

    // One rao in wei. The runtime truncates native value below a rao.
    uint256 private constant RAO_WEI = 1e9;

    uint16 public immutable netuid;
    bytes32 public immutable escrowHotkey;
    bytes32 public immutable selfColdkey;
    address public immutable bootstrap;
    uint256 public immutable demandFeeWei;

    address public coordinator;
    bool public escrowRegistered;
    uint256 private _entered;

    mapping(uint256 noId => Pool pool) private _pools;
    mapping(bytes32 hotkey => bool used) public poolHotkeyUsed;
    mapping(uint256 noId => uint256 epoch) public lastFundedEpoch;
    mapping(uint256 noId => mapping(uint256 epoch => uint256 amount)) public fundedThrough;
    mapping(uint256 noId => uint256 amount) public capturedTotal;
    mapping(uint256 noId => uint256 amount) public claimedTotal;
    mapping(uint256 noId => Root[] history) private _roots;
    mapping(uint256 noId => Pending root) private _pending;
    mapping(uint256 noId => uint64 seq) public commitSeq;
    mapping(uint256 noId => uint64 blockNumber) public lastCommitBlock;
    mapping(
        uint256 noId
            => mapping(
            uint64 seq => mapping(uint256 rootIndex => mapping(uint256 index => DemandState state))
        )
    ) public demanded;
    mapping(uint256 noId => mapping(uint256 index => uint256 amount)) public claimed;

    uint256 public totalCaptured;
    uint256 public totalPaid;
    uint256 public escrowAccounted;

    modifier onlyBootstrap() {
        if (msg.sender != bootstrap) revert Unauthorized();
        _;
    }

    modifier onlyCoordinator() {
        if (msg.sender != coordinator) revert Unauthorized();
        _;
    }

    // One guard shared by every state-changing entry point, so no runtime or
    // refund callback can enter the vault while another call is in progress.
    modifier nonReentrant() {
        if (_entered != 0) revert Reentrancy();
        _entered = 1;
        _;
        _entered = 0;
    }

    constructor(
        uint16 netuid_,
        bytes32 escrowHotkey_,
        bytes32 selfColdkey_,
        address bootstrap_,
        uint256 demandFeeWei_
    ) {
        if (
            netuid_ == 0 || escrowHotkey_ == bytes32(0) || selfColdkey_ == bytes32(0)
                || bootstrap_ == address(0) || demandFeeWei_ == 0 || demandFeeWei_ % RAO_WEI != 0
        ) revert InvalidConfiguration();
        netuid = netuid_;
        escrowHotkey = escrowHotkey_;
        selfColdkey = selfColdkey_;
        bootstrap = bootstrap_;
        demandFeeWei = demandFeeWei_;
    }

    function setCoordinatorOnce(address coordinator_) external onlyBootstrap nonReentrant {
        if (coordinator != address(0)) revert AlreadyInitialized();
        if (coordinator_ == address(0) || coordinator_.code.length == 0) revert InvalidConfiguration();
        coordinator = coordinator_;
        emit CoordinatorFixed(coordinator_);
    }

    /// @notice Limit-registers the immutable escrow hotkey under this vault's
    /// mapped coldkey. The bootstrap may do this exactly once; no external key
    /// ever controls the hotkey.
    function registerEscrow(uint64 maximumBurnRao)
        external
        payable
        onlyBootstrap
        nonReentrant
        returns (uint16 uid)
    {
        if (escrowRegistered) revert AlreadyInitialized();
        if (maximumBurnRao == 0) revert InvalidConfiguration();
        escrowRegistered = true;
        uid = _register(escrowHotkey, maximumBurnRao);
        emit EscrowRegistered(escrowHotkey, uid);
    }

    function registerPool(uint256 noId, bytes32 hotkey, uint64 maximumBurnRao, uint64 firstEpoch)
        external
        payable
        onlyCoordinator
        nonReentrant
        returns (uint16 uid)
    {
        if (
            !escrowRegistered || noId == 0 || noId > type(uint64).max || hotkey == bytes32(0)
                || hotkey == escrowHotkey || poolHotkeyUsed[hotkey] || maximumBurnRao == 0 || firstEpoch == 0
                || _pools[noId].registered
        ) revert InvalidConfiguration();
        _pools[noId] = Pool({hotkey: hotkey, firstEpoch: firstEpoch, registered: true});
        poolHotkeyUsed[hotkey] = true;
        lastFundedEpoch[noId] = firstEpoch - 1;
        uid = _register(hotkey, maximumBurnRao);
        emit PoolRegistered(noId, hotkey, uid, firstEpoch);
    }

    /// @notice Re-registers a pool hotkey that lost its UID to pruning. Pruning
    /// keeps the hotkey's stake and owner, and only this vault's coldkey can
    /// register it again (§7.3). The caller pays the burn.
    function registerPoolUid(uint256 noId, uint64 maximumBurnRao)
        external
        payable
        nonReentrant
        returns (uint16 uid)
    {
        Pool storage pool = _pools[noId];
        if (!pool.registered) revert UnknownPool();
        bytes32 hotkey = pool.hotkey;
        // Only whether a UID exists matters here.
        // slither-disable-next-line unused-return
        (bool exists,) = INeuron(INeuron_ADDRESS).getUid(netuid, hotkey);
        if (exists) revert UidExists();
        if (maximumBurnRao == 0) revert InvalidConfiguration();
        uid = _register(hotkey, maximumBurnRao);
        emit PoolUidRegistered(noId, hotkey, uid, msg.sender);
    }

    /// @notice Funds epoch `epoch` with all the stake this vault can move off
    /// the pool hotkey, moved into escrow. Nothing movable funds zero without a
    /// runtime call. Effects follow the move because the amount is escrow's
    /// gain; the shared guard keeps them atomic with it.
    function captureEmission(uint256 epoch, uint256 noId)
        external
        onlyCoordinator
        nonReentrant
        returns (uint256 amount)
    {
        bytes32 hotkey = _advance(epoch, noId);
        uint256 observed = _movable(hotkey);
        if (observed != 0) {
            amount = _moveToEscrow(hotkey, observed);
            capturedTotal[noId] += amount;
            totalCaptured += amount;
            escrowAccounted += amount;
        }
        uint256 funded = _snapshot(epoch, noId);
        _requireBacking();
        emit EmissionCaptured(epoch, noId, observed, amount, funded);
    }

    /// @notice Funds epoch `epoch` with nothing. Its emission stays on the pool
    /// hotkey for the next capture.
    function deferEmission(uint256 epoch, uint256 noId) external onlyCoordinator nonReentrant {
        _advance(epoch, noId);
        emit EmissionDeferred(epoch, noId, _snapshot(epoch, noId));
    }

    /// @notice Replaces the pool's pending root, and with it every demand on
    /// it. The root's total must equal the funding snapshot of its epoch and
    /// exceed the latest finalized total; its accounts extend the latest
    /// finalized root's. Commits are spaced by the challenge window so that a
    /// demand sample cannot be re-rolled quickly (§4.2 G2).
    function commitPending(uint256 epoch, uint256 noId, uint256 prevRootCount, Commitment calldata commitment)
        external
        onlyCoordinator
        nonReentrant
        returns (uint64 seq)
    {
        Pool storage pool = _pools[noId];
        if (!pool.registered) revert UnknownPool();
        uint64 lastCommit = lastCommitBlock[noId];
        if (lastCommit != 0 && block.number < uint256(lastCommit) + CHALLENGE_BLOCKS) revert CommitTooSoon();
        Root[] storage history = _roots[noId];
        if (prevRootCount != history.length) revert StaleRootCount();
        // With no finalized root, the pool starts empty before its first epoch.
        Root memory previous = Root({
            root: bytes32(0), total: 0, leafCount: 0, epoch: pool.firstEpoch - 1, ledgerHash: bytes32(0)
        });
        if (history.length != 0) previous = history[history.length - 1];
        if (epoch <= previous.epoch || epoch > lastFundedEpoch[noId]) revert EpochOutOfRange();
        if (commitment.total != fundedThrough[noId][epoch]) revert TotalMismatch();
        if (commitment.total <= previous.total) revert TotalNotIncreased();
        if (
            commitment.leafCount == 0 || commitment.leafCount > LedgerTree.MAX_LEAF_COUNT
                || commitment.leafCount < previous.leafCount
        ) revert InvalidLeafCount();
        if (commitment.root == bytes32(0) || commitment.ledgerHash == bytes32(0)) revert InvalidRoot();

        uint64 replacedSeq = _pending[noId].seq;
        seq = ++commitSeq[noId];
        uint64 committedEpoch = SafeCast.toUint64(epoch);
        uint64 committedBlock = SafeCast.toUint64(block.number);
        _pending[noId] = Pending({
            seq: seq,
            root: commitment.root,
            total: commitment.total,
            leafCount: commitment.leafCount,
            epoch: committedEpoch,
            ledgerHash: commitment.ledgerHash,
            committedBlock: committedBlock,
            openDemands: 0
        });
        lastCommitBlock[noId] = committedBlock;
        emit DistributionCommitted(
            noId,
            seq,
            committedEpoch,
            commitment.root,
            commitment.total,
            commitment.leafCount,
            commitment.ledgerHash,
            replacedSeq
        );
    }

    /// @notice Demands, during the challenge window, the on-chain opening in
    /// the pending root of an account proven against the finalized root
    /// `rootIndex`. Any finalized root will do, so withholding the newest tree
    /// cannot stop demands on the accounts of an older one (§4.2 G2). The
    /// pending root cannot finalize until checkPath answers it, so a withheld
    /// or garbage tree blocks itself (§2.2); only a provable account can be
    /// demanded, so an honest successor, which keeps every account at its
    /// value or more (§6.1), can always answer. Each account of each finalized
    /// root can be demanded once per pending root. The fee stays in this vault.
    function demandPath(
        uint256 noId,
        uint256 rootIndex,
        LedgerTree.Account calldata account,
        LedgerTree.ProofNode[] calldata oldProof
    ) external payable nonReentrant {
        if (msg.value != demandFeeWei) revert InvalidDemandFee();
        (Pending storage next, Root storage previous) = _disputedRoots(noId, rootIndex);
        if (block.number >= uint256(next.committedBlock) + CHALLENGE_BLOCKS) revert ChallengeWindowClosed();
        _requireProven(previous, account, oldProof);
        uint64 seq = next.seq;
        uint256 index = account.index;
        mapping(uint256 => DemandState) storage demands = demanded[noId][seq][rootIndex];
        if (demands[index] != DemandState.None) revert AlreadyDemanded();
        demands[index] = DemandState.Open;
        next.openDemands++;
        emit PathDemanded(noId, seq, rootIndex, index, msg.sender);
    }

    /// @notice The §6.5 path check of an account of the finalized root
    /// `rootIndex` against the pending root. A fault deletes the pending root;
    /// an opening with no fault answers an open demand on that account of that
    /// root; anything else reverts. Works at any time while the root is
    /// pending, and never checks backing, so a deficit cannot freeze disputes.
    function checkPath(
        uint256 noId,
        uint256 rootIndex,
        LedgerTree.Account calldata account,
        LedgerTree.ProofNode[] calldata oldProof,
        LedgerTree.PathNode[] calldata newPath
    ) external nonReentrant returns (LedgerTree.Fault fault) {
        (Pending storage next, Root storage previous) = _disputedRoots(noId, rootIndex);
        _requireProven(previous, account, oldProof);
        uint64 seq = next.seq;
        uint256 index = account.index;
        // Leaf counts never decrease from one root to the next (commitPending),
        // so `index` is inside the pending root.
        uint256 level;
        (fault, level) = LedgerTree.walkPath(next.root, next.total, next.leafCount, account, newPath);
        if (fault != LedgerTree.Fault.None) {
            delete _pending[noId];
            emit DistributionDeleted(noId, seq, rootIndex, index, fault, level, msg.sender);
            return fault;
        }
        mapping(uint256 => DemandState) storage demands = demanded[noId][seq][rootIndex];
        if (demands[index] != DemandState.Open) revert NoFault();
        demands[index] = DemandState.Answered;
        next.openDemands--;
        emit PathAnswered(noId, seq, rootIndex, index, msg.sender);
    }

    /// @notice Appends the pending root to the pool's finalized roots once its
    /// challenge window has passed with no open demand (§4.2 G6).
    function finalizePending(uint256 noId) external onlyCoordinator nonReentrant returns (uint256 rootIndex) {
        Pending memory next = _pending[noId];
        if (next.seq == 0) revert NoPendingRoot();
        if (block.number < uint256(next.committedBlock) + CHALLENGE_BLOCKS) revert ChallengeWindowOpen();
        if (next.openDemands != 0) revert DemandOpen();
        rootIndex = _roots[noId].length;
        _roots[noId].push(
            Root({
                root: next.root,
                total: next.total,
                leafCount: next.leafCount,
                epoch: next.epoch,
                ledgerHash: next.ledgerHash
            })
        );
        delete _pending[noId];
        emit DistributionFinalized(
            noId, next.seq, rootIndex, next.epoch, next.root, next.total, next.leafCount, next.ledgerHash
        );
    }

    /// @notice Pays account `account.index` of any finalized root everything it
    /// accrued since its last claim, always to the coldkey in its leaf, whoever
    /// calls (§4.2 G3, G4). A pool never pays out more than its latest
    /// finalized total. Any refusal reverts the whole claim, which stays
    /// claimable.
    function claim(
        uint256 noId,
        uint256 rootIndex,
        LedgerTree.Account calldata account,
        LedgerTree.ProofNode[] calldata proof
    ) external nonReentrant returns (uint256 amount) {
        Root[] storage history = _roots[noId];
        if (rootIndex >= history.length) revert UnknownRoot();
        Root storage finalized = history[rootIndex];
        bytes32 coldkey = account.coldkey;
        if (coldkey == bytes32(0) || coldkey == selfColdkey) revert InvalidColdkey();
        if (!LedgerTree.verifyLeaf(finalized.root, finalized.total, finalized.leafCount, account, proof)) {
            revert InvalidProof();
        }
        uint256 paid = claimed[noId][account.index];
        if (account.cumulative <= paid) revert NothingToClaim();
        amount = account.cumulative - paid;
        if (amount >= WHOLE_POSITION) revert AmountTooLarge();
        if (claimedTotal[noId] + amount > history[history.length - 1].total) revert PoolCapExceeded();

        claimed[noId][account.index] = account.cumulative;
        claimedTotal[noId] += amount;
        totalPaid += amount;
        escrowAccounted -= amount;
        _payFromEscrow(coldkey, amount);
        _requireBacking();
        emit Claimed(noId, rootIndex, account.index, coldkey, account.cumulative, amount, msg.sender);
    }

    /// @notice Moves all the stake this vault's coldkey can move off a foreign
    /// hotkey, such as the v1 escrow, into escrow. Nothing is accounted: the
    /// stake becomes surplus that the coordinator may assign to a pool.
    function absorbStake(bytes32 hotkey) external nonReentrant returns (uint256 amount) {
        if (hotkey == bytes32(0) || hotkey == escrowHotkey || poolHotkeyUsed[hotkey]) revert InvalidHotkey();
        uint256 observed = _movable(hotkey);
        if (observed == 0) revert NothingToAbsorb();
        amount = _moveToEscrow(hotkey, observed);
        _requireBacking();
        emit StakeAbsorbed(hotkey, observed, amount, msg.sender);
    }

    /// @notice Adds escrow stake that no pool accounts for to a pool's funding.
    /// It enters the funding snapshot of the pool's next epoch.
    function assignSurplus(uint256 noId, uint256 amount) external onlyCoordinator nonReentrant {
        if (!_pools[noId].registered) revert UnknownPool();
        uint256 live = _liveEscrowStake();
        if (amount == 0 || live < escrowAccounted || amount > live - escrowAccounted) {
            revert InvalidSurplus();
        }
        capturedTotal[noId] += amount;
        totalCaptured += amount;
        escrowAccounted += amount;
        _requireBacking();
        emit SurplusAssigned(noId, lastFundedEpoch[noId] + 1, amount);
    }

    function pools(uint256 noId) external view returns (Pool memory) {
        return _pools[noId];
    }

    function finalizedTotal(uint256 noId) external view returns (uint256) {
        Root[] storage history = _roots[noId];
        return history.length == 0 ? 0 : history[history.length - 1].total;
    }

    function rootCount(uint256 noId) external view returns (uint256) {
        return _roots[noId].length;
    }

    function roots(uint256 noId, uint256 rootIndex) external view returns (Root memory) {
        return _roots[noId][rootIndex];
    }

    function pending(uint256 noId) external view returns (Pending memory) {
        return _pending[noId];
    }

    /// @notice Executable conservation identity (§4.2 G1).
    function conservationHolds() external view returns (bool) {
        return totalCaptured == totalPaid + escrowAccounted && _liveEscrowStake() >= escrowAccounted;
    }

    function liveEscrowStake() external view returns (uint256) {
        return _liveEscrowStake();
    }

    // Limit-registers `hotkey`, refunding to the caller whatever msg.value the
    // burn left. Runtime 452 charges the burn to this contract's SS58-mirror
    // balance, which msg.value funds before execution; forwarding it to the
    // precompile would move it away before the dispatch can burn it. The burn
    // may spend msg.value but never the balance the vault held before the
    // call, such as demand fees.
    function _register(bytes32 hotkey, uint64 maximumBurnRao) private returns (uint16 uid) {
        uint256 balanceBefore = NativeBalance.beforeSuppliedValue(address(this).balance, msg.value);
        INeuron(INeuron_ADDRESS).registerLimit(netuid, hotkey, maximumBurnRao);
        bool exists;
        (exists, uid) = INeuron(INeuron_ADDRESS).getUid(netuid, hotkey);
        if (!exists) revert UnknownPool();
        uint256 current = address(this).balance;
        if (current < balanceBefore) revert NativeBalanceDecreased();
        if (current > balanceBefore) {
            (bool refunded,) = payable(msg.sender).call{value: current - balanceBefore}("");
            if (!refunded) revert NativeRefundFailed();
        }
    }

    // Records `epoch`, which must be the pool's next unfunded epoch, as funded
    // before any runtime call, and returns the pool hotkey.
    function _advance(uint256 epoch, uint256 noId) private returns (bytes32) {
        Pool storage pool = _pools[noId];
        if (!pool.registered) revert UnknownPool();
        if (epoch != lastFundedEpoch[noId] + 1) revert InvalidTransition();
        lastFundedEpoch[noId] = epoch;
        return pool.hotkey;
    }

    // Writes the funding snapshot of the epoch just advanced to, once (§4.2 G5).
    function _snapshot(uint256 epoch, uint256 noId) private returns (uint256 funded) {
        funded = capturedTotal[noId];
        fundedThrough[noId][epoch] = funded;
    }

    // Moves `observed` of the vault's stake on `hotkey` into escrow and returns
    // escrow's gain. The runtime moves exactly the requested amount out of the
    // source, and escrow's share-pool read can be one rao short (§7.3).
    // Anything else reverts. The deltas compare whole positions, locks included.
    function _moveToEscrow(bytes32 hotkey, uint256 observed) private returns (uint256) {
        if (observed >= WHOLE_POSITION) revert AmountTooLarge();
        uint256 sourceBefore = _stake(hotkey, selfColdkey);
        uint256 escrowBefore = _stake(escrowHotkey, selfColdkey);
        IStaking(ISTAKING_ADDRESS).moveStake(hotkey, escrowHotkey, uint256(netuid), uint256(netuid), observed);
        uint256 escrowAfter = _stake(escrowHotkey, selfColdkey);
        if (
            _stake(hotkey, selfColdkey) + observed != sourceBefore || escrowAfter > escrowBefore + observed
                || escrowAfter + 1 < escrowBefore + observed
        ) revert RuntimeAccountingMismatch();
        return escrowAfter - escrowBefore;
    }

    // Transfers `amount` of escrow stake to `coldkey`, on the escrow hotkey.
    // Escrow must lose exactly `amount`; the destination's read can be one rao
    // short (§7.3). Anything else reverts.
    function _payFromEscrow(bytes32 coldkey, uint256 amount) private {
        uint256 escrowBefore = _stake(escrowHotkey, selfColdkey);
        uint256 destinationBefore = _stake(escrowHotkey, coldkey);
        IStaking(ISTAKING_ADDRESS)
            .transferStake(coldkey, escrowHotkey, uint256(netuid), uint256(netuid), amount);
        uint256 destinationAfter = _stake(escrowHotkey, coldkey);
        if (
            _stake(escrowHotkey, selfColdkey) + amount != escrowBefore
                || destinationAfter > destinationBefore + amount
                || destinationAfter + 1 < destinationBefore + amount
        ) revert RuntimeAccountingMismatch();
    }

    // The pool's pending root and the finalized root `rootIndex` whose
    // accounts it must keep: the two roots a demand or path check is about.
    function _disputedRoots(uint256 noId, uint256 rootIndex)
        private
        view
        returns (Pending storage next, Root storage previous)
    {
        next = _pending[noId];
        if (next.seq == 0) revert NoPendingRoot();
        Root[] storage history = _roots[noId];
        if (rootIndex >= history.length) revert UnknownRoot();
        previous = history[rootIndex];
    }

    // Requires `account` to be an account of the finalized root `previous`.
    function _requireProven(
        Root storage previous,
        LedgerTree.Account calldata account,
        LedgerTree.ProofNode[] calldata proof
    ) private view {
        if (account.index >= previous.leafCount) {
            revert IndexOutOfRange();
        }
        if (!LedgerTree.verifyLeaf(previous.root, previous.total, previous.leafCount, account, proof)) {
            revert InvalidProof();
        }
    }

    function _stake(bytes32 hotkey, bytes32 coldkey) private view returns (uint256) {
        return IStaking(ISTAKING_ADDRESS).getStake(hotkey, coldkey, uint256(netuid));
    }

    // The stake this vault can move off `hotkey`: its position less the
    // registration collateral that runtime 475 locks on it. getStake includes
    // the lock, and a move that digs into it fails StakeUnavailable.
    function _movable(bytes32 hotkey) private view returns (uint256) {
        uint256 staked = _stake(hotkey, selfColdkey);
        // Only the locked amount limits a move; the other fields say how it drains.
        // slither-disable-next-line unused-return
        (, uint64 locked,,,) =
            IStakingCollateral(ISTAKING_ADDRESS).getMinerCollateral(netuid, hotkey, selfColdkey);
        return staked > locked ? staked - locked : 0;
    }

    // Escrow's movable stake: the backing of everything the vault accounts for.
    function _liveEscrowStake() private view returns (uint256) {
        return _movable(escrowHotkey);
    }

    function _requireBacking() private view {
        if (_liveEscrowStake() < escrowAccounted) revert Underfunded();
    }
}
