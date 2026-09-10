//go:build linux || darwin

// Pending analysis owns no background work and grants no acceptance. Its
// immutable job points back to the same signed capture for an explicit resume.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

const finalSemanticCapturePendingStatus = "pending_offline_verification"
const finalSemanticPendingJobFilename = "analysis-pending.json"

// This exact type denotes an absent interpreter, never invalid evidence.
type finalSemanticAnalysisPendingError struct{}

// The command may close a capture while returning this non-success result.
func (self *finalSemanticAnalysisPendingError) Error() string {
	return "compact source bytes are captured; independent V2 final semantic replay is still required (analysis pending, not accepted)"
}

// Every error leaf must be pending. errors.As alone would hide a real error
// joined to pending; bounded traversal also rejects cyclic/custom wrappers.
func isFinalSemanticAnalysisPending(err error) bool {
	var pending func(error, int) bool
	pending = func(value error, depth int) bool {
		if value == nil || depth > 32 {
			return false
		}
		if typed, ok := value.(*finalSemanticAnalysisPendingError); ok {
			return typed != nil
		}
		if joined, ok := value.(interface{ Unwrap() []error }); ok {
			children := joined.Unwrap()
			if len(children) == 0 {
				return false
			}
			for _, child := range children {
				if !pending(child, depth+1) {
					return false
				}
			}
			return true
		}
		if wrapped, ok := value.(interface{ Unwrap() error }); ok {
			return pending(wrapped.Unwrap(), depth+1)
		}
		return false
	}
	return pending(err, 0)
}

// The descriptor is discovery for an explicit resume, not a signed verdict.
// A later semantic_verified supplement supersedes its historical pending state.
type finalSemanticPendingJob struct {
	Schema                       string `json:"schema"`
	Status                       string `json:"status"`
	Phase                        string `json:"phase"`
	RunId                        string `json:"run_id"`
	ResultHash                   string `json:"result_hash"`
	ScenarioCompleteHash         string `json:"scenario_complete_hash"`
	ScenarioEvidenceManifestHash string `json:"scenario_evidence_manifest_hash"`
	CaptureStatusHash            string `json:"capture_status_hash"`
	CollectedInputsHash          string `json:"collected_inputs_hash"`
}

// Called only after the original closure and every original manifest byte
// have been authenticated by the actual supplement owner, outside public writes.
func retainFinalSemanticPendingJob(ctx context.Context, stateRoot string, result *ScenarioResult, closure *finalSemanticOriginalClosure) error {
	if ctx == nil || result == nil || closure == nil || closure.complete == nil || closure.manifest == nil || closure.capture == nil || closure.collected == nil || stateRoot == "" {
		return errors.New("pending semantic job has no original capture owner")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if result.RunID == "" || filepath.Base(result.RunID) != result.RunID || strings.ContainsAny(result.RunID, "/\\\r\n\x00") || result.Result != "pass" || closure.capture.SemanticStatus != finalSemanticCapturePendingStatus || closure.capture.RunID != result.RunID || closure.capture.ResultHash != result.EvidenceHash || closure.collected.RunID != result.RunID || closure.collected.ResultHash != result.EvidenceHash || verifyFinalSemanticCaptureStatus(closure.capture, closure.collected) != nil {
		return errors.New("pending semantic job differs from its closed capture")
	}
	if (result.Name != "release-1.0" && result.Name != "production-soak") || result.Name != closure.capture.Phase || result.Name != closure.collected.Phase || !validCanonicalHashHex(result.EvidenceHash) {
		return errors.New("pending semantic job phase differs from its original capture")
	}
	job := finalSemanticPendingJob{Schema: "urnetwork-final-semantic-pending-v2", Status: finalSemanticCapturePendingStatus, Phase: result.Name, RunId: result.RunID, ResultHash: result.EvidenceHash, ScenarioCompleteHash: closure.complete.ContentHash, ScenarioEvidenceManifestHash: closure.manifest.ContentHash, CaptureStatusHash: closure.capture.EvidenceHash, CollectedInputsHash: closure.collected.EvidenceHash}
	if !validSHA256ContentHash(job.ScenarioCompleteHash) || !validSHA256ContentHash(job.ScenarioEvidenceManifestHash) || !validCanonicalHashHex(job.CaptureStatusHash) || !validCanonicalHashHex(job.CollectedInputsHash) {
		return errors.New("pending semantic job lacks immutable closure hashes")
	}
	raw, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	stageRoot := finalSemanticSupplementStageRoot(stateRoot, result.RunID)
	if err := rejectFinalArtifactSymlinkComponents(stateRoot, stageRoot); err != nil {
		return err
	}
	return errors.Join(writeFinalSemanticImmutableLocal(filepath.Join(stageRoot, finalSemanticPendingJobFilename), append(raw, '\n')), ctx.Err())
}

// Production still authenticates the exact signed release closure and
// lifecycle handoff before any source read. A newer run cannot replace it.
func authenticateFinalPriorCaptureV2(cfg *ResolvedConfig, stateRoot string, result *ScenarioResult) (*ScenarioResult, error) {
	return authenticateFinalPriorCaptureV2Context(context.Background(), cfg, stateRoot, result)
}

// The live caller, not a detached background owner, owns original-file replay.
func authenticateFinalPriorCaptureV2Context(ctx context.Context, cfg *ResolvedConfig, stateRoot string, result *ScenarioResult) (*ScenarioResult, error) {
	if ctx == nil {
		return nil, errors.New("prior capture context is absent")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !finalUsesEvidenceV2(cfg) || result == nil {
		return nil, errors.New("compact prior capture identity is absent")
	}
	if result.Name == "release-1.0" {
		if result.PriorRelease != nil {
			return nil, errors.New("release capture invents a prior phase")
		}
		return nil, nil
	}
	if result.Name != "production-soak" || result.PriorRelease == nil {
		return nil, errors.New("compact production capture lacks its exact signed release gate")
	}
	roles, err := BuildRoleSecrets(cfg)
	if err != nil {
		return nil, err
	}
	prior, _, err := validateExactReleaseCampaignGateContext(ctx, cfg, stateRoot, roles, result.PriorRelease)
	if err != nil {
		return nil, err
	}
	if err := validateFinalPriorProductionIdentity(prior, result); err != nil {
		return nil, err
	}
	return prior, nil
}

// Original authority locators always remain mandatory. Pending capture alone
// omits semantic output that did not exist at the live closure boundary.
func finalCollectedPriorLocators(prior *FinalCollectedPriorPhaseInputs) []FinalArtifactLocator {
	if prior == nil {
		return nil
	}
	result := []FinalArtifactLocator{prior.ScenarioResult, prior.OwnerCompletion, prior.EvidenceManifest, prior.LifecycleHandoff, prior.CaptureStatus, prior.CollectedInputsManifest}
	result = append(result, prior.LiveChainBundles...)
	if prior.SemanticStatus != finalSemanticCapturePendingStatus {
		result = append(result, prior.SemanticSupplement)
		result = append(result, prior.SemanticFileEnvelopes...)
	}
	return result
}

// The current original result pins the predecessor; detached replay must not
// accept a different valid release selected from a mutable directory listing.
func verifyFinalCollectedPendingPriorBinding(cfg *ResolvedConfig, current *FinalSemanticCollectedInputs, loaded map[string][]byte) error {
	if current == nil || current.PriorPhase == nil || current.PriorPhase.SemanticStatus != finalSemanticCapturePendingStatus {
		return nil
	}
	prior := current.PriorPhase
	if !finalUsesEvidenceV2(cfg) || verifyFinalCollectedPriorPhase(prior, current) != nil {
		return errors.New("pending prior capture is not a compact production lineage")
	}
	var result, previous ScenarioResult
	var completion ReleaseEvidenceEnvelope
	if err := decodeStrictJSONBytes(loaded[current.ScenarioResult.URI], &result); err != nil {
		return err
	}
	if err := decodeStrictJSONBytes(loaded[prior.ScenarioResult.URI], &previous); err != nil {
		return err
	}
	if err := decodeStrictJSONBytes(loaded[prior.OwnerCompletion.URI], &completion); err != nil {
		return err
	}
	gate := result.PriorRelease
	if result.Name != current.Phase || result.RunID != current.RunID || result.EvidenceHash != current.ResultHash || validateReleaseCampaignGateShape(cfg, gate) != nil || gate.RunID != prior.RunID || gate.ResultHash != prior.ResultHash || gate.CompleteContentHash != completion.ContentHash || previous.LifecycleHandoff == nil || gate.LifecycleHandoff != *previous.LifecycleHandoff {
		return errors.New("pending prior capture differs from the current original signed handoff")
	}
	if err := validateFinalPriorProductionIdentity(&previous, &result); err != nil {
		return err
	}
	started, err := time.Parse(time.RFC3339Nano, result.StartedAt)
	if err != nil {
		return err
	}
	for _, locator := range []FinalArtifactLocator{prior.OwnerCompletion, prior.EvidenceManifest} {
		var envelope ReleaseEvidenceEnvelope
		if err := decodeStrictJSONBytes(loaded[locator.URI], &envelope); err != nil {
			return err
		}
		created, err := time.Parse(time.RFC3339Nano, envelope.CreatedAt)
		if err != nil || envelope.CreatedAt != created.UTC().Format(time.RFC3339Nano) || !created.Before(started) {
			return errors.New("prior capture authority was not signed before production")
		}
	}
	return nil
}

// Pending never skips owner/domain, original file hashes, terminal snapshot,
// complete live-chain census, or lifecycle authority. Only interpretation waits.
func verifyFinalCollectedPendingPriorBytes(cfg *ResolvedConfig, prior *FinalCollectedPriorPhaseInputs, result *ScenarioResult, complete *ReleaseEvidenceEnvelope, completion *scenarioCompletePayload, manifestEnvelope *ReleaseEvidenceEnvelope, manifest *campaignEvidenceManifestPayload, capture *FinalSemanticCaptureStatus, inputs *FinalSemanticCollectedInputs, loaded map[string][]byte) error {
	if !finalUsesEvidenceV2(cfg) || prior == nil || result == nil || complete == nil || completion == nil || manifestEnvelope == nil || manifest == nil || capture == nil || inputs == nil || prior.SemanticStatus != finalSemanticCapturePendingStatus || prior.SemanticSupplement != (FinalArtifactLocator{}) || len(prior.SemanticFileEnvelopes) != 0 {
		return errors.New("pending prior capture authority is incomplete or contains a verdict")
	}
	if err := validateScenarioCampaignResult(cfg, result, "release-1.0"); err != nil {
		return err
	}
	roles, err := BuildRoleSecrets(cfg)
	if err != nil {
		return err
	}
	owner, exists := roles.EVM["testnet-owner"]
	if !exists {
		return errors.New("pending prior capture owner is absent")
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(owner.PrivateKeyHex, "0x"))
	if err != nil {
		return err
	}
	if err := verifyFinalSemanticOwnerEnvelope(cfg, complete, &key.PublicKey, "scenario-complete", prior.RunID); err != nil {
		return err
	}
	if err := verifyFinalSemanticOwnerEnvelope(cfg, manifestEnvelope, &key.PublicKey, campaignEvidenceManifestKind, prior.RunID); err != nil {
		return err
	}
	originals := map[string]FinalArtifactLocator{
		"result.json":                      prior.ScenarioResult,
		result.LifecycleHandoff.File:       prior.LifecycleHandoff,
		finalSemanticCaptureStatusFilename: prior.CaptureStatus,
		"final-inputs/manifest.json":       prior.CollectedInputsManifest,
	}
	manifestEntries := make(map[string]campaignEvidenceFileEntry, len(manifest.Files))
	for _, entry := range manifest.Files {
		manifestEntries[entry.Path] = entry
	}
	for name, locator := range originals {
		raw := loaded[locator.URI]
		entry, exists := manifestEntries[name]
		if !exists || uint64(len(raw)) != locator.SizeBytes || bytesSHA256(raw) != locator.ContentHash || completion.Files[name] != locator.ContentHash || entry.ContentHash != locator.ContentHash || entry.Size != locator.SizeBytes {
			return fmt.Errorf("pending prior original %s differs from its owner-signed census", name)
		}
	}
	if capture.ClosedAt != result.CompletedAt || capture.CollectedInputsManifest.URI != "final-inputs/manifest.json" || capture.CollectedInputsManifest.SizeBytes != prior.CollectedInputsManifest.SizeBytes || capture.CollectedInputsManifest.ContentHash != prior.CollectedInputsManifest.ContentHash {
		return errors.New("pending prior capture does not bind its original input bytes and completion")
	}
	expected := map[string]FinalArtifactLocator{}
	for _, locator := range inputs.ClosedInputBundles {
		name := strings.TrimSuffix(filepath.Base(locator.URI), ".json")
		if finalSemanticBundleClass(name) != "live-chain" {
			continue
		}
		if locator.URI != "final-inputs/bundles/"+name+".json" {
			return errors.New("pending prior live-chain locator is noncanonical")
		}
		expected[name] = locator
	}
	if len(expected) != len(prior.LiveChainBundles) || len(expected) == 0 {
		return errors.New("pending prior live-chain census differs from the original manifest")
	}
	for _, locator := range prior.LiveChainBundles {
		raw := loaded[locator.URI]
		bundle, err := decodeFinalCollectedFileBundle(raw)
		if err != nil {
			return err
		}
		original, exists := expected[bundle.Name]
		if !exists || original.ContentHash != locator.ContentHash || original.SizeBytes != locator.SizeBytes || uint64(len(raw)) != locator.SizeBytes || bytesSHA256(raw) != locator.ContentHash {
			return errors.New("pending prior live-chain bytes differ from the original closure")
		}
		entry, exists := manifestEntries[original.URI]
		if !exists || completion.Files[original.URI] != original.ContentHash || entry.ContentHash != original.ContentHash || entry.Size != original.SizeBytes {
			return errors.New("pending prior live-chain original lacks its signed manifest binding")
		}
		delete(expected, bundle.Name)
	}
	return nil
}
