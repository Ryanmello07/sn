// The activation owner retains two independent initial starts. Process
// postconditions are deliberately separate from protocol or economic success.
package main

import (
	"context"
	"errors"
	"time"
)

// A real production implementation must consume the open activation blockers:
// checkpoint/epoch authority, operator/contract health, custody exclusion and
// effective stake. The CLI deliberately supplies no such capability today.
// Neither a signed process envelope nor owned-RPC readiness implements it.
type validatorActivationAuthority interface {
	authorizeActivation(context.Context, validatorActivationApproval, bootstrapChainPreparation, bootstrapChainReadiness, int) error
}

// Only a bounded projection is retained; EvidenceHash commits the full current
// owned-RPC read. This is an assertion, not independent finality/storage proof.
type validatorActivationReadiness struct {
	ObservedAt      time.Time                     `json:"observed_at"`
	PlanHash        string                        `json:"bootstrap_plan_hash"`
	EvidenceHash    string                        `json:"readiness_hash"`
	FinalizedNumber uint64                        `json:"finalized_number"`
	FinalizedHash   string                        `json:"finalized_hash"`
	Local           bootstrapChainReadinessState  `json:"original_custody"`
	Roles           []bootstrapChainRoleReadiness `json:"ur_validators"`
}

func (self validatorActivationReadiness) validate(plan validatorActivationPlan) error {
	if self.ObservedAt.IsZero() || self.PlanHash != plan.PlanHash || !planSha256(self.EvidenceHash) || !rootCanonicalHash(self.FinalizedHash) || len(self.Roles) != 2 ||
		!planSha256(self.Local.PreparationHash) || !planSha256(self.Local.ContractsHash) || !planSha256(self.Local.RootProgressHash) || !planSha256(self.Local.RootCustodyHash) || !planSha256(self.Local.RootServiceHash) {
		return errors.New("validator activation readiness lacks original custody or exact observation")
	}
	for i, role := range self.Roles {
		if role.Role != plan.Units[i].Role || role.ValidatorId != plan.Units[i].Source.ValidatorId || role.Observed == nil || role.Active == nil || !*role.Active || role.ValidatorPermit == nil || !*role.ValidatorPermit ||
			len(role.ObservationBlockers) != 0 || len(role.ActivationBlockers) == 0 || role.Expected.RegistrationBlock == nil ||
			role.Observed.Hotkey != role.Expected.Hotkey || role.Observed.Coldkey != role.Expected.Coldkey || role.Observed.RegistrationBlock != *role.Expected.RegistrationBlock ||
			self.FinalizedNumber < role.ApprovedFromBlock || self.FinalizedNumber > role.ApprovedThroughBlock {
			return errors.New("validator activation readiness role prerequisites differ")
		}
	}
	return nil
}

// This always executes the qualified current observer; no imported ready file
// can authorize a start. The independently signed preparation is reloaded too.
func observeValidatorActivation(ctx context.Context, approval validatorActivationApproval, now func() time.Time) (bootstrapChainPreparation, bootstrapChainReadiness, *validatorActivationReadiness, error) {
	startedAt := now()
	preparation, err := loadValidatorActivationPreparation(ctx, approval)
	if err != nil {
		return preparation, bootstrapChainReadiness{}, nil, err
	}
	client, err := newOwnedSubmissionClient(approval.Plan.Route)
	if err != nil {
		return preparation, bootstrapChainReadiness{}, nil, err
	}
	defer client.httpClient.CloseIdleConnections()
	readiness, err := client.observeBootstrapChainReadiness(ctx, preparation)
	if err != nil || !readiness.ObservationComplete || readiness.Census == nil || readiness.LocalPreparation == nil || len(readiness.Blockers) != 0 || readiness.PlanHash != approval.Plan.PlanHash {
		return preparation, readiness, nil, errors.Join(errors.New("validator activation current bootstrap observation is unavailable"), err)
	}
	readiness.ContentHash = ""
	readiness.ContentHash = rootObjectHash(readiness)
	identity := readiness.Census.Observation.Identity
	result := &validatorActivationReadiness{ObservedAt: startedAt, PlanHash: readiness.PlanHash, EvidenceHash: readiness.ContentHash,
		FinalizedNumber: identity.FinalizedNumber, FinalizedHash: identity.FinalizedHash, Local: *readiness.LocalPreparation, Roles: readiness.UrValidators}
	return preparation, readiness, result, errors.Join(result.validate(approval.Plan), ctx.Err())
}

type validatorActivationResult struct {
	Schema             string                          `json:"schema"`
	PlanHash           string                          `json:"bootstrap_plan_hash"`
	Status             string                          `json:"status"`
	Operations         uint32                          `json:"operations"`
	Units              [2]validatorActivationUnitState `json:"units"`
	Readiness          *validatorActivationReadiness   `json:"readiness,omitempty"`
	ActivationReady    bool                            `json:"activation_ready"`
	RootServiceActive  bool                            `json:"root_service_active"`
	ChainSuccessProven bool                            `json:"chain_success_proven"`
	Disposition        string                          `json:"operator_disposition"`
}

func (self validatorActivationRecord) result() validatorActivationResult {
	disposition := "Original starts are independent per role. Process evidence does not prove protocol readiness, weights, root activation or economic outcome."
	for _, unit := range self.Units {
		if !unit.StartAt.IsZero() && unit.Generation == nil {
			disposition = "A start is consumed without an attributable acknowledgement. Manual host reconciliation is required; never delete this journal or create another start allowance."
		}
	}
	return validatorActivationResult{Schema: validatorActivationSchema, PlanHash: self.Approval.Plan.PlanHash, Status: self.Status, Operations: self.Operations,
		Units: self.Units, Readiness: self.Readiness, Disposition: disposition}
}

// Installation, admission and start are explicit distinct operations. Every
// effect follows synced intent; all manager/RPC operations join before return.
func advanceValidatorActivation(ctx context.Context, store *validatorActivationStore, host *validatorActivationHost, authority validatorActivationAuthority, operation string, now func() time.Time) (validatorActivationResult, error) {
	if ctx == nil || ctx.Err() != nil || store == nil || host == nil || now == nil || operation != "install" && operation != "admit" && operation != "start" && operation != "resume" {
		return validatorActivationResult{}, errors.New("validator activation owner or operation is unavailable")
	}
	record, err := store.load(ctx)
	if err != nil {
		return validatorActivationResult{}, err
	}
	plan := store.approval.Plan
	// These are local refusals, not attempted observations/effects. Repeated
	// unavailable-capability calls must not spend a future operation allowance.
	// A consumed acknowledged start without its postcondition still reconciles.
	if operation != "install" {
		fresh, recovery := false, false
		for _, unit := range record.Units {
			if !unit.Installed {
				record.Status = "source-refused"
				return record.result(), errors.New("validator activation requires both installed units before admission or recovery")
			}
			if !unit.StartAt.IsZero() && unit.Generation == nil && operation != "admit" {
				record.Status = "partial"
				return record.result(), errors.New("validator activation consumed start needs manual reconciliation; no operation was consumed")
			}
			fresh = fresh || unit.StartAt.IsZero()
			recovery = recovery || !unit.StartAt.IsZero() && unit.Generation != nil && unit.Completed == nil
		}
		if fresh && !recovery && (operation == "start" && authority == nil || operation == "resume") {
			record.Status = "activation-authority-unavailable"
			return record.result(), errors.New("validator activation requires a qualified current authority; no operation was consumed")
		}
	}
	finish := func(status string, cause error) (validatorActivationResult, error) {
		record.Status = status
		return record.result(), errors.Join(cause, store.save(record))
	}
	stamp := now()
	if stamp.IsZero() || stamp.Before(record.HighWaterAt) {
		return finish("clock-rollback", errors.New("validator activation clock moved backwards"))
	}
	record.HighWaterAt = stamp
	if record.Operations >= plan.MaximumOperations {
		return finish("operation-limit", errors.New("validator activation operation allowance is exhausted"))
	}
	if (operation == "install" || operation == "start") && (stamp.Before(plan.ValidFrom) || !stamp.Before(plan.ExpiresAt)) {
		return finish("approval-window-closed", errors.New("validator activation effect window is closed"))
	}
	record.Operations++
	record.Status = "operation-reserved"
	if err := store.save(record); err != nil {
		return record.result(), err
	}
	preparation, err := loadValidatorActivationPreparation(ctx, store.approval)
	if err != nil {
		return finish("source-refused", err)
	}
	if operation == "install" {
		for i := range record.Units {
			if !record.Units[i].StartAt.IsZero() {
				return finish("source-refused", errors.New("validator activation cannot reinstall after a consumed start"))
			}
		}
		for i := range record.Units {
			unit := &record.Units[i]
			unit.InstallIntent, unit.Status = true, "installing"
			if err := store.save(record); err != nil {
				return record.result(), err
			}
			if err := validatorActivationWindow(ctx, plan, record.HighWaterAt, now(), nil); err != nil {
				return finish("approval-window-closed", err)
			}
			if err := host.install(ctx, plan, i, preparation.Plan.Config.Validators[i].Config); err != nil {
				return finish("source-refused", err)
			}
			unit.Installed, unit.Status = true, "installed"
			if err := store.save(record); err != nil {
				return record.result(), err
			}
		}
		if err := validatorActivationWindow(ctx, plan, record.HighWaterAt, now(), nil); err != nil {
			return finish("approval-window-closed", err)
		}
		if _, err := host.host.command(ctx, plan.hostPlan(0), "--system", "--no-pager", "--no-ask-password", "daemon-reload"); err != nil {
			return finish("source-refused", err)
		}
		if err := host.admit(ctx, plan, record); err != nil {
			return finish("source-refused", err)
		}
		return finish("installed", nil)
	}
	if err := host.admit(ctx, plan, record); err != nil {
		return finish("source-refused", err)
	}
	if operation == "admit" {
		_, _, evidence, err := observeValidatorActivation(ctx, store.approval, now)
		if err != nil {
			return finish("source-refused", err)
		}
		record.Readiness = evidence
		return finish("admitted-process-only", nil)
	}
	for i := range record.Units {
		unit, profile := &record.Units[i], plan.hostPlan(i)
		if !unit.StartAt.IsZero() && unit.Generation == nil {
			unit.Status = "uncertain-consumed-start"
			return finish("partial", errors.New("validator activation consumed start has no attributable acknowledgement"))
		}
		if unit.StartAt.IsZero() {
			if operation == "resume" || authority == nil {
				return finish("activation-authority-unavailable", errors.New("validator activation requires a qualified current authority; readiness and signed unit approval do not discharge activation blockers"))
			}
			preparation, readiness, evidence, err := observeValidatorActivation(ctx, store.approval, now)
			if err != nil {
				return finish("source-refused", err)
			}
			if err := authority.authorizeActivation(ctx, store.approval, preparation, readiness, i); err != nil {
				return finish("authority-refused", err)
			}
			// Expensive external authority/readiness precedes final host checks.
			// The host deployment custodian excludes privileged concurrent edits.
			if err := host.admit(ctx, plan, record); err != nil {
				return finish("source-refused", err)
			}
			stamp = now()
			if err := validatorActivationWindow(ctx, plan, record.HighWaterAt, stamp, evidence); err != nil {
				return finish("approval-window-closed", err)
			}
			monotonic, err := host.host.monotonic()
			if err != nil || monotonic == 0 {
				return finish("source-refused", errors.Join(errors.New("validator activation monotonic clock unavailable"), err))
			}
			record.HighWaterAt, record.Readiness = stamp, evidence
			unit.StartAt, unit.StartMonotonicUsec, unit.Readiness, unit.Status = stamp, monotonic, evidence, "start-consumed"
			if err := store.save(record); err != nil {
				return record.result(), err
			}
			if err := validatorActivationWindow(ctx, plan, record.HighWaterAt, now(), evidence); err != nil {
				unit.Status = "uncertain-consumed-start"
				return finish("partial", err)
			}
			if err := host.host.start(ctx, profile); err != nil {
				unit.Status = "uncertain-consumed-start"
				return finish("partial", err)
			}
			manager, err := host.host.inspect(ctx, profile)
			if err != nil || !repairValidatorRunning(profile, manager, manager.Generation) || manager.Generation.StartedUsec < monotonic {
				unit.Status = "uncertain-consumed-start"
				return finish("partial", errors.Join(errors.New("validator activation start lacks an attributable invocation"), err))
			}
			generation := manager.Generation
			unit.Generation, unit.Status = &generation, "waiting-progress"
			if err := store.save(record); err != nil {
				return record.result(), err
			}
		}
		manager, err := host.host.inspect(ctx, profile)
		if err != nil || !repairValidatorRunning(profile, manager, *unit.Generation) {
			return finish("partial", errors.Join(errors.New("validator activation retained invocation changed"), err))
		}
		stamp = now()
		if stamp.Before(record.HighWaterAt) {
			return finish("clock-rollback", errors.New("validator activation postcondition clock moved backwards"))
		}
		record.HighWaterAt = stamp
		postcondition, err := host.host.progress(ctx, profile, unit.StartAt, stamp)
		if err != nil {
			return finish("partial", err)
		}
		manager, err = host.host.inspect(ctx, profile)
		if err != nil || !repairValidatorRunning(profile, manager, *unit.Generation) {
			return finish("partial", errors.Join(errors.New("validator activation invocation changed during postcondition"), err))
		}
		unit.Completed, unit.Status = postcondition, "process-observed"
		if err := store.save(record); err != nil {
			return record.result(), err
		}
	}
	return finish("processes-observed", nil)
}

// Synced intent can be slow. Cancellation, rollback, expiry and read age are
// rechecked after sync; refusal never refunds a consumed allowance.
func validatorActivationWindow(ctx context.Context, plan validatorActivationPlan, highWater, now time.Time, evidence *validatorActivationReadiness) error {
	if now.IsZero() || now.Before(highWater) || now.Before(plan.ValidFrom) || !now.Before(plan.ExpiresAt) ||
		evidence != nil && (now.Before(evidence.ObservedAt) || now.Sub(evidence.ObservedAt) > time.Duration(plan.MaximumSampleAgeSeconds)*time.Second) || ctx.Err() != nil {
		return errors.Join(errors.New("validator activation authority window or current observation expired"), ctx.Err())
	}
	return nil
}
