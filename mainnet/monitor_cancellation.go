// A parent cancellation can race the retained checkpoint load after successful
// admission. It joins that owner without reporting an invented custody loss.
package main

import (
	"context"
	"errors"

	"github.com/urnetwork/connect/durablevolume"
)

func monitorCanceledCheckpointLoad(ctx context.Context, err error) bool {
	if ctx.Err() == nil || !errors.Is(err, ctx.Err()) {
		return false
	}
	var ownership *monitorOutputOwnershipError
	var cleanup *monitorAdmissionCleanupError
	return !errors.Is(err, durablevolume.ErrIdentity) && !errors.Is(err, errRpcIdentityMismatch) && !errors.Is(err, errRpcIntegrity) && !errors.As(err, &ownership) && !errors.As(err, &cleanup) && monitorOnlyCancellationCause(err, ctx.Err(), 0)
}

// Joined independent I/O/close failures are not canceled observations. Owned
// display wrappers can preserve the exact sentinel without discarding siblings.
func monitorOnlyCancellationCause(err, cancellation error, depth int) bool {
	if err == cancellation {
		return true
	}
	if err == nil || depth >= 32 {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !monitorOnlyCancellationCause(cause, cancellation, depth+1) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return monitorOnlyCancellationCause(wrapped.Unwrap(), cancellation, depth+1)
	}
	return false
}
