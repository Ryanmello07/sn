// Combined observations use the same bounded durable textfile owner as other
// monitor roles. Gauges describe evidence availability, never spending authority.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Restart preserves the previous timestamp and incident until a new sample is
// durably committed. Only an absent metrics file receives the startup marker.
func initializeEconomicConservationMetrics(owner *monitorMetricsStore) error {
	if err := owner.validateDestination(); err != nil {
		return err
	}
	if _, err := owner.directory.stat(filepath.Base(owner.path)); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return saveEconomicConservationMetrics(owner, nil)
}

func saveEconomicConservationMetrics(owner *monitorMetricsStore, summary *economicConservationSummary) error {
	raw, err := renderEconomicConservationMetrics(summary)
	if err != nil {
		return err
	}
	return owner.saveRaw(raw)
}

// No endpoint, identity, error or amount becomes a metric label. A nullable
// conformance result has a separate known bit; unknown is never a healthy zero.
func renderEconomicConservationMetrics(summary *economicConservationSummary) ([]byte, error) {
	value := economicConservationSummary{}
	if summary != nil {
		if summary.Schema != "urnetwork-economic-conservation-sample-v1" || summary.SampleAt.IsZero() || summary.SampleAt.Unix() < 0 || !planSha256(summary.PolicyHash) || !planSha256(summary.CheckpointHash) {
			return nil, errors.New("economic metrics require an original completed summary")
		}
		value = *summary
	}
	bit := func(value bool) uint64 {
		if value {
			return 1
		}
		return 0
	}
	var sampleAt, pendingRoots, heldRoots, claimHeld uint64
	if summary != nil {
		sampleAt = uint64(value.SampleAt.Unix())
	}
	claimsCurrent := summary != nil
	for _, status := range value.ClaimStatuses {
		if status != "ok" {
			claimsCurrent = false
		}
		if monitorClaimTerminal(status) {
			claimHeld++
		}
	}
	if value.OriginalEntitlements != nil {
		pendingRoots = value.OriginalEntitlements.PendingRoots
		heldRoots = value.OriginalEntitlements.HeldRoots + value.OriginalEntitlements.CapacityHeldRoots
	}
	healthy := summary != nil && value.NativeCurrent && value.VaultCurrent && claimsCurrent && !value.NativePending && !value.NativeFeePending && !value.NativeHeld && !value.VaultHeld && value.NativeIssue == "" && value.VaultIssue == "" && value.NativeFeeIssue == "" && value.JoinIssue == "" && pendingRoots == 0 && heldRoots == 0
	var output strings.Builder
	for _, metric := range []struct {
		name  string
		help  string
		value uint64
	}{
		{name: "sample_timestamp_seconds", help: "Last durably committed combined sample; zero before the first sample.", value: sampleAt},
		{name: "sample_healthy", help: "One for complete current reads without pending or held sources; independent freshness and conformance checks are required.", value: bit(healthy)},
		{name: "native_current", help: "Native observation is current in this sample.", value: bit(value.NativeCurrent)},
		{name: "vault_current", help: "Vault observation is current in this sample.", value: bit(value.VaultCurrent)},
		{name: "claims_current", help: "All selected Claim observations are current in this sample.", value: bit(claimsCurrent)},
		{name: "native_pending", help: "Original native capture or proof is still pending.", value: bit(value.NativePending)},
		{name: "fee_pending", help: "Selected original fee observation is still pending.", value: bit(value.NativeFeePending)},
		{name: "entitlement_pending_roots", help: "Original entitlement roots awaiting their evidence.", value: pendingRoots},
		{name: "native_integrity_held", help: "Original native evidence has a retained contradiction.", value: bit(value.NativeHeld)},
		{name: "vault_integrity_held", help: "Original vault evidence has a retained contradiction.", value: bit(value.VaultHeld)},
		{name: "claim_integrity_held", help: "Number of selected Claim observations with terminal evidence refusal.", value: claimHeld},
		{name: "entitlement_held_roots", help: "Original entitlement roots held by integrity or capacity admission.", value: heldRoots},
		{name: "join_held", help: "Original cross-domain reconciliation has an unresolved refusal.", value: bit(value.JoinIssue != "")},
		{name: "capacity_warning", help: "The admitted original retention resources need reviewed continuation.", value: bit(value.CapacityWarning)},
		{name: "target_known", help: "The original economic target has a definite observed result.", value: bit(value.TargetMet != nil)},
		{name: "target_met", help: "The original economic target is met; meaningful only with target_known and fresh evidence, never activation authority.", value: bit(value.TargetMet != nil && *value.TargetMet)},
		{name: "complete_evidence", help: "Every required independent economic evidence component is admitted.", value: bit(value.Conformance != nil && value.Conformance.CompleteEvidence)},
		{name: "original_fee_census_complete", help: "Complete original body and fee coverage is admitted.", value: bit(value.OriginalFees != nil && value.OriginalFees.Complete)},
		{name: "original_finality_complete", help: "Original independent consensus certificates cover the selected obligations.", value: bit(value.OriginalFinality != nil && value.OriginalFinality.Complete)},
		{name: "provider_measurements_authenticated", help: "Complete original provider measurement authority is admitted.", value: bit(value.OriginalEntitlements != nil && value.OriginalEntitlements.ProviderMeasurementsAuthenticated)},
		{name: "facts_remaining", help: "Facts remaining inside the current signed active retention budget.", value: value.FactsRemaining},
		{name: "native_finalized_block", help: "Retained native cursor; current and consensus coverage gauges are required separately.", value: value.NativeCursor.Number},
		{name: "vault_finalized_block", help: "Retained vault cursor; current and consensus coverage gauges are required separately.", value: value.VaultCursor.Number},
	} {
		fmt.Fprintf(&output, "# HELP sn_mainnet_conservation_%s %s\n# TYPE sn_mainnet_conservation_%s gauge\nsn_mainnet_conservation_%s %d\n", metric.name, metric.help, metric.name, metric.name, metric.value)
	}
	return []byte(output.String()), nil
}
