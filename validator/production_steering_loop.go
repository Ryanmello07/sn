// Production polls the actual retained-intent owner before fresh scheduler
// state. Independent workers survive read outages without forgetting defects.
package validator

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// The only injected decisions are the existing poll and read operation. The
// caller passes SubmitOnce itself; no callback grants receipt or signing proof.
func runReleaseProductionSteeringLoopWithWait(ctx context.Context, submit func() error, wait func() bool, progress *releaseProgress) error {
	if ctx == nil || submit == nil || wait == nil {
		return errors.New("production steering loop owner is incomplete")
	}
	var pendingErr error
	failures := 0
	for {
		if err := ctx.Err(); err != nil {
			return releaseRuntimeError(ctx, errors.Join(pendingErr, err))
		}
		err := submit()
		if ctx.Err() != nil {
			return releaseRuntimeError(ctx, errors.Join(pendingErr, err, ctx.Err()))
		}
		var readWait *productionSteeringReadWait
		var originalPending *productionPendingReconciliation
		var transition *productionSteeringTransition
		switch {
		case err == nil || releaseOnlyErrors(err, ErrSteeringAlreadyFinal):
			failures, pendingErr = 0, nil
			progress.observeSteering(0, false, "complete", true)
		case errors.As(err, &readWait) && readWait.phase >= productionReadIntent && readWait.phase <= productionReadApplication && retryableProductionSteeringRead(readWait.cause) && releaseOnlyErrors(err, readWait):
			outcome := "read_wait"
			if readWait.phase == productionReadReceipt {
				outcome = "receipt_transport_wait"
			}
			progress.observeSteering(readWait.nativeEpoch, readWait.epochKnown, outcome, false)
			fmt.Printf("release steer: %v; retaining progress for the next poll\n", readWait)
		case errors.As(err, &originalPending) && (originalPending.cause == nil || retryableProductionSteeringRead(originalPending.cause)) && releaseOnlyErrors(err, originalPending):
			progress.observeSteering(originalPending.nativeEpoch, true, "receipt_pending", originalPending.cause == nil)
			fmt.Printf("release steer: %v; no new signature while the original outcome is unknown\n", originalPending)
		case errors.As(err, &transition) && releaseOnlyErrors(err, transition):
			outcome := "working"
			if transition.revealWait {
				outcome = "reveal_wait"
			}
			progress.observeSteering(transition.nativeEpoch, true, outcome, true)
			if !transition.revealWait {
				failures, pendingErr = 0, nil
			}
		default:
			failures++
			pendingErr = errors.Join(pendingErr, err)
			progress.observeSteering(0, false, "hard_error", false)
			fmt.Printf("release steer: production hard failure %d: %v\n", failures, err)
		}
		if failures >= releaseSteeringFailureLimit {
			return fmt.Errorf("release steering failed %d consecutive attempts: %w", failures, pendingErr)
		}
		if !wait() {
			return releaseRuntimeError(ctx, pendingErr)
		}
	}
}

// The deadline covers preparation as well as bounded retained reads. A poll
// never consults fresh signing runtime before the durable intent owner does.
func (self *ReleaseSteerer) runProductionSteering(ctx context.Context, poll, timeout time.Duration) error {
	if ctx == nil || poll <= 0 || timeout < productionSteeringReadTimeout {
		return errors.New("production steering timing owner is invalid")
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	return runReleaseProductionSteeringLoopWithWait(ctx, func() error {
		return runReleaseSteeringOperation(ctx, timeout, self.SubmitOnce)
	}, func() bool {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			return true
		}
	}, self.runtimeProgress())
}
