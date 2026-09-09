//go:build linux || darwin

package main

import (
	"context"
	"errors"
	"testing"
)

func TestEvidenceRelayAuditDrainRequiresFreshPassAfterHappyPath(t *testing.T) {
	worker := &evidenceRelayRuntime{done: make(chan struct{}), changed: make(chan struct{}), startedAuditPasses: 9, completedAuditPasses: 8}
	ctx, cancel := context.WithCancel(t.Context())
	observed := &evidenceRelayWaitObservedContext{Context: ctx, armed: make(chan struct{}, 4)}
	result, joined := make(chan error, 1), make(chan struct{})
	go func() { defer close(joined); result <- worker.WaitAuditPass(observed) }()
	defer func() { cancel(); <-joined }()
	select {
	case err := <-result:
		t.Fatalf("cached audit pass completed a new drain request: %v", err)
	case <-observed.armed:
	}
	// Pass nine was already in flight when happy-path completion requested
	// this drain; its earlier discovery snapshot cannot satisfy the request.
	func() {
		worker.stateLock.Lock()
		defer worker.stateLock.Unlock()
		worker.completedAuditPasses = 9
		close(worker.changed)
		worker.changed = make(chan struct{})
	}()
	select {
	case err := <-result:
		t.Fatalf("already-started discovery satisfied a fresh drain: %v", err)
	case <-observed.armed:
	}
	func() {
		worker.stateLock.Lock()
		defer worker.stateLock.Unlock()
		worker.startedAuditPasses, worker.completedAuditPasses = 10, 10
		close(worker.changed)
		worker.changed = make(chan struct{})
	}()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceRelayAuditDrainRejectsStoppedAndExhaustedOwner(t *testing.T) {
	worker := &evidenceRelayRuntime{done: make(chan struct{}), changed: make(chan struct{}), startedAuditPasses: 8, completedAuditPasses: 8}
	close(worker.done)
	if err := worker.WaitAuditPass(t.Context()); err == nil {
		t.Fatal("stopped worker completed an unobserved final audit pass")
	}
	worker.startedAuditPasses = ^uint64(0)
	if err := worker.WaitAuditPass(t.Context()); err == nil {
		t.Fatal("audit pass overflow wrapped into an already completed round")
	}
	failure := errors.New("audit publication disappeared")
	worker.startedAuditPasses, worker.completedAuditPasses, worker.resultErr = 8, 8, failure
	if err := worker.WaitAuditPass(t.Context()); !errors.Is(err, failure) {
		t.Fatal("real audit failure disappeared during drain", err)
	}
}
