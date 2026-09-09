package main

// A role account owns one ordered transaction at a time. Waiting for that
// turn is cancellable and never holds the small manager state lock across RPC.

import (
	"context"
	"errors"
)

// Lazy initialization also covers managers constructed directly by focused
// tests. The returned release belongs to this caller and is deferred before
// any operation that can fail, panic or exit its goroutine.
func (self *EvmTxManager) acquireNonceTurn(ctx context.Context) (func(), error) {
	if self == nil || ctx == nil {
		return nil, errors.New("EVM account turn owner or context is absent")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	turn := func() chan struct{} {
		self.stateLock.Lock()
		defer self.stateLock.Unlock()
		if self.nonceTurn == nil {
			self.nonceTurn = make(chan struct{}, 1)
		}
		return self.nonceTurn
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case turn <- struct{}{}:
	}
	if err := ctx.Err(); err != nil {
		<-turn
		return nil, err
	}
	return func() { <-turn }, nil
}
