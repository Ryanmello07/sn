package ledger

import (
	"errors"
	"fmt"
	"io"
	"math/bits"
	"slices"

	"github.com/holiman/uint256"

	"github.com/urfoundation/sn/merkle"
)

// DemandSample is the size of the §9.3 value-weighted sample of the
// previous root's accounts before any doubling.
const DemandSample = 256

// ErrDemandIndex is returned by DemandSet for a fault or an earlier
// unanswered index that is not an index of the previous root.
var ErrDemandIndex = errors.New("ledger: demand index is not an account of the previous root")

// DemandSet returns, in ascending order, the indexes of the previous root's
// accounts to demand on a pending root whose objects are missing or do not
// reproduce it (§9.3, the residual risk of §4.2 G2):
//
//   - every fault: an index whose published row is below the account's
//     value, names another coldkey or is missing (Result.Faults);
//   - every index in again: the unanswered demands of the earlier pending
//     roots over the same previous root that were replaced or deleted while
//     a demand was open;
//   - a sample of DemandSample << doublings of the remaining accounts,
//     where doublings counts those earlier roots, drawn by value without
//     replacement. When no more remaining accounts have a positive value,
//     the sample takes them all; an account of zero value holds nothing to
//     protect and is never drawn.
//
// The sample reads its randomness from random, which must be a CSPRNG read
// after the pending root was committed (crypto/rand.Reader), so that the
// operator cannot predict it. The result depends only on the input and the
// bytes read. A theft of a fraction f of the previous root's value escapes
// m such draws with probability at most (1−f)^m.
func DemandSet(previous []merkle.LedgerAccount, faults []Fault, doublings uint, again []uint64, random io.Reader) ([]uint64, error) {
	n := uint64(len(previous))
	demanded := make([]bool, n)
	var set []uint64
	for _, i := range slices.Concat(faultIndexes(faults), again) {
		if i >= n {
			return nil, fmt.Errorf("%w: %d of %d", ErrDemandIndex, i, n)
		}
		if !demanded[i] {
			demanded[i] = true
			set = append(set, i)
		}
	}
	size := n
	if doublings < 32 {
		size = min(n, DemandSample<<doublings)
	}
	weights := newWeightTree(previous, demanded)
	for drawn := uint64(0); drawn < size && !weights.total.IsZero(); drawn++ {
		r, err := uniform(random, &weights.total)
		if err != nil {
			return nil, fmt.Errorf("ledger: demand sample randomness: %w", err)
		}
		i := weights.find(r)
		weights.remove(i, &previous[i].Cumulative)
		set = append(set, uint64(i))
	}
	slices.Sort(set)
	return set, nil
}

func faultIndexes(faults []Fault) []uint64 {
	indexes := make([]uint64, len(faults))
	for k := range faults {
		indexes[k] = faults[k].Index
	}
	return indexes
}

// weightTree is a Fenwick tree over account values, for drawing accounts
// by value without replacement.
type weightTree struct {
	// node[k], 1-based, sums the values of positions k − lowbit(k) … k − 1.
	node  []uint256.Int
	total uint256.Int
}

// newWeightTree holds every account's value except the excluded ones'.
func newWeightTree(accounts []merkle.LedgerAccount, excluded []bool) *weightTree {
	t := &weightTree{node: make([]uint256.Int, len(accounts)+1)}
	for i := range accounts {
		k := i + 1
		if !excluded[i] {
			t.node[k].Add(&t.node[k], &accounts[i].Cumulative)
			t.total.Add(&t.total, &accounts[i].Cumulative)
		}
		if parent := k + k&-k; parent < len(t.node) {
			t.node[parent].Add(&t.node[parent], &t.node[k])
		}
	}
	return t
}

// find returns the position whose value interval holds r, for r below the
// total: the first position whose prefix sum exceeds r.
func (t *weightTree) find(r uint256.Int) int {
	position := 0
	for step := 1 << (bits.Len(uint(len(t.node)-1)) - 1); step > 0; step >>= 1 {
		if next := position + step; next < len(t.node) && !t.node[next].Gt(&r) {
			position = next
			r.Sub(&r, &t.node[next])
		}
	}
	return position
}

// remove takes value off position i.
func (t *weightTree) remove(i int, value *uint256.Int) {
	for k := i + 1; k < len(t.node); k += k & -k {
		t.node[k].Sub(&t.node[k], value)
	}
	t.total.Sub(&t.total, value)
}

// uniform returns a uniform value below a positive bound, by rejection.
func uniform(random io.Reader, bound *uint256.Int) (uint256.Int, error) {
	n := bound.BitLen()
	buf := make([]byte, (n+7)/8)
	for {
		var r uint256.Int
		if _, err := io.ReadFull(random, buf); err != nil {
			return r, err
		}
		buf[0] &= 0xff >> (8*len(buf) - n)
		r.SetBytes(buf)
		if r.Lt(bound) {
			return r, nil
		}
	}
}
