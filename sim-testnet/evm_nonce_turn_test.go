package main

// Explicit account-turn and context barriers cover cancellation and lifecycle
// ordering without sleeps, negative timeouts or scheduler-dependent polling.

import (
	"context"
	"errors"
	"math/big"
	"runtime"
	"sync"
	"testing"
)

// Signals when the real sender has armed its cancellable wait. Err remains
// the underlying context's authority, rather than an injected test verdict.
type evmNonceTurnObservedContext struct {
	context.Context
	once    sync.Once
	entered chan struct{}
}

// The observer changes no cancellation semantics and fires at most once.
func (self *evmNonceTurnObservedContext) Done() <-chan struct{} {
	self.once.Do(func() { close(self.entered) })
	return self.Context.Done()
}

// With the account already owned, Send must wait before touching the nonce,
// journal or signer. Original Send dereferences its journal instead of waiting.
func TestEvmTxManagerSendCancellationDoesNotEnterBusyAccount(t *testing.T) {
	manager := &EvmTxManager{}
	release, err := manager.acquireNonceTurn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	observed := &evmNonceTurnObservedContext{Context: ctx, entered: make(chan struct{})}
	type sendResult struct {
		err        error
		panicValue any
	}
	joined := make(chan sendResult, 1)
	go func() {
		result := sendResult{}
		defer func() { result.panicValue = recover(); joined <- result }()
		_, result.err = manager.Send(observed, "unused-plan", Action{}, nil, new(big.Int), nil)
	}()
	select {
	case result := <-joined:
		t.Fatalf("sender entered a busy account before cancellation: error=%v panic=%v", result.err, result.panicValue)
	case <-observed.entered:
	}
	cancel()
	result := <-joined
	if result.panicValue != nil || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("busy sender did not join cancellation before account access: error=%v panic=%v", result.err, result.panicValue)
	}
}

// Different funded roles do not share a turn; canceling a waiting caller on
// one role cannot take away the active owner's turn or strand the next caller.
func TestEvmTxManagerNonceTurnsRemainAccountLocal(t *testing.T) {
	first, second := &EvmTxManager{}, &EvmTxManager{}
	releaseFirst, err := first.acquireNonceTurn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	releaseSecond, err := second.acquireNonceTurn(t.Context())
	if err != nil {
		releaseFirst()
		t.Fatal(err)
	}
	releaseSecond()
	releaseFirst()
	for _, manager := range []*EvmTxManager{first, second} {
		release, err := manager.acquireNonceTurn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
}

// The account owner releases even when its goroutine exits without returning;
// the next caller needs no retry, timeout or process restart to acquire it.
func TestEvmTxManagerNonceTurnReleasesAfterGoexit(t *testing.T) {
	manager := &EvmTxManager{}
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		release, err := manager.acquireNonceTurn(t.Context())
		if err != nil {
			t.Error(err)
			return
		}
		defer release()
		runtime.Goexit()
	}()
	<-joined
	release, err := manager.acquireNonceTurn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	release()
}

// Invalid or already-canceled ownership fails before initializing account
// state, including when a free turn would otherwise be selectable.
func TestEvmTxManagerNonceTurnRejectsMissingAndCanceledContext(t *testing.T) {
	manager := &EvmTxManager{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, input := range []context.Context{nil, ctx} {
		release, err := manager.acquireNonceTurn(input)
		if release != nil || err == nil || manager.nonceTurn != nil {
			t.Fatal("invalid context acquired or initialized an account turn")
		}
	}
	var missing *EvmTxManager
	if release, err := missing.acquireNonceTurn(t.Context()); release != nil || err == nil {
		t.Fatal("missing account owner acquired a turn")
	}
}
