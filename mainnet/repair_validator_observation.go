// Observation failures retain their cause; only a completed contradictory read
// establishes custody loss. Private instance hooks can fail, never admit a read.
package main

import (
	"context"
	"errors"
	"os"
	"syscall"

	"github.com/urnetwork/connect/durablevolume"
)

// A deterministic after-syscall barrier cannot supply bytes or replace an error.
// No command, policy or environment option installs this instance-local hook.
type repairValidatorObservationKey struct{}

// Both the caller's cancellation and the actual observation error survive.
func repairValidatorObservation(ctx context.Context, operation string, cause error) error {
	if ctx == nil {
		return errors.Join(errors.New("validator repair observation context is absent"), cause)
	}
	if after, ok := ctx.Value(repairValidatorObservationKey{}).(func(string) error); ok && after != nil {
		cause = errors.Join(cause, after(operation))
	}
	return errors.Join(cause, ctx.Err())
}

// Missing retained names are evidence of loss. Failed descriptor observations,
// including a closed caller-owned descriptor, cannot establish a replacement.
func repairValidatorObservationError(message string, cause error, retainedName bool) error {
	if cause == nil {
		return nil
	}
	if errors.Is(cause, durablevolume.ErrIdentity) || retainedName && (errors.Is(cause, os.ErrNotExist) || errors.Is(cause, syscall.ENOTDIR) || errors.Is(cause, syscall.ELOOP)) {
		return errors.Join(durablevolume.ErrIdentity, errors.New(message), cause)
	}
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) || errors.Is(cause, os.ErrClosed) || errors.Is(cause, syscall.EBADF) {
		return errors.Join(errors.New(message), cause)
	}
	return mainnetDurableUnavailable(message, cause)
}
