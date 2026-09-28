// Receipt scan results retain the exact finalized coverage boundary. A missing
// receipt without completed coverage is unknown, never authenticated absence.
package crv4

import "github.com/centrifuge/go-substrate-rpc-client/v4/types"

// Only the complete-body scanner constructs this witness. Exported accessors
// copy values; callers cannot turn a newer head into old-prefix coverage.
type FinalizedExtrinsicScan struct {
	from          uint64
	finalizedHash types.Hash
	finalizedAt   uint64
	absent        bool
	receipt       *FinalizedExtrinsic
}

// Membership still needs exact-runtime dispatch and source-event validation.
func (self *FinalizedExtrinsicScan) Receipt() *FinalizedExtrinsic {
	if self == nil || self.receipt == nil {
		return nil
	}
	copy := *self.receipt
	return &copy
}

// The range is inclusive. False means that no negative conclusion exists,
// including a prepared block later than the currently finalized head.
func (self *FinalizedExtrinsicScan) AbsenceBoundary() (uint64, types.Hash, bool) {
	if self == nil || !self.absent || self.receipt != nil || self.from > self.finalizedAt {
		return 0, types.Hash{}, false
	}
	return self.finalizedAt, self.finalizedHash, true
}
