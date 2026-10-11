package ledger

import (
	"bytes"
	"errors"
	"io"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/holiman/uint256"

	"github.com/urfoundation/sn/merkle"
)

// seeded returns a deterministic byte stream standing in for crypto/rand.
func seeded(seed byte) io.Reader {
	return rand.NewChaCha8([32]byte{seed})
}

// valuedAccounts returns accounts with the given values, coldkey i+1 at i.
func valuedAccounts(values ...uint64) []merkle.LedgerAccount {
	accounts := make([]merkle.LedgerAccount, len(values))
	for i, v := range values {
		accounts[i] = merkle.LedgerAccount{Coldkey: key(i + 1), Cumulative: rao(v)}
	}
	return accounts
}

// equalAccounts returns n accounts of value 1,000.
func equalAccounts(n int) []merkle.LedgerAccount {
	values := make([]uint64, n)
	for i := range values {
		values[i] = 1_000
	}
	return valuedAccounts(values...)
}

func mustDemandSet(t *testing.T, previous []merkle.LedgerAccount, faults []Fault, doublings uint, again []uint64, random io.Reader) []uint64 {
	t.Helper()
	set, err := DemandSet(previous, faults, doublings, again, random)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.IsSorted(set) || len(slices.Compact(slices.Clone(set))) != len(set) {
		t.Fatalf("demand set is not ascending and distinct: %v", set)
	}
	for _, i := range set {
		if i >= uint64(len(previous)) {
			t.Fatalf("demand set holds index %d of %d accounts", i, len(previous))
		}
	}
	return set
}

// VA-4: the sample starts at 256 accounts, and is never larger than the
// account count: a pool of 64 or 256 accounts is demanded whole.
func TestDemandSetFloor(t *testing.T) {
	for _, c := range []struct{ n, want int }{{1, 1}, {64, 64}, {255, 255}, {256, 256}, {257, 256}, {1_000, 256}} {
		if set := mustDemandSet(t, equalAccounts(c.n), nil, 0, nil, seeded(1)); len(set) != c.want {
			t.Fatalf("%d accounts: sample of %d, want %d", c.n, len(set), c.want)
		}
	}
	if set := mustDemandSet(t, nil, nil, 3, nil, seeded(1)); len(set) != 0 {
		t.Fatalf("no previous accounts: %v", set)
	}
}

// VA-4, E-4: the sample doubles for each earlier failed root over the same
// finalized root, up to every account.
func TestDemandSetDoubling(t *testing.T) {
	previous := equalAccounts(3_000)
	for doublings, want := range map[uint]int{0: 256, 1: 512, 2: 1_024, 3: 2_048, 4: 3_000, 31: 3_000, 32: 3_000, 200: 3_000} {
		if set := mustDemandSet(t, previous, nil, doublings, nil, seeded(2)); len(set) != want {
			t.Fatalf("%d doublings: sample of %d, want %d", doublings, len(set), want)
		}
	}
}

// VA-4: the sample is weighted by value: of 256 heavy and 768 light
// accounts, a 256-account sample is all heavy, where a uniform one would be
// about a quarter heavy; of 256 heavy accounts and one light one, the light
// one is left out.
func TestDemandSetWeighting(t *testing.T) {
	values := make([]uint64, 1_024)
	for i := range values {
		values[i] = 1
		if i%4 == 0 {
			values[i] = 1_000_000_000
		}
	}
	for seed := range byte(20) {
		heavy := 0
		for _, i := range mustDemandSet(t, valuedAccounts(values...), nil, 0, nil, seeded(seed)) {
			if values[i] > 1 {
				heavy++
			}
		}
		if heavy != 256 {
			t.Fatalf("seed %d: %d of 256 drawn accounts are heavy", seed, heavy)
		}
	}
	values = values[:257]
	for i := range values {
		values[i] = 1_000_000_000
	}
	values[100] = 1
	for seed := range byte(20) {
		if set := mustDemandSet(t, valuedAccounts(values...), nil, 0, nil, seeded(seed)); slices.Contains(set, 100) {
			t.Fatalf("seed %d: the light account was drawn before a heavy one", seed)
		}
	}
}

// VA-4: draws follow value exactly: position i is drawn for a uniform value
// in [prefix(i), prefix(i+1)), a zero value is never drawn, and a drawn
// account's value leaves the tree.
func TestDemandSetDrawArithmetic(t *testing.T) {
	tree := newWeightTree(valuedAccounts(3, 0, 5, 1), []bool{false, false, false, false})
	for r, want := range []int{0, 0, 0, 2, 2, 2, 2, 2, 3} {
		if got := tree.find(rao(uint64(r))); got != want {
			t.Fatalf("value %d draws position %d, want %d", r, got, want)
		}
	}
	value := rao(5)
	tree.remove(2, &value)
	for r, want := range []int{0, 0, 0, 3} {
		if got := tree.find(rao(uint64(r))); got != want || tree.total != rao(4) {
			t.Fatalf("after removal, value %d draws position %d of total %v, want %d", r, got, tree.total.Dec(), want)
		}
	}
	excluded := newWeightTree(valuedAccounts(3, 0, 5, 1), []bool{true, false, false, true})
	if excluded.total != rao(5) || excluded.find(rao(0)) != 2 || excluded.find(rao(4)) != 2 {
		t.Fatalf("excluded accounts are drawn: total %v", excluded.total.Dec())
	}
	// A bound of 5 masks one byte to three bits and rejects 7 and 5.
	bound := rao(5)
	if r, err := uniform(bytes.NewReader([]byte{0xff, 0xfd, 0x0b, 0x00}), &bound); err != nil || r != rao(3) {
		t.Fatalf("uniform below 5 = %v, %v; want 3", r.Dec(), err)
	}
	// A bound of 2^156 + 1 masks the first of 20 bytes to five bits.
	wide := new(uint256.Int).Lsh(uint256.NewInt(1), 156)
	wide.AddUint64(wide, 1)
	if r, err := uniform(bytes.NewReader(append([]byte{0xf0}, make([]byte, 19)...)), wide); err != nil || r.BitLen() != 157 || r.Eq(wide) {
		t.Fatalf("uniform below 2^156 + 1 = %v, %v", r.Dec(), err)
	}
}

// VA-4: accounts whose published rows show a fault, and the unanswered
// indexes of earlier failed roots, are always demanded, once, besides the
// sample of the other accounts; zero-value accounts are demanded only then.
func TestDemandSetForced(t *testing.T) {
	previous := equalAccounts(1_000)
	previous[9].Cumulative.Clear()
	faults := []Fault{{3, merkle.LedgerFaultLowerValue}, {9, merkle.LedgerFaultWrongLeaf}}
	set := mustDemandSet(t, previous, faults, 0, []uint64{12, 9, 12}, seeded(3))
	if len(set) != 3+256 {
		t.Fatalf("forced and sampled: %d demands", len(set))
	}
	for _, i := range []uint64{3, 9, 12} {
		if !slices.Contains(set, i) {
			t.Fatalf("forced index %d is not demanded: %v", i, set)
		}
	}
	// Zero-value accounts are never sampled, so a pool of 100 valued and 500
	// empty accounts is demanded exactly at its valued accounts and faults.
	values := make([]uint64, 600)
	var want []uint64
	for i := range 100 {
		values[i*6] = uint64(i + 1)
		want = append(want, uint64(i*6))
	}
	set = mustDemandSet(t, valuedAccounts(values...), []Fault{{1, merkle.LedgerFaultWrongLeaf}}, 5, nil, seeded(4))
	want = append(want, 1)
	slices.Sort(want)
	if !slices.Equal(set, want) {
		t.Fatalf("zero-value pool: %v, want %v", set, want)
	}
}

// VA-4: the set depends only on the input and the random bytes.
func TestDemandSetDeterministic(t *testing.T) {
	previous := equalAccounts(5_000)
	a := mustDemandSet(t, previous, nil, 1, []uint64{4_999}, seeded(5))
	b := mustDemandSet(t, previous, nil, 1, []uint64{4_999}, seeded(5))
	c := mustDemandSet(t, previous, nil, 1, []uint64{4_999}, seeded(6))
	if !slices.Equal(a, b) || slices.Equal(a, c) || len(a) != 513 || len(c) != 513 {
		t.Fatalf("same seed %v vs %v; other seed equal %v", len(a), len(b), slices.Equal(a, c))
	}
}

// VA-4: an index outside the previous root, and a failed random source,
// are refused.
func TestDemandSetErrors(t *testing.T) {
	previous := equalAccounts(300)
	if _, err := DemandSet(previous, []Fault{{300, merkle.LedgerFaultWrongLeaf}}, 0, nil, seeded(7)); !errors.Is(err, ErrDemandIndex) {
		t.Fatalf("fault index 300 of 300: %v", err)
	}
	if _, err := DemandSet(previous, nil, 0, []uint64{1, 1 << 40}, seeded(7)); !errors.Is(err, ErrDemandIndex) {
		t.Fatalf("again index 2^40 of 300: %v", err)
	}
	failing := io.MultiReader(io.LimitReader(seeded(7), 64), failingReader{bytes.NewReader(nil)})
	if _, err := DemandSet(previous, nil, 0, nil, failing); err == nil || err.Error() != "ledger: demand sample randomness: connection reset" {
		t.Fatalf("failed random source: %v", err)
	}
	if _, err := DemandSet(previous, nil, 0, nil, bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Fatalf("empty random source: %v", err)
	}
}
