//go:build linux || darwin

package main

import (
	"context"
	"errors"
	"testing"
)

// The observer reports the actual WaitThrough cancellation subscription;
// it changes no context behavior and needs no scheduling or time assumption.
type evidenceRelayWaitObservedContext struct {
	context.Context
	armed chan struct{}
}

func (self *evidenceRelayWaitObservedContext) Done() <-chan struct{} {
	self.armed <- struct{}{}
	return self.Context.Done()
}

func TestEvidenceRelayWaitThroughRequiresEveryConfiguredSource(t *testing.T) {
	worker := &evidenceRelayRuntime{sources: make([]evidenceRelaySource, 2), done: make(chan struct{}), changed: make(chan struct{}), through: map[uint64]uint64{}, completed: map[uint64]bool{1: false, 2: false}}
	ctx, cancel := context.WithCancel(t.Context())
	observed := &evidenceRelayWaitObservedContext{Context: ctx, armed: make(chan struct{}, 4)}
	result, joined := make(chan error, 1), make(chan struct{})
	go func() { defer close(joined); result <- worker.WaitThrough(observed, 7) }()
	defer func() { cancel(); <-joined }()
	select {
	case err := <-result:
		t.Fatalf("unpublished sources returned: %v", err)
	case <-observed.armed:
	}
	func() {
		worker.stateLock.Lock()
		defer worker.stateLock.Unlock()
		worker.through[1], worker.completed[1] = 7, true
		close(worker.changed)
		worker.changed = make(chan struct{})
	}()
	select {
	case err := <-result:
		t.Fatalf("only one source satisfied the complete census: %v", err)
	case <-observed.armed:
	}
	func() {
		worker.stateLock.Lock()
		defer worker.stateLock.Unlock()
		worker.through[2], worker.completed[2] = 7, true
		close(worker.changed)
		worker.changed = make(chan struct{})
	}()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceRelayWaitThroughDistinguishesEpochZeroFromMissing(t *testing.T) {
	worker := &evidenceRelayRuntime{sources: make([]evidenceRelaySource, 1), done: make(chan struct{}), changed: make(chan struct{}), through: map[uint64]uint64{1: 0}, completed: map[uint64]bool{1: false}}
	close(worker.done)
	if err := worker.WaitThrough(t.Context(), 0); err == nil {
		t.Fatal("unpublished epoch zero counted as completion")
	}
	worker.completed[1] = true
	if err := worker.WaitThrough(t.Context(), 0); err != nil {
		t.Fatal("genuine completed epoch zero was discarded", err)
	}
}

func TestEvidenceRelayWaitThroughRetainsFailureAfterCompletedProgress(t *testing.T) {
	failure := errors.New("canonical publication changed after progress")
	worker := &evidenceRelayRuntime{sources: make([]evidenceRelaySource, 1), done: make(chan struct{}), changed: make(chan struct{}), through: map[uint64]uint64{1: 9}, completed: map[uint64]bool{1: true}, resultErr: failure}
	if err := worker.WaitThrough(t.Context(), 9); !errors.Is(err, failure) {
		t.Fatal("completed progress hid a subsequent real failure", err)
	}
	worker.resultErr = nil
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := worker.WaitThrough(ctx, 9); !errors.Is(err, context.Canceled) {
		t.Fatal("completed progress ignored caller cancellation", err)
	}
}
