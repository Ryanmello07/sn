package merkle

import (
	"context"
	"testing"

	"github.com/holiman/uint256"
)

func ledgerTestAccounts(n int) []LedgerAccount {
	accounts := make([]LedgerAccount, n)
	for i := range accounts {
		accounts[i].Coldkey[0] = byte(i + 1)
		accounts[i].Coldkey[31] = byte(i >> 8)
		accounts[i].Cumulative.SetUint64(uint64(1_000 + 7*i))
	}
	return accounts
}

// GT-2, GT-4 (smoke): every index of an honest tree proves, and its path
// shows no fault against itself.
func TestLedgerSumTreeRoundTrip(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 5, 8, 9, 33} {
		accounts := ledgerTestAccounts(n)
		tree, err := BuildLedgerSumTree(context.Background(), accounts)
		if err != nil {
			t.Fatal(err)
		}
		root, total := tree.Root()
		var want uint256.Int
		for i := range accounts {
			want.Add(&want, &accounts[i].Cumulative)
		}
		if !total.Eq(&want) {
			t.Fatalf("n=%d total %s, want %s", n, total.Dec(), want.Dec())
		}
		for i := range accounts {
			proof, err := tree.Proof(uint64(i))
			if err != nil {
				t.Fatal(err)
			}
			if len(proof) != tree.Depth() {
				t.Fatalf("n=%d proof length %d, depth %d", n, len(proof), tree.Depth())
			}
			if !VerifyLedgerProof(root, &total, uint64(n), uint64(i), accounts[i].Coldkey, &accounts[i].Cumulative, proof) {
				t.Fatalf("n=%d index %d does not verify", n, i)
			}
			path, err := tree.Path(uint64(i))
			if err != nil {
				t.Fatal(err)
			}
			fault, level, err := WalkLedgerPath(root, &total, uint64(n), uint64(i), accounts[i].Coldkey, &accounts[i].Cumulative, path)
			if err != nil || fault != LedgerFaultNone || level != 0 {
				t.Fatalf("n=%d index %d: fault %v level %d err %v", n, i, fault, level, err)
			}
		}
	}
}
