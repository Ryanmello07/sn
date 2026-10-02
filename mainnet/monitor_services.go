// A finite joined worker per expected role isolates service observations from
// chain retries and from other roles. No worker owns signing or repair state.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/urfoundation/sn/internal/durablehead"
)

// Hooks observe real reads, real durability, and owned waits. None can supply
// a source record, source match, checkpoint content or metric verdict.
type monitorServiceHooks struct {
	read          func(string) monitorServiceReadHooks
	syncDirectory func(role, kind string, file *os.File) error
	afterClose    func(role, kind string, file *os.File) error
	afterWorker   func(role string, exit int)
	afterResult   func(context.Context, int)
	afterEvent    func(context.Context, string)
	wait          func(context.Context, string, time.Duration) bool
}

// Role events contain bounded operational evidence and a closed export outcome.
type monitorServiceEvent struct {
	Schema           string                              `json:"schema"`
	Role             string                              `json:"role"`
	ObservedAt       string                              `json:"observed_at"`
	Status           string                              `json:"status"`
	Severity         string                              `json:"severity,omitempty"`
	Publication      string                              `json:"publication"`
	State            *monitorValidatorState              `json:"state"`
	Diagnostics      *monitorDiagnosticObservation       `json:"diagnostics,omitempty"`
	NativeDeadline   *monitorNativeDeadlineObservation   `json:"native_deadline,omitempty"`
	SteeringLiveness *monitorSteeringLivenessObservation `json:"steering_liveness,omitempty"`
}

// Every domain owns its output files and all retries until the parent joins it.
type monitorValidatorWorker struct {
	policy     monitorValidatorPolicy
	checkpoint *monitorServiceCheckpoint
	metrics    *monitorMetricsStore
	state      *monitorValidatorState
	storage    monitorStorageRecovery
}

// A terminal role releases its own descriptors before reporting completion.
// Parent cleanup remains idempotent and also covers partially admitted roles.
func closeMonitorServiceOwners(role string, metrics *monitorMetricsStore, checkpoint *monitorCheckpointStore, hooks monitorServiceHooks) error {
	var metricsFile, checkpointFile *os.File
	if metrics != nil {
		metricsFile = metrics.lock
	}
	if checkpoint != nil {
		checkpointFile = checkpoint.lock
	}
	var result error
	for _, owner := range []struct {
		kind  string
		file  *os.File
		close func() error
	}{
		{kind: "metrics", file: metricsFile, close: metrics.close},
		{kind: "checkpoint", file: checkpointFile, close: checkpoint.close},
	} {
		if owner.file == nil {
			continue
		}
		err := owner.close()
		if hooks.afterClose != nil {
			err = errors.Join(err, hooks.afterClose(role, owner.kind, owner.file))
		}
		if err != nil {
			result = errors.Join(result, fmt.Errorf("%s %s close: %w", role, owner.kind, err))
		}
	}
	return result
}

// Cancellation interrupts waits. Physical regular-file operations are bounded
// in bytes and remain joined; no abandoned timeout goroutine performs I/O later.
func waitMonitorService(ctx context.Context, role string, duration time.Duration, hooks monitorServiceHooks) bool {
	if hooks.wait != nil {
		return hooks.wait(ctx, role, duration)
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}

// Existing chain ownership stays separate. Ordinary output failures retry only
// that domain; runtime integrity failures stop only the affected domain.
func runMonitorServices(ctx context.Context, client *rpcClient, expected identityExpectation, policy *monitorServicesPolicy, checkpointPath, metricsPath string, interval, stallAfter time.Duration, stdout, stderr io.Writer, now func() time.Time, hooks monitorServiceHooks) (result int) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	output, err := newMonitorOutput(ctx, stdout, stderr, len(policy.Validators)+len(policy.Operators)+len(policy.Providers), now)
	if err != nil {
		return 3
	}
	defer func() {
		if output.close() != nil {
			result = 3
		}
	}()
	diagnostic := output.errors.Writer("diagnostic")
	var workers []*monitorValidatorWorker
	var operators []*monitorOperatorWorker
	var providers []*monitorProviderWorker
	defer func() {
		var cleanupErr error
		for _, worker := range providers {
			cleanupErr = errors.Join(cleanupErr, worker.close(hooks))
		}
		for _, worker := range operators {
			cleanupErr = errors.Join(cleanupErr, closeMonitorServiceOwners(worker.policy.Role, worker.metrics, worker.checkpoint.owner, hooks))
		}
		for _, worker := range workers {
			cleanupErr = errors.Join(cleanupErr, closeMonitorServiceOwners(worker.policy.Role, worker.metrics, worker.checkpoint.owner, hooks))
		}
		if cleanupErr != nil {
			fmt.Fprintln(diagnostic, "monitor service cleanup:", cleanupErr)
			result = 3
		}
	}()
	for _, validator := range policy.Validators {
		checkpointFile, metricsFile := monitorValidatorPaths(checkpointPath, metricsPath, validator.Role)
		checkpoint, err := openMonitorServiceCheckpoint(checkpointFile, expected, validator, ctx)
		if err != nil {
			fmt.Fprintln(diagnostic, "monitor service checkpoint admission:", err)
			return 3
		}
		worker := &monitorValidatorWorker{policy: validator, checkpoint: checkpoint}
		workers = append(workers, worker)
		metrics, err := openMonitorMetrics(metricsFile, ctx)
		if err != nil {
			fmt.Fprintln(diagnostic, "monitor service metrics admission:", err)
			return 3
		}
		worker.metrics = metrics
		worker.state, err = checkpoint.load(ctx)
		if err != nil {
			fmt.Fprintln(diagnostic, "monitor service retained state:", err)
			return 3
		}
		if hooks.syncDirectory != nil {
			checkpoint.owner.syncDirectory = func(file *os.File) error { return hooks.syncDirectory(validator.Role, "checkpoint", file) }
			metrics.syncDirectory = func(file *os.File) error { return hooks.syncDirectory(validator.Role, "metrics", file) }
		}
	}

	for _, operator := range policy.Operators {
		worker, err := openMonitorOperatorWorker(ctx, operator, expected, checkpointPath, metricsPath, hooks)
		if err != nil {
			fmt.Fprintln(diagnostic, "monitor operator admission:", err)
			return 3
		}
		operators = append(operators, worker)
	}
	for _, provider := range policy.Providers {
		worker, err := openMonitorProviderWorker(ctx, provider, expected, checkpointPath, metricsPath, hooks)
		if err != nil {
			fmt.Fprintln(diagnostic, "monitor provider admission:", err)
			return 3
		}
		providers = append(providers, worker)
	}
	// The channel holds every terminal result even during cancellation. Every
	// launched worker sends once and is consumed before output owners close.
	results := make(chan int, len(workers)+len(operators)+len(providers)+1)
	go func() {
		backoff := time.Second
		for ctx.Err() == nil {
			exit := runChainMonitor(ctx, client, expected, checkpointPath, metricsPath, interval, stallAfter, output.events.Writer("chain"), diagnostic, now, hooks)
			if exit != 1 || ctx.Err() != nil {
				results <- exit
				return
			}
			if !waitMonitorService(ctx, "chain", backoff, hooks) {
				results <- 0
				return
			}
			backoff = min(2*backoff, time.Minute)
		}
		results <- 0
	}()
	for index, worker := range workers {
		writer := output.events.Writer(fmt.Sprintf("validator%d", index))
		go func() {
			exit := worker.run(ctx, interval, writer, diagnostic, now, hooks)
			if err := closeMonitorServiceOwners(worker.policy.Role, worker.metrics, worker.checkpoint.owner, hooks); err != nil {
				fmt.Fprintln(diagnostic, "monitor service cleanup:", err)
				exit = 3
			}
			if hooks.afterWorker != nil {
				hooks.afterWorker(worker.policy.Role, exit)
			}
			results <- exit
		}()
	}

	for index, worker := range operators {
		writer := output.events.Writer(fmt.Sprintf("validator%d", len(workers)+index))
		go func() {
			exit := worker.run(ctx, interval, stallAfter, writer, diagnostic, now, hooks)
			if err := closeMonitorServiceOwners(worker.policy.Role, worker.metrics, worker.checkpoint.owner, hooks); err != nil {
				fmt.Fprintln(diagnostic, "monitor operator cleanup:", err)
				exit = 3
			}
			if hooks.afterWorker != nil {
				hooks.afterWorker(worker.policy.Role, exit)
			}
			results <- exit
		}()
	}
	for index, worker := range providers {
		writer := output.events.Writer(fmt.Sprintf("validator%d", len(workers)+len(operators)+index))
		go func() {
			exit := worker.run(ctx, interval, writer, diagnostic, now, hooks)
			if err := worker.close(hooks); err != nil {
				fmt.Fprintln(diagnostic, "monitor provider cleanup:", err)
				exit = 3
			}
			if hooks.afterWorker != nil {
				hooks.afterWorker(worker.policy.Role, exit)
			}
			results <- exit
		}()
	}
	for remaining := len(workers) + len(operators) + len(providers) + 1; remaining > 0; remaining-- {
		exit := <-results
		if exit != 0 {
			result = max(result, exit)
		}
		if hooks.afterResult != nil {
			hooks.afterResult(ctx, exit)
		}
		// A failed domain stays visibly stopped while unrelated chain/service
		// owners continue. Parent cancellation still joins every live worker.
	}
	return result
}

// Completed source reads continue through ordinary checkpoint/metrics errors.
// The previous acknowledged export timestamp cannot advance on ambiguous writes.
func (self *monitorValidatorWorker) run(ctx context.Context, interval time.Duration, stdout, stderr io.Writer, now func() time.Time, hooks monitorServiceHooks) int {
	startedAt := now().UTC()
	publication := "starting"
	backoff := time.Second
	encoder := json.NewEncoder(stdout)
	for ctx.Err() == nil {
		readHooks := monitorServiceReadHooks{}
		if hooks.read != nil {
			readHooks = hooks.read(self.policy.Role)
		}
		value, code := readMonitorValidatorProgress(ctx, self.policy, readHooks)
		if ctx.Err() != nil {
			return 0
		}
		sampledAt := now().UTC()
		self.state.observe(startedAt, sampledAt, value, code)
		if err := self.state.retainReadIncident(self.policy); err != nil {
			fmt.Fprintln(stderr, "monitor service read incident:", err)
			return 3
		}
		self.state.retainNativeDeadline(self.policy, sampledAt)
		if err := self.state.retainSteeringLiveness(self.policy); err != nil {
			fmt.Fprintln(stderr, "monitor service steering liveness:", err)
			return 3
		}
		checkpointErr := self.checkpoint.save(self.state)
		if checkpointErr != nil {
			publication = "retrying"
		}
		raw, renderErr := renderMonitorValidatorMetrics(self.policy, self.state, publication, checkpointErr == nil)
		if renderErr != nil {
			return 3
		}
		observation := monitorDiagnosticSnapshot(stdout, stderr)
		raw = appendMonitorOutputMetrics(raw, "sn_mainnet_validator", self.policy.Role, observation)
		metricsErr := self.metrics.saveRaw(raw)
		combined := errors.Join(checkpointErr, metricsErr)
		var ownership *monitorOutputOwnershipError
		terminal := errors.As(combined, &ownership)
		if combined != nil {
			publication = "retrying"
		} else {
			publication = "published"
			self.state.PublicationLastSuccessAt = sampledAt
		}
		status, severity := self.state.condition(sampledAt, self.policy)
		event := monitorServiceEvent{Schema: "urnetwork-mainnet-validator-event-v1", Role: self.policy.Role, ObservedAt: sampledAt.Format(time.RFC3339Nano), Status: status, Severity: severity, Publication: publication, State: self.state}
		if self.policy.NativeDeadline != nil || self.state.NativeDeadline != nil {
			deadline := self.state.nativeDeadline(self.policy, sampledAt)
			event.NativeDeadline = &deadline
		}
		if self.policy.SteeringLiveness != nil || self.state.SteeringLiveness != nil {
			steering := self.state.steeringLiveness(self.policy)
			event.SteeringLiveness = &steering
		}
		event.Diagnostics = observation
		if terminal {
			event.Publication, event.Severity = "ownership-error", "critical"
		}
		if err := encoder.Encode(event); err != nil {
			if ctx.Err() != nil {
				return 0
			}
			return 3
		}
		if terminal {
			return 3
		}
		if errors.Is(checkpointErr, durablehead.ErrUncertain) {
			prior := self.checkpoint
			err := self.storage.resume(ctx, self.policy.Role, func() error {
				file := prior.owner.lock
				err := prior.owner.close()
				if hooks.afterClose != nil {
					err = errors.Join(err, hooks.afterClose(self.policy.Role, "checkpoint", file))
				}
				return err
			}, func() error {
				next, err := openMonitorServiceCheckpoint(prior.owner.path, prior.owner.expected, self.policy, ctx)
				if err != nil {
					return err
				}
				state, err := next.load(ctx)
				if err != nil {
					return errors.Join(err, next.owner.close())
				}
				next.owner.syncDirectory = prior.owner.syncDirectory
				self.checkpoint, self.state = next, state
				return nil
			}, hooks)
			if err != nil {
				if ctx.Err() != nil {
					return 0
				}
				fmt.Fprintln(stderr, "monitor service storage continuation:", err)
				return 3
			}
			continue
		}
		delay := interval
		if combined != nil {
			delay = backoff
			backoff = min(2*backoff, time.Minute)
		} else {
			backoff = time.Second
		}
		if !waitMonitorService(ctx, self.policy.Role, delay, hooks) {
			return 0
		}
	}
	return 0
}
