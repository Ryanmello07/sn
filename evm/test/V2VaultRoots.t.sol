// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {V2VaultBase} from "./utils/V2VaultBase.sol";
import {V2TreeHelper} from "./utils/V2TreeHelper.sol";
import {ISTSettlementVault2} from "../src/interfaces/ISTSettlementVault2.sol";
import {LedgerTree} from "../src/LedgerTree.sol";
import {SafeCast} from "@openzeppelin/contracts/utils/math/SafeCast.sol";

/// @dev Commits, path checks, demands and finalization (VERSION2-FINAL.md
///      §15.3 V-7 to V-11). The finalized root F has five accounts (depth 3,
///      total 150). Its honest successor P keeps them in place, raises three
///      and appends a sixth (total 200). Faulty successors keep P's total, as
///      vault2's funding binding forces them to.
contract V2VaultRootsTest is V2VaultBase {
    uint256 internal constant CAROL_INDEX = 2;
    uint256 internal constant F_TOTAL = 150;
    uint256 internal constant P_TOTAL = 200;

    function _oldKeys() internal pure returns (bytes32[] memory keys) {
        keys = new bytes32[](5);
        (keys[0], keys[1], keys[2], keys[3], keys[4]) = (ALICE, BOB, CAROL, DAVE, ERIN);
    }

    function _oldValues() internal pure returns (uint256[] memory values) {
        values = new uint256[](5);
        (values[0], values[1], values[2], values[3], values[4]) = (10, 20, 30, 40, 50);
    }

    function _newKeys() internal pure returns (bytes32[] memory) {
        return _keys(ALICE, BOB, CAROL, DAVE, ERIN, FRANK);
    }

    function _newValues() internal pure returns (uint256[] memory) {
        return _values(15, 20, 35, 40, 60, 30);
    }

    // F, finalized as root 0 for FIRST_EPOCH.
    function _publishF() internal returns (V2TreeHelper.Node[][] memory) {
        return _publish(NO1, _oldKeys(), _oldValues());
    }

    function _honestP() internal pure returns (V2TreeHelper.Node[][] memory) {
        return V2TreeHelper.build(_newKeys(), _newValues());
    }

    // Carol is lowered from 30 to 29 and Frank takes the rao.
    function _loweredP() internal pure returns (V2TreeHelper.Node[][] memory) {
        uint256[] memory values = _newValues();
        (values[2], values[5]) = (29, 36);
        return V2TreeHelper.build(_newKeys(), values);
    }

    // Carol's position goes to Mallory.
    function _movedP() internal pure returns (V2TreeHelper.Node[][] memory) {
        bytes32[] memory keys = _newKeys();
        keys[2] = MALLORY;
        return V2TreeHelper.build(keys, _newValues());
    }

    // Carol's position holds a hash that is no leaf hash at all.
    function _garbageLeafP() internal pure returns (V2TreeHelper.Node[][] memory levels) {
        levels = _honestP();
        levels[0][CAROL_INDEX].hash = keccak256("garbage-leaf");
        V2TreeHelper.rehash(levels, 0);
    }

    // The node at `level` on Carol's path states 5 more than its children
    // hold, and Frank holds 5 less, so the root still sums to P_TOTAL. At
    // level 3 the root's own sum, the committed total, is the inconsistent one.
    function _inconsistentP(uint256 level) internal pure returns (V2TreeHelper.Node[][] memory levels) {
        uint256[] memory values = _newValues();
        values[5] = 25;
        levels = V2TreeHelper.build(_newKeys(), values);
        levels[level][CAROL_INDEX >> level].sum += 5;
        V2TreeHelper.rehash(levels, level);
    }

    // The node above Carol and Dave is withheld: its hash has no known preimage.
    function _withheldNodeP() internal pure returns (V2TreeHelper.Node[][] memory levels) {
        levels = _honestP();
        levels[1][CAROL_INDEX >> 1].hash = keccak256("withheld-node");
        V2TreeHelper.rehash(levels, 1);
    }

    function _check(
        V2TreeHelper.Node[][] memory f,
        V2TreeHelper.Node[][] memory p,
        uint256 index,
        bytes32 coldkey,
        uint256 cumulative
    ) internal returns (LedgerTree.Fault) {
        return vault.checkPath(
            NO1,
            _account(index, coldkey, cumulative),
            V2TreeHelper.proof(f, index),
            V2TreeHelper.path(p, index)
        );
    }

    function _checkCarol(V2TreeHelper.Node[][] memory f, V2TreeHelper.Node[][] memory p)
        internal
        returns (LedgerTree.Fault)
    {
        return _check(f, p, CAROL_INDEX, CAROL, 30);
    }

    function _demand(uint256 index) internal {
        vm.deal(stranger, DEMAND_FEE);
        vm.prank(stranger);
        vault.demandPath{value: DEMAND_FEE}(NO1, index);
    }

    function _raw(bytes32 root, uint256 total, uint64 leafCount)
        internal
        pure
        returns (ISTSettlementVault2.Commitment memory)
    {
        return ISTSettlementVault2.Commitment({
            root: root, total: total, leafCount: leafCount, ledgerHash: _ledgerHash(root)
        });
    }

    // Shows `fault` at `level` on Carol's path deleting the pending root `p`.
    function _assertCarolDeletes(
        V2TreeHelper.Node[][] memory f,
        V2TreeHelper.Node[][] memory p,
        LedgerTree.Fault fault,
        uint256 level
    ) internal {
        uint64 seq = _propose(NO1, p, 6);
        uint64 committedBlock = vault.lastCommitBlock(NO1);
        LedgerTree.Account memory carol = _account(CAROL_INDEX, CAROL, 30);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, CAROL_INDEX);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(p, CAROL_INDEX);

        vm.expectEmit(address(vault));
        emit ISTSettlementVault2.DistributionDeleted(NO1, seq, CAROL_INDEX, fault, level, stranger);
        vm.prank(stranger);
        assertEq(uint8(vault.checkPath(NO1, carol, oldProof, newPath)), uint8(fault));

        ISTSettlementVault2.Pending memory cleared = vault.pending(NO1);
        assertEq(cleared.seq, 0);
        assertEq(cleared.root, bytes32(0));
        assertEq(cleared.total, 0);
        assertEq(vault.rootCount(NO1), 1);
        assertEq(vault.finalizedTotal(NO1), F_TOTAL);
        assertEq(vault.lastCommitBlock(NO1), committedBlock, "a deletion keeps the commit spacing");
        assertEq(vault.commitSeq(NO1), seq);
    }

    // ------------------------------------------------------------------
    // V-7: commitPending
    // ------------------------------------------------------------------

    // V-7, V-EV
    function test_V7_commitRecordsThePendingRoot() public {
        uint256 epoch = _capture(NO1, F_TOTAL);
        V2TreeHelper.Node[][] memory f = V2TreeHelper.build(_oldKeys(), _oldValues());
        ISTSettlementVault2.Commitment memory commitment = _commitment(f, 5);

        vm.expectEmit(address(vault));
        emit ISTSettlementVault2.DistributionCommitted(
            NO1, 1, uint64(epoch), commitment.root, F_TOTAL, 5, commitment.ledgerHash, 0
        );
        assertEq(vault.commitPending(epoch, NO1, 0, commitment), 1);

        ISTSettlementVault2.Pending memory next = vault.pending(NO1);
        assertEq(next.seq, 1);
        assertEq(next.root, commitment.root);
        assertEq(next.total, F_TOTAL);
        assertEq(next.leafCount, 5);
        assertEq(next.epoch, epoch);
        assertEq(next.ledgerHash, commitment.ledgerHash);
        assertEq(next.committedBlock, block.number);
        assertEq(next.openDemands, 0);
        assertEq(vault.commitSeq(NO1), 1);
        assertEq(vault.lastCommitBlock(NO1), block.number);
        assertEq(vault.rootCount(NO1), 0);
    }

    // V-7
    function test_V7_commitRefusesAnUnknownPool() public {
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), 100, 1);
        vm.expectRevert(ISTSettlementVault2.UnknownPool.selector);
        vault.commitPending(FIRST_EPOCH, NO2, 0, commitment);
    }

    // V-7
    function test_V7_commitRefusesWithinTheWindowOfThePreviousCommit() public {
        _publishF();
        V2TreeHelper.Node[][] memory p = _honestP();
        _propose(NO1, p, 6);
        uint256 committed = block.number;
        ISTSettlementVault2.Commitment memory commitment = _commitment(p, 6);

        vm.roll(committed + WINDOW - 1);
        vm.expectRevert(ISTSettlementVault2.CommitTooSoon.selector);
        vault.commitPending(FIRST_EPOCH + 1, NO1, 1, commitment);

        vm.roll(committed + WINDOW);
        assertEq(vault.commitPending(FIRST_EPOCH + 1, NO1, 1, commitment), 3);
    }

    // V-7: an operator that proves a fault in its own root cannot re-roll the
    // demand sample by committing again at once.
    function test_V7_commitRefusesWithinTheWindowAfterASelfProvenFault() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        _propose(NO1, _loweredP(), 6);
        uint256 committed = block.number;
        _checkCarol(f, _loweredP());
        assertEq(vault.pending(NO1).seq, 0);
        ISTSettlementVault2.Commitment memory commitment = _commitment(_honestP(), 6);

        vm.roll(committed + WINDOW - 1);
        vm.expectRevert(ISTSettlementVault2.CommitTooSoon.selector);
        vault.commitPending(FIRST_EPOCH + 1, NO1, 1, commitment);

        vm.roll(committed + WINDOW);
        vault.commitPending(FIRST_EPOCH + 1, NO1, 1, commitment);
    }

    // V-7
    function test_V7_commitRefusesAStaleRootCount() public {
        _capture(NO1, F_TOTAL);
        ISTSettlementVault2.Commitment memory commitment =
            _commitment(V2TreeHelper.build(_oldKeys(), _oldValues()), 5);
        vm.expectRevert(ISTSettlementVault2.StaleRootCount.selector);
        vault.commitPending(FIRST_EPOCH, NO1, 1, commitment);
    }

    // V-7: a finalize between build and commit leaves the build's predecessor stale.
    function test_V7_commitRefusesARootCountFromBeforeAFinalization() public {
        _publishF();
        _capture(NO1, P_TOTAL - F_TOTAL);
        ISTSettlementVault2.Commitment memory commitment = _commitment(_honestP(), 6);
        vm.expectRevert(ISTSettlementVault2.StaleRootCount.selector);
        vault.commitPending(FIRST_EPOCH + 1, NO1, 0, commitment);
    }

    // V-7
    function test_V7_commitRefusesTheEpochBeforeThePoolsFirst() public {
        _capture(NO1, F_TOTAL);
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), 0, 1);
        vm.expectRevert(ISTSettlementVault2.EpochOutOfRange.selector);
        vault.commitPending(FIRST_EPOCH - 1, NO1, 0, commitment);
    }

    // V-7
    function test_V7_commitRefusesAnUnfundedEpoch() public {
        _capture(NO1, F_TOTAL);
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), F_TOTAL, 1);
        vm.expectRevert(ISTSettlementVault2.EpochOutOfRange.selector);
        vault.commitPending(FIRST_EPOCH + 1, NO1, 0, commitment);
    }

    // V-7
    function test_V7_commitRefusesAnEpochNotAfterTheFinalizedRoot() public {
        _publishF();
        _capture(NO1, P_TOTAL - F_TOTAL);
        ISTSettlementVault2.Commitment memory commitment = _commitment(_honestP(), 6);
        vm.expectRevert(ISTSettlementVault2.EpochOutOfRange.selector);
        vault.commitPending(FIRST_EPOCH, NO1, 1, commitment);
    }

    // V-7
    function test_V7_commitRefusesATotalOtherThanTheEpochsSnapshot() public {
        _capture(NO1, 100);
        _capture(NO1, 50);
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), 100, 1);
        vm.expectRevert(ISTSettlementVault2.TotalMismatch.selector);
        vault.commitPending(FIRST_EPOCH + 1, NO1, 0, commitment);
    }

    // V-7
    function test_V7_commitRefusesATotalNotAboveTheFinalizedTotal() public {
        _publishF();
        uint256 epoch = _defer(NO1);
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), F_TOTAL, 5);
        vm.expectRevert(ISTSettlementVault2.TotalNotIncreased.selector);
        vault.commitPending(epoch, NO1, 1, commitment);
    }

    // V-7
    function test_V7_commitRefusesAnUnfundedFirstRoot() public {
        _capture(NO1, 0);
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), 0, 1);
        vm.expectRevert(ISTSettlementVault2.TotalNotIncreased.selector);
        vault.commitPending(FIRST_EPOCH, NO1, 0, commitment);
    }

    // V-7
    function test_V7_commitRefusesZeroLeaves() public {
        _capture(NO1, 100);
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), 100, 0);
        vm.expectRevert(ISTSettlementVault2.InvalidLeafCount.selector);
        vault.commitPending(FIRST_EPOCH, NO1, 0, commitment);
    }

    // V-7
    function test_V7_commitRefusesMoreThan2To29Leaves() public {
        _capture(NO1, 100);
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), 100, (1 << 29) + 1);
        vm.expectRevert(ISTSettlementVault2.InvalidLeafCount.selector);
        vault.commitPending(FIRST_EPOCH, NO1, 0, commitment);
    }

    // V-7
    function test_V7_commitAccepts2To29Leaves() public {
        _capture(NO1, 100);
        vault.commitPending(FIRST_EPOCH, NO1, 0, _raw(keccak256("root"), 100, 1 << 29));
        assertEq(vault.pending(NO1).leafCount, 1 << 29);
    }

    // V-7
    function test_V7_commitRefusesFewerLeavesThanTheFinalizedRoot() public {
        _publishF();
        _capture(NO1, P_TOTAL - F_TOTAL);
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), P_TOTAL, 4);
        vm.expectRevert(ISTSettlementVault2.InvalidLeafCount.selector);
        vault.commitPending(FIRST_EPOCH + 1, NO1, 1, commitment);
    }

    // V-7
    function test_V7_commitRefusesAZeroRoot() public {
        _capture(NO1, 100);
        ISTSettlementVault2.Commitment memory commitment = _raw(bytes32(0), 100, 1);
        vm.expectRevert(ISTSettlementVault2.InvalidRoot.selector);
        vault.commitPending(FIRST_EPOCH, NO1, 0, commitment);
    }

    // V-7: an epoch that does not fit the root's uint64 field is refused, not
    // truncated. Only a pool registered at the last uint64 epoch reaches one.
    function test_V7_commitRefusesAnEpochBeyondUint64() public {
        neuron.setUid(NETUID, POOL2, POOL2_UID);
        vault.registerPool(NO2, POOL2, BURN_LIMIT, type(uint64).max);
        _capture(NO2, 100);
        uint256 epoch = _capture(NO2, 50);
        assertEq(epoch, uint256(type(uint64).max) + 1);
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), 150, 1);
        vm.expectRevert(abi.encodeWithSelector(SafeCast.SafeCastOverflowedUintDowncast.selector, 64, epoch));
        vault.commitPending(epoch, NO2, 0, commitment);
    }

    // V-7: likewise a block number beyond uint64.
    function test_V7_commitRefusesABlockNumberBeyondUint64() public {
        _capture(NO1, 100);
        ISTSettlementVault2.Commitment memory commitment = _raw(keccak256("root"), 100, 1);
        uint256 blockNumber = uint256(type(uint64).max) + 1;
        vm.roll(blockNumber);
        vm.expectRevert(
            abi.encodeWithSelector(SafeCast.SafeCastOverflowedUintDowncast.selector, 64, blockNumber)
        );
        vault.commitPending(FIRST_EPOCH, NO1, 0, commitment);
    }

    // V-7, V-EV: a replacement takes the next seq, names the seq it replaced,
    // and starts with no demand; the replaced root's demands no longer block.
    function test_V7_aReplacementBumpsSeqAndDiscardsDemands() public {
        _publishF();
        uint64 replaced = _propose(NO1, _honestP(), 6);
        _demand(CAROL_INDEX);
        assertEq(vault.pending(NO1).openDemands, 1);

        vm.roll(block.number + WINDOW);
        uint256 epoch = _defer(NO1);
        V2TreeHelper.Node[][] memory p = _honestP();
        ISTSettlementVault2.Commitment memory commitment = _commitment(p, 6);
        vm.expectEmit(address(vault));
        emit ISTSettlementVault2.DistributionCommitted(
            NO1, replaced + 1, uint64(epoch), commitment.root, P_TOTAL, 6, commitment.ledgerHash, replaced
        );
        uint64 seq = vault.commitPending(epoch, NO1, 1, commitment);

        assertEq(seq, replaced + 1);
        assertEq(vault.pending(NO1).openDemands, 0);
        assertFalse(vault.demanded(NO1, seq, CAROL_INDEX));
        _finalize(NO1);
        assertEq(vault.rootCount(NO1), 2);
    }

    // V-7: seq never repeats, across finalization, deletion and replacement.
    function test_V7_seqAfterAFinalizationOrDeletionExceedsEveryEarlierSeq() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        uint64 first = vault.commitSeq(NO1);
        assertEq(first, 1);

        uint64 deleted = _propose(NO1, _loweredP(), 6);
        _checkCarol(f, _loweredP());
        assertGt(deleted, first);

        vm.roll(block.number + WINDOW);
        uint256 epoch = _defer(NO1);
        ISTSettlementVault2.Commitment memory commitment = _commitment(_honestP(), 6);
        vm.expectEmit(address(vault));
        emit ISTSettlementVault2.DistributionCommitted(
            NO1, deleted + 1, uint64(epoch), commitment.root, P_TOTAL, 6, commitment.ledgerHash, 0
        );
        uint64 next = vault.commitPending(epoch, NO1, 1, commitment);
        assertGt(next, deleted);
        assertEq(vault.commitSeq(NO1), next);
    }

    // ------------------------------------------------------------------
    // V-8: checkPath deletes the pending root for each fault
    // ------------------------------------------------------------------

    // V-8
    function test_V8_aLoweredAccountDeletesThePendingRoot() public {
        _assertCarolDeletes(_publishF(), _loweredP(), LedgerTree.Fault.LowerValue, 0);
    }

    // V-8
    function test_V8_aMovedColdkeyDeletesThePendingRoot() public {
        _assertCarolDeletes(_publishF(), _movedP(), LedgerTree.Fault.WrongLeaf, 0);
    }

    // V-8
    function test_V8_aGarbageLeafDeletesThePendingRoot() public {
        _assertCarolDeletes(_publishF(), _garbageLeafP(), LedgerTree.Fault.WrongLeaf, 0);
    }

    // V-8
    function test_V8_anInconsistentNodeAtLevel1DeletesThePendingRoot() public {
        _assertCarolDeletes(_publishF(), _inconsistentP(1), LedgerTree.Fault.InconsistentNode, 1);
    }

    // V-8
    function test_V8_anInconsistentNodeAtLevel2DeletesThePendingRoot() public {
        _assertCarolDeletes(_publishF(), _inconsistentP(2), LedgerTree.Fault.InconsistentNode, 2);
    }

    // V-8: the root's children do not add up to the committed total.
    function test_V8_anInconsistentRootDeletesThePendingRoot() public {
        _assertCarolDeletes(_publishF(), _inconsistentP(3), LedgerTree.Fault.InconsistentNode, 3);
    }

    // V-8: a fault deletes the root whatever demands are open on it.
    function test_V8_aFaultDeletesThePendingRootDespiteOpenDemands() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        _propose(NO1, _loweredP(), 6);
        _demand(0);
        _demand(CAROL_INDEX);
        assertEq(uint8(_checkCarol(f, _loweredP())), uint8(LedgerTree.Fault.LowerValue));
        assertEq(vault.pending(NO1).seq, 0);
        assertEq(vault.pending(NO1).openDemands, 0);
        vm.roll(block.number + WINDOW);
        vm.expectRevert(ISTSettlementVault2.NoPendingRoot.selector);
        vault.finalizePending(NO1);
    }

    // V-8: at depth 0 the leaf is (root, total). A lower value cannot occur
    // there: both roots hold one account and the new total must exceed the old.
    function test_V8_aMovedColdkeyAtDepthZeroDeletesThePendingRoot() public {
        V2TreeHelper.Node[][] memory f = _publish(NO1, _keys(ALICE), _values(100));
        V2TreeHelper.Node[][] memory p = V2TreeHelper.build(_keys(MALLORY), _values(150));
        uint64 seq = _propose(NO1, p, 1);
        LedgerTree.Account memory alice = _account(0, ALICE, 100);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, 0);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(p, 0);
        assertEq(oldProof.length, 0);
        assertEq(newPath.length, 0);

        vm.expectEmit(address(vault));
        emit ISTSettlementVault2.DistributionDeleted(
            NO1, seq, 0, LedgerTree.Fault.WrongLeaf, 0, address(this)
        );
        assertEq(uint8(vault.checkPath(NO1, alice, oldProof, newPath)), uint8(LedgerTree.Fault.WrongLeaf));
        assertEq(vault.pending(NO1).seq, 0);
    }

    // V-8
    function test_V8_aGarbageRootAtDepthZeroDeletesThePendingRoot() public {
        V2TreeHelper.Node[][] memory f = _publish(NO1, _keys(ALICE), _values(100));
        V2TreeHelper.Node[][] memory p = V2TreeHelper.build(_keys(ALICE), _values(150));
        p[0][0].hash = keccak256("garbage-root");
        uint64 seq = _propose(NO1, p, 1);
        LedgerTree.Account memory alice = _account(0, ALICE, 100);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, 0);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(p, 0);

        vm.expectEmit(address(vault));
        emit ISTSettlementVault2.DistributionDeleted(
            NO1, seq, 0, LedgerTree.Fault.WrongLeaf, 0, address(this)
        );
        vault.checkPath(NO1, alice, oldProof, newPath);
        assertEq(vault.pending(NO1).seq, 0);
    }

    // ------------------------------------------------------------------
    // V-9: checkPath refusals
    // ------------------------------------------------------------------

    // V-9: no index of an honest successor shows a fault, at any shape. The
    // successor keeps every account in place, never lowers one and appends
    // new ones.
    function testFuzz_V9_checkPathFindsNoFaultOnAnHonestIndex(uint256 seed, uint8 oldCount, uint8 added)
        public
    {
        uint256 n = bound(oldCount, 1, 17);
        uint256 m = n + bound(added, 0, 17);
        bytes32[] memory keys = new bytes32[](m);
        bytes32[] memory oldKeys = new bytes32[](n);
        uint256[] memory oldValues = new uint256[](n);
        uint256[] memory newValues = new uint256[](m);
        for (uint256 i; i < m; i++) {
            uint256 draw = uint256(keccak256(abi.encode(seed, i)));
            keys[i] = keccak256(abi.encode("fuzz-coldkey", seed, i));
            newValues[i] = 1 + draw % 1e12;
            if (i < n) {
                oldKeys[i] = keys[i];
                oldValues[i] = 1 + (draw >> 64) % 1e12;
                newValues[i] = oldValues[i] + (draw >> 128) % 1e9;
            }
        }
        newValues[0] += 1;
        V2TreeHelper.Node[][] memory f = _publish(NO1, oldKeys, oldValues);
        V2TreeHelper.Node[][] memory p = V2TreeHelper.build(keys, newValues);
        _propose(NO1, p, m);

        for (uint256 index; index < n; index++) {
            LedgerTree.Account memory account = _account(index, keys[index], oldValues[index]);
            LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, index);
            LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(p, index);
            vm.expectRevert(ISTSettlementVault2.NoFault.selector);
            vault.checkPath(NO1, account, oldProof, newPath);
        }
        assertGt(vault.pending(NO1).seq, 0);
    }

    // V-9
    function test_V9_checkPathRefusesAPreimageThatDoesNotHashToItsNode() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        _propose(NO1, _loweredP(), 6);
        LedgerTree.Account memory carol = _account(CAROL_INDEX, CAROL, 30);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, CAROL_INDEX);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(_loweredP(), CAROL_INDEX);
        newPath[1].leftSum += 1;
        vm.expectRevert(LedgerTree.MalformedPath.selector);
        vault.checkPath(NO1, carol, oldProof, newPath);
    }

    // V-9
    function test_V9_checkPathRefusesAPathOfTheWrongLength() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        _propose(NO1, _loweredP(), 6);
        LedgerTree.Account memory carol = _account(CAROL_INDEX, CAROL, 30);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, CAROL_INDEX);
        LedgerTree.PathNode[] memory full = V2TreeHelper.path(_loweredP(), CAROL_INDEX);
        LedgerTree.PathNode[] memory short = new LedgerTree.PathNode[](2);
        (short[0], short[1]) = (full[0], full[1]);
        vm.expectRevert(LedgerTree.MalformedPath.selector);
        vault.checkPath(NO1, carol, oldProof, short);
    }

    // V-9
    function test_V9_checkPathRefusesAnInvalidOldProof() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        _propose(NO1, _loweredP(), 6);
        LedgerTree.Account memory inflated = _account(CAROL_INDEX, CAROL, 31);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, CAROL_INDEX);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(_loweredP(), CAROL_INDEX);
        vm.expectRevert(ISTSettlementVault2.InvalidProof.selector);
        vault.checkPath(NO1, inflated, oldProof, newPath);
    }

    // V-9: an account the finalized root does not hold has nothing to protect.
    function test_V9_checkPathRefusesAnIndexOutsideTheFinalizedRoot() public {
        _publishF();
        V2TreeHelper.Node[][] memory p = _honestP();
        _propose(NO1, p, 6);
        LedgerTree.Account memory frank = _account(5, FRANK, 30);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(p, 5);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(p, 5);
        vm.expectRevert(ISTSettlementVault2.IndexOutOfRange.selector);
        vault.checkPath(NO1, frank, oldProof, newPath);
    }

    // V-9
    function test_V9_checkPathRefusesWithNoPendingRoot() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        LedgerTree.Account memory carol = _account(CAROL_INDEX, CAROL, 30);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, CAROL_INDEX);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(f, CAROL_INDEX);
        vm.expectRevert(ISTSettlementVault2.NoPendingRoot.selector);
        vault.checkPath(NO1, carol, oldProof, newPath);
    }

    // V-9: a pool's first root has no finalized predecessor to protect.
    function test_V9_checkPathRefusesWithNoFinalizedRoot() public {
        V2TreeHelper.Node[][] memory f = V2TreeHelper.build(_oldKeys(), _oldValues());
        _propose(NO1, f, 5);
        LedgerTree.Account memory carol = _account(CAROL_INDEX, CAROL, 30);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, CAROL_INDEX);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(f, CAROL_INDEX);
        vm.expectRevert(ISTSettlementVault2.NoFinalizedRoot.selector);
        vault.checkPath(NO1, carol, oldProof, newPath);
    }

    // ------------------------------------------------------------------
    // V-10: demandPath
    // ------------------------------------------------------------------

    // V-10, V-EV
    function test_V10_aDemandIsRecordedAndItsFeeKept() public {
        _publishF();
        uint64 seq = _propose(NO1, _honestP(), 6);
        vm.deal(stranger, DEMAND_FEE);

        vm.expectEmit(address(vault));
        emit ISTSettlementVault2.PathDemanded(NO1, seq, CAROL_INDEX, stranger);
        vm.prank(stranger);
        vault.demandPath{value: DEMAND_FEE}(NO1, CAROL_INDEX);

        assertTrue(vault.demanded(NO1, seq, CAROL_INDEX));
        assertEq(vault.pending(NO1).openDemands, 1);
        assertEq(address(vault).balance, DEMAND_FEE);
        assertEq(stranger.balance, 0);
    }

    // V-10, V-EV: a clean opening answers the demand, and anyone may give it.
    function test_V10_aCleanOpeningAnswersTheDemand() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        V2TreeHelper.Node[][] memory p = _honestP();
        uint64 seq = _propose(NO1, p, 6);
        _demand(CAROL_INDEX);
        _demand(0);
        LedgerTree.Account memory carol = _account(CAROL_INDEX, CAROL, 30);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, CAROL_INDEX);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(p, CAROL_INDEX);
        address answerer = makeAddr("answerer");

        vm.expectEmit(address(vault));
        emit ISTSettlementVault2.PathAnswered(NO1, seq, CAROL_INDEX, answerer);
        vm.prank(answerer);
        assertEq(uint8(vault.checkPath(NO1, carol, oldProof, newPath)), uint8(LedgerTree.Fault.None));

        assertFalse(vault.demanded(NO1, seq, CAROL_INDEX));
        assertTrue(vault.demanded(NO1, seq, 0));
        assertEq(vault.pending(NO1).openDemands, 1);
        assertEq(vault.pending(NO1).seq, seq);
    }

    // V-10
    function test_V10_demandRefusesTheSameIndexTwicePerSeq() public {
        _publishF();
        _propose(NO1, _honestP(), 6);
        _demand(CAROL_INDEX);
        vm.deal(stranger, DEMAND_FEE);
        vm.prank(stranger);
        vm.expectRevert(ISTSettlementVault2.AlreadyDemanded.selector);
        vault.demandPath{value: DEMAND_FEE}(NO1, CAROL_INDEX);
    }

    // V-10: once answered, an index cannot be demanded again under the same seq.
    function test_V10_demandRefusesAnAnsweredIndexUnderTheSameSeq() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        V2TreeHelper.Node[][] memory p = _honestP();
        _propose(NO1, p, 6);
        _demand(CAROL_INDEX);
        _checkCarol(f, p);
        vm.deal(stranger, DEMAND_FEE);
        vm.prank(stranger);
        vm.expectRevert(ISTSettlementVault2.AlreadyDemanded.selector);
        vault.demandPath{value: DEMAND_FEE}(NO1, CAROL_INDEX);
    }

    // V-10: an opening with no fault and no open demand changes nothing.
    function test_V10_checkPathRefusesACleanOpeningWithNoOpenDemand() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        V2TreeHelper.Node[][] memory p = _honestP();
        _propose(NO1, p, 6);
        LedgerTree.Account memory carol = _account(CAROL_INDEX, CAROL, 30);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, CAROL_INDEX);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(p, CAROL_INDEX);
        vm.expectRevert(ISTSettlementVault2.NoFault.selector);
        vault.checkPath(NO1, carol, oldProof, newPath);
    }

    // V-10
    function test_V10_demandRefusesAfterTheWindow() public {
        _publishF();
        _propose(NO1, _honestP(), 6);
        vm.roll(block.number + WINDOW);
        vm.deal(stranger, DEMAND_FEE);
        vm.prank(stranger);
        vm.expectRevert(ISTSettlementVault2.ChallengeWindowClosed.selector);
        vault.demandPath{value: DEMAND_FEE}(NO1, CAROL_INDEX);
    }

    // V-10, V-11: a demand in the window's last block blocks finalization
    // until it is answered after the window.
    function test_V10_aDemandInTheWindowsLastBlockIsAnsweredAfterIt() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        V2TreeHelper.Node[][] memory p = _honestP();
        _propose(NO1, p, 6);
        uint256 committed = block.number;

        vm.roll(committed + WINDOW - 1);
        _demand(CAROL_INDEX);
        vm.roll(committed + WINDOW);
        vm.expectRevert(ISTSettlementVault2.DemandOpen.selector);
        vault.finalizePending(NO1);

        _checkCarol(f, p);
        assertEq(vault.finalizePending(NO1), 1);
    }

    // V-10: a withheld or garbage node can never be opened, so a demand under
    // it can never be answered or turned into a fault, and it blocks the root.
    function test_V10_anUnanswerableDemandBlocksFinalization() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        V2TreeHelper.Node[][] memory p = _withheldNodeP();
        _propose(NO1, p, 6);
        _demand(CAROL_INDEX);
        _demand(0);
        _check(f, p, 0, ALICE, 10);

        LedgerTree.Account memory carol = _account(CAROL_INDEX, CAROL, 30);
        LedgerTree.ProofNode[] memory oldProof = V2TreeHelper.proof(f, CAROL_INDEX);
        LedgerTree.PathNode[] memory newPath = V2TreeHelper.path(p, CAROL_INDEX);
        vm.expectRevert(LedgerTree.MalformedPath.selector);
        vault.checkPath(NO1, carol, oldProof, newPath);

        vm.roll(block.number + WINDOW);
        vm.expectRevert(ISTSettlementVault2.DemandOpen.selector);
        vault.finalizePending(NO1);
    }

    // V-10: an unpublished honest tree blocks until its operator opens the path.
    function test_V10_aWithheldTreeBlocksFinalizationUntilAnswered() public {
        V2TreeHelper.Node[][] memory f = _publishF();
        V2TreeHelper.Node[][] memory p = _honestP();
        _propose(NO1, p, 6);
        _demand(CAROL_INDEX);
        vm.roll(block.number + WINDOW * 3);
        vm.expectRevert(ISTSettlementVault2.DemandOpen.selector);
        vault.finalizePending(NO1);
        _checkCarol(f, p);
        vault.finalizePending(NO1);
    }

    // V-10
    function test_V10_demandRefusesAnIndexOutsideTheFinalizedRoot() public {
        _publishF();
        _propose(NO1, _honestP(), 6);
        vm.deal(stranger, DEMAND_FEE);
        vm.prank(stranger);
        vm.expectRevert(ISTSettlementVault2.IndexOutOfRange.selector);
        vault.demandPath{value: DEMAND_FEE}(NO1, 5);
    }

    // V-10
    function test_V10_demandRefusesWithNoPendingRoot() public {
        _publishF();
        vm.deal(stranger, DEMAND_FEE);
        vm.prank(stranger);
        vm.expectRevert(ISTSettlementVault2.NoPendingRoot.selector);
        vault.demandPath{value: DEMAND_FEE}(NO1, 0);
    }

    // V-10
    function test_V10_demandRefusesWithNoFinalizedRoot() public {
        _propose(NO1, V2TreeHelper.build(_oldKeys(), _oldValues()), 5);
        vm.deal(stranger, DEMAND_FEE);
        vm.prank(stranger);
        vm.expectRevert(ISTSettlementVault2.NoFinalizedRoot.selector);
        vault.demandPath{value: DEMAND_FEE}(NO1, 0);
    }

    // V-10
    function test_V10_demandRefusesALowerFee() public {
        _publishF();
        _propose(NO1, _honestP(), 6);
        vm.deal(stranger, DEMAND_FEE);
        vm.prank(stranger);
        vm.expectRevert(ISTSettlementVault2.InvalidDemandFee.selector);
        vault.demandPath{value: DEMAND_FEE - 1}(NO1, 0);
    }

    // V-10
    function test_V10_demandRefusesAHigherFee() public {
        _publishF();
        _propose(NO1, _honestP(), 6);
        vm.deal(stranger, DEMAND_FEE + 1);
        vm.prank(stranger);
        vm.expectRevert(ISTSettlementVault2.InvalidDemandFee.selector);
        vault.demandPath{value: DEMAND_FEE + 1}(NO1, 0);
    }

    // V-10
    function test_V10_demandRefusesNoFee() public {
        _publishF();
        _propose(NO1, _honestP(), 6);
        vm.prank(stranger);
        vm.expectRevert(ISTSettlementVault2.InvalidDemandFee.selector);
        vault.demandPath(NO1, 0);
    }

    // ------------------------------------------------------------------
    // V-11: finalizePending
    // ------------------------------------------------------------------

    // V-11, V-EV
    function test_V11_finalizeAppendsTheRootAndClearsTheSlot() public {
        _publishF();
        V2TreeHelper.Node[][] memory p = _honestP();
        uint256 epoch = _capture(NO1, P_TOTAL - F_TOTAL);
        ISTSettlementVault2.Commitment memory commitment = _commitment(p, 6);
        uint64 seq = vault.commitPending(epoch, NO1, 1, commitment);
        vm.roll(block.number + WINDOW);

        vm.expectEmit(address(vault));
        emit ISTSettlementVault2.DistributionFinalized(
            NO1, seq, 1, uint64(epoch), commitment.root, P_TOTAL, 6, commitment.ledgerHash
        );
        assertEq(vault.finalizePending(NO1), 1);

        ISTSettlementVault2.Root memory root = vault.roots(NO1, 1);
        assertEq(root.root, commitment.root);
        assertEq(root.total, P_TOTAL);
        assertEq(root.leafCount, 6);
        assertEq(root.epoch, epoch);
        assertEq(root.ledgerHash, commitment.ledgerHash);
        assertEq(vault.rootCount(NO1), 2);
        assertEq(vault.finalizedTotal(NO1), P_TOTAL);
        assertEq(vault.roots(NO1, 0).total, F_TOTAL, "every finalized root is kept");

        ISTSettlementVault2.Pending memory cleared = vault.pending(NO1);
        assertEq(cleared.seq, 0);
        assertEq(cleared.root, bytes32(0));
        assertEq(cleared.total, 0);
        assertEq(cleared.leafCount, 0);
        assertEq(cleared.epoch, 0);
        assertEq(cleared.ledgerHash, bytes32(0));
        assertEq(cleared.committedBlock, 0);
        assertEq(cleared.openDemands, 0);
    }

    // V-11
    function test_V11_finalizeRefusesWithNoPendingRoot() public {
        _publishF();
        vm.expectRevert(ISTSettlementVault2.NoPendingRoot.selector);
        vault.finalizePending(NO1);
    }

    // V-11
    function test_V11_finalizeRefusesInsideTheWindow() public {
        _publishF();
        _propose(NO1, _honestP(), 6);
        vm.roll(block.number + WINDOW - 1);
        vm.expectRevert(ISTSettlementVault2.ChallengeWindowOpen.selector);
        vault.finalizePending(NO1);
    }

    // V-11
    function test_V11_finalizeRefusesWithAnOpenDemand() public {
        _publishF();
        _propose(NO1, _honestP(), 6);
        _demand(CAROL_INDEX);
        vm.roll(block.number + WINDOW);
        vm.expectRevert(ISTSettlementVault2.DemandOpen.selector);
        vault.finalizePending(NO1);
    }

    // V-11: the finalized total of a pool with no root is zero.
    function test_V11_aPoolWithNoRootHasNoFinalizedTotal() public view {
        assertEq(vault.finalizedTotal(NO1), 0);
        assertEq(vault.rootCount(NO1), 0);
    }
}
