package main

// These source controls retain the actual integration roots in producer modes;
// they never substitute for real signed replay, native persistence or race runs.

import (
	"os"
	"strings"
	"testing"
)

// Composition retains all original head/EMA roots and the independent exact
// read-control boundary rather than moving coverage to a legacy helper.
func TestProducerGateStateSelectionCoversHeadEMAIntegration(t *testing.T) {
	assertProducerStateRegressionCoverage(t, "producer_tests", "./validator", "head-EMA-runtime-integration", []string{
		"../validator/release_head_v2_test.go", "../validator/release_head_v2_admission_test.go", "../validator/release_head_v2_integration_test.go",
		"../validator/head_ema_v2_test.go", "../validator/head_ema_v2_control_test.go", "../validator/head_ema_v2_runtime_test.go", "../validator/head_ema_v2_write_test.go",
	})
	assertProducerStateRegressionCoverage(t, "capture_tests", "./sim-testnet", "head-EMA-runtime-retention", []string{"release_gate_head_ema_integration_test.go"})
}

// The real entrypoint owns the constructor-selected token and retains exact
// bounded math/namespace checks. Startup/intent routing is still a separate task.
func TestProducerGateStateSelectionPinsHeadEMAIntegrationOwner(t *testing.T) {
	for _, edge := range []struct{ path, caller, callee string }{
		{path: "../validator/release_head_v2.go", caller: "gatherHeadV2", callee: "ownReleaseHeadEMAV2"},
		{path: "../validator/release_head_v2.go", caller: "gatherHeadV2", callee: "reserveReleaseHeadV2OperatorControls"},
		{path: "../validator/release_head_v2.go", caller: "gatherHeadV2", callee: "finish"},
		{path: "../validator/release_head_v2_owner.go", caller: "ownReleaseHeadEMAV2", callee: "CompareAndSwap"},
		{path: "../validator/release_head_v2_owner.go", caller: "ownReleaseHeadEMAV2", callee: "releaseHeadV2Limits"},
		{path: "../validator/release_head_v2_owner.go", caller: "witness", callee: "witnessHeadEMAStoreV2Runtime"},
		{path: "../validator/release_head_v2_owner.go", caller: "finish", callee: "witness"},
		{path: "../validator/release_head_v2_owner.go", caller: "releaseHeadV2Limits", callee: "min"},
		{path: "../validator/release_head_v2_owner.go", caller: "reserveReleaseHeadV2OperatorControls", callee: "charge"},
		{path: "../validator/release_head_v2_owner.go", caller: "reserveReleaseHeadV2OperatorControls", callee: "releaseMeasurementV2ControlStorage"},
		{path: "../validator/release_head_v2_admission.go", caller: "admitReleaseHeadV2KnownWithLock", callee: "reserveReleaseHeadV2OwnerWithLock"},
		{path: "../validator/release_head_v2_admission.go", caller: "previewForEpochV2WithBudget", callee: "ownReleaseHeadEMAV2"},
		{path: "../validator/release_head_v2_admission.go", caller: "previewForEpochV2WithBudget", callee: "finish"},
		{path: "../validator/head_ema_v2_load_unix.go", caller: "NewHeadEMAStoreV2", callee: "newHeadEMAStoreV2"},
		{path: "../validator/head_ema_v2_runtime.go", caller: "CommitForEpochV2", callee: "runHeadEMAStoreV2"},
	} {
		if !releaseClosureFunctionCalls(t, edge.path, edge.caller)[edge.callee] {
			t.Errorf("%s lost %s", edge.caller, edge.callee)
		}
	}
	for _, path := range []string{"../validator/release_head_v2_test.go", "../validator/release_head_v2_admission_test.go", "../validator/release_measurement_v2_bound_test.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "NewHeadEMAStore(") {
			t.Errorf("%s replaced v2 behavior with a legacy-created owner", path)
		}
		if strings.Contains(string(source), ".PreviewForEpoch(") || strings.Contains(string(source), ".CommitForEpoch(") {
			t.Errorf("%s discarded explicit operation contexts during v2 fixture preparation", path)
		}
	}
}
