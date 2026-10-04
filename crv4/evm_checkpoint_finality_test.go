// Native/EVM first insertion needs current finality as well as unchanged
// canonical hashes. These controls preserve the exact historical query.
package crv4

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// Only the closing finalized response changes after both authenticated map
// reads. A valid advancement retains the selected native100/EVM70 mapping.
func TestEvmCheckpointClosesOriginalFinalizedWitness(t *testing.T) {
	for _, fault := range []string{"unchanged", "advanced", "regressed", "missing head", "changed closing hash", "head timeout", "header timeout"} {
		fixture, query, _ := newEVMCheckpointTestFixture(t)
		opening := fixture.finalized
		advancedHeader, advanced := receiptTestHeader(t, opening, 104, nil, 1)
		fixture.headers[advanced.Hex()], fixture.blockHashes[104] = advancedHeader, advanced
		originalHook := fixture.hook
		finalizedReads := 0
		fixture.hook = func(ctx context.Context, result any, method string, args ...any) (bool, error) {
			if method == "chain_getFinalizedHead" {
				finalizedReads++
				if finalizedReads == 2 {
					if len(fixture.storageCalls) != 2 {
						t.Fatalf("%s finality closure preceded map reads: %v", fault, fixture.storageCalls)
					}
					switch fault {
					case "advanced", "changed closing hash", "header timeout":
						return true, setRuntimeIdentityTestResult(result, advanced.Hex())
					case "regressed":
						return true, setRuntimeIdentityTestResult(result, query.NativeHash.Hex())
					case "missing head":
						return true, setRuntimeIdentityTestResult(result, "")
					case "head timeout":
						return true, context.DeadlineExceeded
					}
				}
			}
			if finalizedReads == 2 && fault == "header timeout" && method == "chain_getHeader" {
				return true, context.DeadlineExceeded
			}
			if finalizedReads == 2 && fault == "changed closing hash" && method == "chain_getBlockHash" && args[0] == uint64(104) {
				return true, setRuntimeIdentityTestResult(result, types.Hash{99}.Hex())
			}
			return originalHook(ctx, result, method, args...)
		}
		observed, err := ReadEVMCheckpointAtContext(fixture.ctx, fixture.chain, query, fixture.allowed...)
		wantSuccess := fault == "unchanged" || fault == "advanced"
		if (err == nil) != wantSuccess {
			t.Fatalf("%s finality success=%t finalized reads=%d observation=%+v error=%v", fault, wantSuccess, finalizedReads, observed, err)
		}
		if wantSuccess && observed.Query != query || !wantSuccess && observed != (EVMCheckpointObservation{}) {
			t.Fatalf("%s replaced the original query or published partial proof: %+v", fault, observed)
		}
		if strings.HasSuffix(fault, "timeout") && (!errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "regressed") || strings.Contains(err.Error(), "not canonical")) {
			t.Fatalf("%s became finality contradiction: %v", fault, err)
		}
		if fixture.chain.Runtime.SpecVersion != 999 {
			t.Fatalf("%s changed dial-time signing metadata", fault)
		}
	}
}
