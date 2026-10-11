// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {STSettlement2} from "../../src/STSettlement2.sol";
import {ISTSettlementVault2} from "../../src/interfaces/ISTSettlementVault2.sol";

/// @dev The phase C coordinator upgrade's surplus wrapper (VERSION2-FINAL.md
///      §16 step 5), for E-7's sweep on phase-A bytecode: the owner assigns
///      escrow stake that vault2 accounts to no pool, such as the absorbed
///      v1 sweep, to a pool. It adds no storage to STSettlement2.
contract STSettlement2SurplusUpgrade is STSettlement2 {
    function assignSurplus(uint256 noId, uint256 amount) external onlyOwner {
        ISTSettlementVault2(vault).assignSurplus(noId, amount);
    }
}
