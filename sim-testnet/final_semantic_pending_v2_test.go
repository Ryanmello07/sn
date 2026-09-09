//go:build linux || darwin

// The existing campaign owner must terminate after two captured phases even
// when interpretation is unavailable, while retaining every real failure.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Wrappers and homogeneous pending joins are allowed; any real leaf wins.
func TestFinalCaptureV2PendingErrorNeverHidesRealFailure(t *testing.T) {
	pending := &finalSemanticAnalysisPendingError{}
	failure := errors.New("original signed source changed")
	for _, candidate := range []error{pending, fmt.Errorf("release analysis: %w", pending), errors.Join(pending, fmt.Errorf("soak: %w", pending))} {
		if !isFinalSemanticAnalysisPending(candidate) {
			t.Errorf("exact pending result lost: %v", candidate)
		}
	}
	for _, candidate := range []error{nil, failure, errors.New(pending.Error()), context.Canceled, errors.Join(pending, failure), fmt.Errorf("outer: %w", errors.Join(context.Canceled, pending))} {
		if isFinalSemanticAnalysisPending(candidate) {
			t.Errorf("real/foreign error was converted to pending: %v", candidate)
		}
	}
}

// Real signed campaign fixtures cross the handoff and both analyzers run once.
// No timer, retry, lingering owner, or semantic success is needed to return.
func TestFinalCaptureV2PendingAnalyzerClosesBothPhasesAndJoins(t *testing.T) {
	cfg := testResolvedConfig(t)
	stateRoot := t.TempDir()
	roles, err := BuildRoleSecrets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	journal := openCampaignTestJournal(t, stateRoot)
	var stateLock sync.Mutex
	calls := map[string]int{}
	phases := []string{}
	releaseAnalyzed := make(chan struct{})
	runner := func(ctx context.Context, _ *ResolvedConfig, root, phase string, _ *Journal, _ *Executor, _ *scenarioCampaignAttempt) error {
		if phase == "production-soak" {
			select {
			case <-releaseAnalyzed:
			case <-ctx.Done():
				return ctx.Err()
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		phases = append(phases, phase)
		if phase == "release-1.0" {
			writeScenarioCampaignFixture(t, cfg, root, phase, 26, 32)
		} else {
			writeScenarioCampaignFixture(t, cfg, root, phase, 32, 36)
		}
		return nil
	}
	analyzer := func(_ context.Context, _ *ResolvedConfig, _, _ string, _ *RoleSecrets, result *ScenarioResult) error {
		stateLock.Lock()
		calls[result.Name]++
		stateLock.Unlock()
		if result.Name == "release-1.0" {
			close(releaseAnalyzed)
		}
		return &finalSemanticAnalysisPendingError{}
	}
	err = runReleaseCandidateCampaignWithAnalyzer(t.Context(), cfg, stateRoot, journal, campaignTestExecutor(), roles, runner, noOpCampaignPreflight, analyzer)
	if !isFinalSemanticAnalysisPending(err) || len(phases) != 2 || phases[0] != "release-1.0" || phases[1] != "production-soak" {
		t.Fatalf("pending campaign phases=%v result=%v", phases, err)
	}
	stateLock.Lock()
	defer stateLock.Unlock()
	if calls["release-1.0"] != 1 || calls["production-soak"] != 1 {
		t.Fatalf("pending analyzer retried or leaked: %v", calls)
	}
	for _, phase := range phases {
		result, _, err := loadCompletedScenarioCampaign(cfg, stateRoot, roles, phase)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{finalSemanticEvidenceFilename, finalSemanticMarkdownFilename, finalSemanticSupplementFilename} {
			if _, err := os.Lstat(filepath.Join(stateRoot, "runs", result.RunID, name)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("pending phase created accepted output %s: %v", name, err)
			}
		}
	}
}

// A pending leaf joined to source failure still cancels the real live runner.
func TestFinalCaptureV2PendingAnalyzerMixedFailureCancelsProduction(t *testing.T) {
	cfg := testResolvedConfig(t)
	stateRoot := t.TempDir()
	roles, err := BuildRoleSecrets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	journal := openCampaignTestJournal(t, stateRoot)
	started := make(chan struct{})
	failure := errors.New("companion readback changed")
	runner := func(ctx context.Context, _ *ResolvedConfig, root, phase string, _ *Journal, _ *Executor, _ *scenarioCampaignAttempt) error {
		if phase == "release-1.0" {
			writeScenarioCampaignFixture(t, cfg, root, phase, 26, 32)
			return nil
		}
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	analyzer := func(ctx context.Context, _ *ResolvedConfig, _, _ string, _ *RoleSecrets, _ *ScenarioResult) error {
		select {
		case <-started:
			return errors.Join(&finalSemanticAnalysisPendingError{}, failure)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	err = runReleaseCandidateCampaignWithAnalyzer(t.Context(), cfg, stateRoot, journal, campaignTestExecutor(), roles, runner, noOpCampaignPreflight, analyzer)
	if isFinalSemanticAnalysisPending(err) || !errors.Is(err, failure) || !errors.Is(err, context.Canceled) {
		t.Fatalf("mixed failure was downgraded: %v", err)
	}
}

// Cancellation after a known pending result joins the other live owner.
func TestFinalCaptureV2PendingAnalyzerCancellationJoinsOwner(t *testing.T) {
	cfg := testResolvedConfig(t)
	stateRoot := t.TempDir()
	roles, err := BuildRoleSecrets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	journal := openCampaignTestJournal(t, stateRoot)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	analyzed := make(chan struct{})
	runnerExited := make(chan struct{})
	runner := func(ctx context.Context, _ *ResolvedConfig, root, phase string, _ *Journal, _ *Executor, _ *scenarioCampaignAttempt) error {
		if phase == "release-1.0" {
			writeScenarioCampaignFixture(t, cfg, root, phase, 26, 32)
			return nil
		}
		defer close(runnerExited)
		select {
		case <-analyzed:
			cancel()
		case <-ctx.Done():
		}
		return ctx.Err()
	}
	analyzer := func(_ context.Context, _ *ResolvedConfig, _, _ string, _ *RoleSecrets, _ *ScenarioResult) error {
		defer close(analyzed)
		return &finalSemanticAnalysisPendingError{}
	}
	err = runReleaseCandidateCampaignWithAnalyzer(ctx, cfg, stateRoot, journal, campaignTestExecutor(), roles, runner, noOpCampaignPreflight, analyzer)
	if !errors.Is(err, context.Canceled) || isFinalSemanticAnalysisPending(err) {
		t.Fatalf("cancelled pending capture result=%v", err)
	}
	select {
	case <-runnerExited:
	default:
		t.Fatal("campaign returned before its runner joined")
	}
}
