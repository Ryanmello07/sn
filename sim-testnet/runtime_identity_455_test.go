// Runtime455 source provenance and artifact admission preserve historical
// decoding without inheriting a tag or mainnet proposal from runtime454.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// Independent literals make a changed source constant fail the admission test.
func runtime455ReviewedTestLock() *ReleaseLock {
	return &ReleaseLock{SchemaVersion: 1, Release: "1.0", Runtime: ReleaseRuntimeLock{
		SourceRepository: "https://github.com/RaoFoundation/subtensor",
		SourceRefKind:    "commit", SourceRefName: "67dcf7f791dc495064c293f080a0702cb433e51e",
		SourceCommit:         "67dcf7f791dc495064c293f080a0702cb433e51e",
		CodeHash:             "0xbca85925668cabb2880164610d64eda2e4d9bf2777994f9cdfdb9d36253ce74a",
		MetadataHash:         "0x16da562c347a354c55eb1ad5cd5094343afe7acdc12e5b526bf6c8cb12e866bc",
		CompressedWasmSHA256: "0x232bfc0d65ec2dbe4280b152e23f13879df9692d2286dd08c6ba14483deee00f",
		SpecVersion:          455, TransactionVersion: 1, StateVersion: 1,
	}}
}

// Exact commit provenance cannot be expressed as a mutable branch, invented
// release tag or copied mainnet multisig proposal/timepoint.
func TestRuntime455CurrentLockSeparatesCommitFromMainnetProposal(t *testing.T) {
	lock := runtime455ReviewedTestLock()
	if err := validateReviewedRuntimeIdentity(lock); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ReleaseRuntimeLock){
		func(value *ReleaseRuntimeLock) { value.SourceRefKind = "branch" },
		func(value *ReleaseRuntimeLock) { value.SourceRefName = "testnet" },
		func(value *ReleaseRuntimeLock) { value.SourceRefKind, value.SourceRefName = "", "" },
		func(value *ReleaseRuntimeLock) { value.SourceTag = "v455" },
		func(value *ReleaseRuntimeLock) { value.SourceCommit = "14cde6410fe8ec81a940e290c56f94a632a0988d" },
		func(value *ReleaseRuntimeLock) {
			value.UpstreamReleaseCallHash = "0xa555b212406469b24d3a370ac59675bad303e319274ebd7a7fd0804dede3315b"
		},
		func(value *ReleaseRuntimeLock) { value.UpstreamReleaseTimepoint = "8996567:7" },
	} {
		mutated := *lock
		mutate(&mutated.Runtime)
		if err := validateReviewedRuntimeIdentity(&mutated); err == nil {
			t.Fatalf("unreviewed/fabricated provenance was accepted: %+v", mutated.Runtime)
		}
	}
}

// The new optional reference fields round-trip current locks and stay absent
// from historical bytes. Old tag/proposal fields retain their literal meaning.
func TestRuntime455CanonicalLockRetainsHistoricalProvenance(t *testing.T) {
	current := runtime455ReviewedTestLock()
	historical := &ReleaseLock{SchemaVersion: 1, Release: "1.0", Runtime: ReleaseRuntimeLock{
		SourceRepository: "https://github.com/RaoFoundation/subtensor", SourceTag: "v454",
		SourceCommit:            "14cde6410fe8ec81a940e290c56f94a632a0988d",
		CodeHash:                "0x725e3d1eca8d5c29c1f0fa6476d5360661b852f52aebad979d6636e227a431ef",
		MetadataHash:            "0x4d17516b694ef8d18f8a565dcb2df0117e7a0018a3ffa40812c91a1621225702",
		CompressedWasmSHA256:    "0xa55e76b4f4620bcdb4c787e499c87a35abb9913ba4cde001b08a00d1945ac4db",
		UpstreamReleaseCallHash: "0x5a1c30f0387796da59522d4b84a71395533a4ee676e06c52eedb14262ae9c3c6", UpstreamReleaseTimepoint: "8996567:7",
		SpecVersion: 454, TransactionVersion: 1, StateVersion: 1,
	}}
	for _, lock := range []*ReleaseLock{current, historical} {
		raw, err := yaml.Marshal(lock)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "release.lock.yml")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		var decoded ReleaseLock
		if err := strictYAML(path, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Runtime != lock.Runtime {
			t.Fatalf("runtime provenance changed through strict decoding: %+v", decoded.Runtime)
		}
		encoded, err := json.Marshal(decoded.Runtime)
		if err != nil {
			t.Fatal(err)
		}
		if lock == historical && (bytes.Contains(raw, []byte("source_ref_")) || bytes.Contains(encoded, []byte("source_ref_"))) {
			t.Fatal("empty new source fields changed historical wire shape")
		}
	}
	if err := validateReviewedRuntimeIdentity(historical); err == nil {
		t.Fatal("historical454 lock was reinterpreted as current455")
	}
}

// Read-only public history admits each exact reviewed predecessor. The same
// object cannot become the current launch identity, even with a mutated config.
func TestRuntime455HistoricalPublicationsRemainEvidenceOnly(t *testing.T) {
	cfg := testResolvedConfig(t)
	artifacts, err := releaseHistoryRuntimeArtifacts(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 5 || artifacts[0].Version.SpecVersion != 455 {
		t.Fatal("current455 plus complete451–454 history is absent")
	}
	for _, artifact := range artifacts {
		public := &PublicDeploymentManifest{RuntimeSpec: artifact.Version.SpecVersion, TransactionVersion: artifact.Version.TransactionVersion, StateVersion: artifact.Version.StateVersion, RuntimeCodeHash: artifact.CodeHash, RuntimeMetadataHash: artifact.MetadataHash}
		if err := validatePublishedRuntimeIdentityShape(public); err != nil {
			t.Fatalf("reviewed history%d refused: %v", artifact.Version.SpecVersion, err)
		}
		currentErr := validatePublishedRuntimeIdentity(public, cfg)
		if artifact.Version.SpecVersion == 455 {
			if currentErr != nil {
				t.Fatal(currentErr)
			}
			continue
		}
		if currentErr == nil {
			t.Fatalf("historical%d became current authority", artifact.Version.SpecVersion)
		}
		historicalCfg, historicalLock, historicalPublic := *cfg, *cfg.Release, *cfg.Public
		historicalLock.Runtime.SpecVersion, historicalLock.Runtime.CodeHash, historicalLock.Runtime.MetadataHash = artifact.Version.SpecVersion, artifact.CodeHash, artifact.MetadataHash
		historicalPublic.Chain.ExpectedRuntimeSpec = artifact.Version.SpecVersion
		historicalCfg.Release, historicalCfg.Public = &historicalLock, &historicalPublic
		if err := validatePublishedRuntimeIdentity(public, &historicalCfg); err == nil {
			t.Fatal("mutated historical config bypassed current lock admission")
		}
	}
}

// Version selection never permits another reviewed artifact's code or metadata
// hash, a future version, or a changed transaction/state domain.
func TestRuntime455PublicationsRejectCrossArtifactPairs(t *testing.T) {
	artifacts, err := releaseHistoryRuntimeArtifacts(testResolvedConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	for index, artifact := range artifacts {
		public := PublicDeploymentManifest{RuntimeSpec: artifact.Version.SpecVersion, TransactionVersion: 1, StateVersion: 1, RuntimeCodeHash: artifact.CodeHash, RuntimeMetadataHash: artifact.MetadataHash}
		for otherIndex, other := range artifacts {
			if otherIndex == index {
				continue
			}
			wrongCode, wrongMetadata := public, public
			wrongCode.RuntimeCodeHash, wrongMetadata.RuntimeMetadataHash = other.CodeHash, other.MetadataHash
			if validatePublishedRuntimeIdentityShape(&wrongCode) == nil || validatePublishedRuntimeIdentityShape(&wrongMetadata) == nil {
				t.Fatalf("artifact%d accepted hashes from%d", artifact.Version.SpecVersion, other.Version.SpecVersion)
			}
		}
		for _, mutate := range []func(*PublicDeploymentManifest){
			func(value *PublicDeploymentManifest) { value.RuntimeSpec = 456 },
			func(value *PublicDeploymentManifest) { value.TransactionVersion = 2 },
			func(value *PublicDeploymentManifest) { value.StateVersion = 2 },
		} {
			mutated := public
			mutate(&mutated)
			if err := validatePublishedRuntimeIdentityShape(&mutated); err == nil {
				t.Fatal("unreviewed public runtime domain was accepted")
			}
		}
	}
}
