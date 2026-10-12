// A history replay that only ran out of time stays a retryable read wait, no
// matter how many object reads it joined. Every other leaf keeps its verdict.
package validator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// A replay-shaped graph: nested wrappers around joins of object reads that
// each hit the deadline, deeper than the classifier's 32 levels.
func productionSteeringDeadlineTestGraph(depth int, leaf func(int) error) error {
	reads := make([]error, 0, 8)
	for index := 0; index < 8; index++ {
		reads = append(reads, &url.Error{Op: "Get", URL: fmt.Sprintf("https://reader:secret@api.example/sn/attempt-artifact?hash=0x%02x&kind=metadata", index), Err: leaf(index)})
	}
	err := errors.Join(reads...)
	for level := 0; level < depth; level++ {
		err = errors.Join(fmt.Errorf("V2 intent %d: %w", level, err), context.DeadlineExceeded)
	}
	return err
}

func TestProductionSteeringDeadlineOnlyGraphStaysRetryable(t *testing.T) {
	t.Parallel()
	deadline := func(int) error { return context.DeadlineExceeded }
	deep := productionSteeringDeadlineTestGraph(40, deadline)
	if retryableProductionSteeringRead(deep) {
		t.Fatal("test graph is not beyond the classifier's bounds")
	}
	collapsed := collapseProductionSteeringDeadline(deep, false)
	compact, ok := collapsed.(*productionSteeringReadDeadline)
	if !ok || !retryableProductionSteeringRead(collapsed) || compact.leaves != 48 {
		t.Fatalf("deadline-only graph did not collapse to one retryable cause: %T %v", collapsed, collapsed)
	}
	if strings.Contains(compact.Error(), "secret") || !strings.Contains(compact.Error(), "api.example/sn/attempt-artifact") {
		t.Fatalf("sample leaked credentials or lost the read: %s", compact.Error())
	}
	observed := observeReleaseError(collapsed)
	if !retryableProductionSteeringRead(observed) || !strings.Contains(observed.Error(), "ran out of time") {
		t.Fatalf("observation lost the collapsed verdict: %v", observed)
	}
	// Cancellation counts only when the bounded operation itself expired.
	canceled := productionSteeringDeadlineTestGraph(40, func(index int) error {
		if index%2 == 0 {
			return context.Canceled
		}
		return context.DeadlineExceeded
	})
	if collapsed := collapseProductionSteeringDeadline(canceled, false); collapsed != canceled {
		t.Fatal("unexpired cancellation collapsed into a deadline")
	}
	if collapsed := collapseProductionSteeringDeadline(canceled, true); !retryableProductionSteeringRead(collapsed) {
		t.Fatal("cancellation caused by the expired operation stayed hard")
	}
	onlyCanceled := errors.Join(context.Canceled, context.Canceled)
	if collapsed := collapseProductionSteeringDeadline(onlyCanceled, true); collapsed != onlyCanceled {
		t.Fatal("a graph without any deadline collapsed")
	}
	// A small retryable graph keeps its own cause.
	small := &url.Error{Op: "Get", URL: "https://api.example/x", Err: context.DeadlineExceeded}
	if collapsed := collapseProductionSteeringDeadline(small, false); collapsed != small {
		t.Fatal("an already retryable cause was replaced")
	}
}

// Any refusal, file, native, foreign or unknown leaf keeps the whole graph hard.
func TestProductionSteeringDeadlineCollapseKeepsHardCauses(t *testing.T) {
	t.Parallel()
	for name, hard := range map[string]error{
		"refusal":       &releaseObservedRefusal{original: errors.New("x")},
		"observed hard": &releaseObservedHard{cause: context.DeadlineExceeded},
		"file":          &os.PathError{Op: "open", Path: "/state", Err: os.ErrNotExist},
		"native":        &releaseObservedNativeRead{original: context.DeadlineExceeded, retryable: true},
		"foreign":       &productionSteeringCauseMatcher{cause: context.DeadlineExceeded},
		"contradiction": errors.New("V2 intent history bytes or finite census differ"),
		"nil child":     &productionSteeringCauseNode{children: []error{context.DeadlineExceeded, nil}},
		"too wide":      &productionSteeringCauseNode{children: make([]error, 129)},
		"empty join":    &productionSteeringCauseNode{},
	} {
		graph := productionSteeringDeadlineTestGraph(40, func(index int) error {
			if index == 5 {
				return hard
			}
			return context.DeadlineExceeded
		})
		collapsed := collapseProductionSteeringDeadline(graph, true)
		if collapsed != graph || retryableProductionSteeringRead(collapsed) {
			t.Fatalf("%s leaf was hidden by the deadline collapse", name)
		}
	}
	cycle := &productionSteeringCauseNode{}
	cycle.children = []error{cycle}
	if collapsed := collapseProductionSteeringDeadline(cycle, true); collapsed != cycle {
		t.Fatal("a cyclic graph collapsed")
	}
}

// The actual read owner retries and then waits on a deadline-only replay
// instead of returning it as a hard failure.
func TestProductionSteeringReadKeepsDeepDeadlineRetryable(t *testing.T) {
	t.Parallel()
	self := &ReleaseSteerer{cfg: &ReleaseConfig{SchemaVersion: ReleaseMainnetProductionSchemaVersion}}
	reads, waits := 0, 0
	self.productionReadHooks.wait = func(context.Context, time.Duration) error {
		waits++
		return context.DeadlineExceeded
	}
	err := self.productionRead(t.Context(), productionReadIntent, nil, func(context.Context) error {
		reads++
		return productionSteeringDeadlineTestGraph(40, func(int) error { return context.DeadlineExceeded })
	})
	var wait *productionSteeringReadWait
	if !errors.As(err, &wait) || wait.phase != productionReadIntent || reads != 1 || waits != 1 {
		t.Fatalf("deep deadline replay became a hard read failure: reads=%d waits=%d err=%v", reads, waits, err)
	}
	// A mixed graph is still returned hard without a retry.
	reads, waits = 0, 0
	err = self.productionRead(t.Context(), productionReadIntent, nil, func(context.Context) error {
		reads++
		return productionSteeringDeadlineTestGraph(40, func(index int) error {
			if index == 3 {
				return errors.New("V2 prepared vector differs from the independently replayed decision")
			}
			return context.DeadlineExceeded
		})
	})
	if err == nil || errors.As(err, &wait) || reads != 1 || waits != 0 {
		t.Fatalf("mixed replay gained a retry: reads=%d waits=%d err=%v", reads, waits, err)
	}
}
