package validator

import (
	"net"
	"os"
	"reflect"
	"syscall"
)

// Parallel operators can wait for different parts of the same preparation.
// Every cause must be an exact cut marker or typed read interruption. A marker
// cannot hide an independent integrity, storage, cancellation or fatal error.
// The transport result keeps pure cut waits out of pre-intent read authority.
func classifyReleasePreparationRetry(err error) (retryable, transport bool) {
	remaining := 512
	return classifyReleasePreparationRetryBounded(err, 0, &remaining)
}

// A rejected transport tree must not be reopened through an unbounded cut
// search. The whole joined preparation shares one finite traversal allowance.
func classifyReleasePreparationRetryBounded(err error, depth int, remaining *int) (retryable, transport bool) {
	if err == nil || depth > 32 || *remaining <= 0 {
		return false, false
	}
	*remaining--
	value := reflect.ValueOf(err)
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		if value.IsNil() {
			return false, false
		}
	}
	switch err {
	case errAttemptCutPending, errAttemptCutSnapshotStale, errAttemptSettlementSnapshotStale:
		return true, false
	}
	switch cause := err.(type) {
	case *TrailFatalError, *os.PathError, *os.LinkError:
		return false, false
	case *artifactUnavailable:
		return classifyReleasePreparationRetryBounded(cause.cause, depth+1, remaining)
	case *net.DNSError:
		// A completed resolver negative cannot be reopened as a cut wait
		// through its child after the transport owner rejects it.
		return classifyReleaseSnapshotRetryBounded(cause, false, false, false, depth, remaining)
	case syscall.Errno:
		return classifyReleaseSnapshotRetryBounded(err, false, false, false, depth, remaining)
	case interface{ Is(error) bool }, interface{ As(any) bool }:
		return false, false
	}
	// Transport inspection spends the same allowance as the cut fallback.
	// A rejected subtree cannot restart hundreds of independent walks.
	if retryable, transient := classifyReleaseSnapshotRetryBounded(err, false, false, false, depth, remaining); retryable && transient {
		return true, true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		if len(causes) == 0 || len(causes) > 128 || len(causes) > *remaining {
			return false, false
		}
		for _, cause := range causes {
			childRetryable, childTransport := classifyReleasePreparationRetryBounded(cause, depth+1, remaining)
			if !childRetryable {
				return false, false
			}
			transport = transport || childTransport
		}
		return true, transport
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return classifyReleasePreparationRetryBounded(wrapped.Unwrap(), depth+1, remaining)
	}
	return false, false
}
