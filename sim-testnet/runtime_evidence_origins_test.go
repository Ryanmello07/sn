//go:build linux || darwin

// Renderer admission and the real release-loader round trip keep the complete
// operator origin census identical to the public proof publisher's census.
package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRuntimeEvidenceV2OriginsAdmitCanonicalHttpsAndExactTestnetLoopback(t *testing.T) {
	for _, origins := range [][]string{{"https://no1.example", "https://no2.example"}, {"http://127.0.0.1:18081", "http://127.0.0.1:18082"}} {
		cfg := testResolvedConfig(t)
		cfg.OperatorAPIOrigins = append([]string(nil), origins...)
		if err := validateRuntimeOperatorApiOrigins(cfg); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(cfg.OperatorAPIOrigins, origins) {
			t.Fatal("origin validation rewrote approved input")
		}
	}
}

func TestRuntimeEvidenceV2OriginsRefuseBadCensusBeforeBothRenderersWrite(t *testing.T) {
	for _, origins := range [][]string{
		nil,
		{"https://no1.example"},
		{"https://same.example", "https://same.example"},
		{"https://no1.example/path", "https://no2.example"},
		{"https://NO1.example/", "https://no2.example"},
		{"http://external.example", "https://no2.example"},
		{"http://127.0.0.1:18082", "http://127.0.0.1:18081"},
	} {
		cfg := testResolvedConfig(t)
		cfg.Public.Chain.EVMPublicReadEndpoint = "https://test.chain.opentensor.ai"
		stateDir := t.TempDir()
		configureRuntimeEvidenceV2Test(t, cfg, stateDir)
		deployment := ContractDeployment{Schema: "urnetwork-contract-deployment-v1", DeploymentID: cfg.Config.Deployment.DeploymentID,
			CoordinatorProxy: common.Address{1}, SettlementVault: common.Address{2}, DeployBlock: 123, DeployBlockHash: common.Hash{3}.Hex(),
			CoordinatorEventStartBlock: 100, CoordinatorEventStartBlockHash: common.Hash{4}.Hex()}
		if err := saveContractDeployment(stateDir, deployment); err != nil {
			t.Fatal(err)
		}
		cfg.OperatorAPIOrigins = append([]string(nil), origins...)
		before := validatorNamespaceTreeSnapshot(t, stateDir)
		for _, render := range []func() error{
			func() error { return RenderRuntimeConfigs(cfg, stateDir, nil) },
			func() error { return renderValidatorMinerConfigs(cfg, stateDir, nil, &deployment) },
		} {
			if err := render(); err == nil || !strings.Contains(err.Error(), "origin") {
				t.Fatalf("renderer did not reject origin census before writes: %v", err)
			}
			if !reflect.DeepEqual(before, validatorNamespaceTreeSnapshot(t, stateDir)) {
				t.Fatal("invalid origin changed runtime state")
			}
		}
	}
}
