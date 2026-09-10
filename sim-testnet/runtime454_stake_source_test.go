package main

// The native stake reader consumes more than the runtime version and storage
// schema: its selective SCALE layout, weighted calculation, integer view and
// registered-owner exception must remain in the exact upstream source gate.

import (
	"os"
	"strings"
	"testing"
)

// Independent exact digests bind every newly consumed runtime source file.
// This guard is selected by the existing TestRuntime454 producer prefix; the
// existing full source guard still requires all prior paths and artifacts.
func TestRuntime454ValidatorStakeSourcePins(t *testing.T) {
	raw, err := os.ReadFile("../docs/spec/runtime-v454-source.sha256")
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		"pallets/subtensor/runtime-api/src/lib.rs":    "d7cc457db24ffc052c8cb4bf28103cabd2c129490ea5fe55dc6b7df307ef7a11",
		"pallets/subtensor/src/rpc_info/metagraph.rs": "59d593074bebe2957eaebe1bb21bee8f937405162c6be698459fafb0063a3161",
		"pallets/subtensor/src/subnets/weights.rs":    "11e81b703f3a65fe308a3cd1aeaf9e71e84519e26793b5e29a28993daa11526a",
		"pallets/subtensor/src/utils/misc.rs":         "a40fc6cc1bb38ed07f0650df7af352152d6ac9941c48b6062133a93c42bd2ca6",
		"pallets/subtensor/src/epoch/math.rs":         "a445d0da2e66654cefb16965d3b0cef712f6414c8b8c594a658fd4c17eb698c2",
	}
	seen := make(map[string]bool)
	for _, row := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		fields := strings.Fields(row)
		if len(fields) != 2 || seen[fields[1]] {
			t.Fatalf("malformed or duplicate runtime source row %q", row)
		}
		seen[fields[1]] = true
		if digest, required := expected[fields[1]]; required && fields[0] != digest {
			t.Errorf("native stake source %s has digest %s, want %s", fields[1], fields[0], digest)
		}
	}
	for path := range expected {
		if !seen[path] {
			t.Errorf("native stake runtime source is not attested: %s", path)
		}
	}
	checker, err := os.ReadFile("../scripts/check-runtime-v454-source.sh")
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 29 || !strings.Contains(string(checker), "\nexpected_files=29\n") {
		t.Fatal("source checker does not admit the exact expanded 29-file manifest")
	}
}
