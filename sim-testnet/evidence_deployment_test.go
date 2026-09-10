// Exercises the actual generated deployment bytes and approval envelope.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"maps"
	"math/big"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Only deterministic test role material is used; no wallet or RPC is opened.
func validatorEvidenceInstallTest(t *testing.T) (*ResolvedConfig, *RoleSecrets, *DeploymentPayloads) {
	t.Helper()
	cfg := testResolvedConfig(t)
	cfg.Netuid = 521
	roles, err := BuildRoleSecrets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	payloads, err := buildDeploymentPayloads(cfg, roles, 41)
	if err != nil {
		t.Fatal(err)
	}
	if payloads.ValidatorEvidence == nil {
		t.Fatal("generated evidence deployment is absent")
	}
	return cfg, roles, payloads
}

func TestValidatorEvidenceInstallGeneratedConstructorAndRuntime(t *testing.T) {
	_, _, payloads := validatorEvidenceInstallTest(t)
	evidence := payloads.ValidatorEvidence
	artifact := artifactByName("ValidatorEvidence")
	creation := hexBytes(artifact.CreationBytecode)
	deploymentHash := sha256.Sum256([]byte(payloads.Manifest.DeploymentID))
	arguments := append(abiWordAddress(payloads.Manifest.CoordinatorProxy), common.HexToHash(testnetGenesis).Bytes()...)
	arguments = append(arguments, deploymentHash[:]...)
	if !bytes.Equal(evidence.Creation, append(creation, arguments...)) || len(arguments) != 96 {
		t.Fatal("constructor does not bind the exact independently encoded three ABI words")
	}
	if evidence.Manifest.Address != crypto.CreateAddress(payloads.Deployer, 52) || evidence.Manifest.DeployerNonce != 52 {
		t.Fatal("evidence deployment changed an existing CREATE boundary")
	}
	words := map[string][]byte{
		"coordinator": abiWordAddress(payloads.Manifest.CoordinatorProxy), "settlementVault": abiWordAddress(payloads.Manifest.SettlementVault),
		"chainId": new(big.Int).SetUint64(testnetChainID).FillBytes(make([]byte, 32)), "netuid": new(big.Int).SetUint64(521).FillBytes(make([]byte, 32)),
		"genesisHash": common.HexToHash(testnetGenesis).Bytes(), "deploymentIdHash": deploymentHash[:],
	}
	if len(artifact.ImmutableReferences) != len(words) {
		t.Fatal("incomplete immutable census")
	}
	for name, want := range words {
		offsets := artifact.ImmutableReferences[name]
		if len(offsets) == 0 {
			t.Fatalf("missing immutable %s", name)
		}
		for _, offset := range offsets {
			if !bytes.Equal(evidence.Runtime[offset:offset+32], want) {
				t.Fatalf("immutable %s at %d differs", name, offset)
			}
		}
	}
	if crypto.Keccak256Hash(evidence.Runtime) != evidence.Manifest.RuntimeCodeHash {
		t.Fatal("runtime hash mismatch")
	}
}

func TestValidatorEvidenceInstallPreservesCustodyIdentityAcrossNonceRebind(t *testing.T) {
	_, _, payloads := validatorEvidenceInstallTest(t)
	before, err := contractDeploymentIdentityHash(payloads.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	original := payloads.ValidatorEvidence
	if err := configureCoordinatorUpgradeNonce(payloads, 73); err != nil {
		t.Fatal(err)
	}
	after, err := contractDeploymentIdentityHash(payloads.Manifest)
	if err != nil || before != after {
		t.Fatalf("additive evidence changed historical custody identity: %v", err)
	}
	if payloads.ValidatorEvidence.Manifest.DeployerNonce != 75 || payloads.ValidatorEvidence.Manifest.Address != crypto.CreateAddress(payloads.Deployer, 75) || payloads.ValidatorEvidence.Manifest.Address == original.Manifest.Address {
		t.Fatal("evidence did not follow the approved upgrade/batcher nonce sequence")
	}
	if !bytes.Equal(original.Runtime, payloads.ValidatorEvidence.Runtime) || !bytes.Equal(original.Creation, payloads.ValidatorEvidence.Creation) {
		t.Fatal("nonce rebind changed immutable domain")
	}
	if _, exists := payloads.Manifest.RuntimeHashes[payloads.ValidatorEvidence.Manifest.Address.Hex()]; exists {
		t.Fatal("evidence polluted the six-contract historical identity")
	}
	previousRuntime := payloads.ExpectedRuntime
	if err := configureCoordinatorUpgradeNonce(payloads, 81); err != nil {
		t.Fatal(err)
	}
	if payloads.ValidatorEvidence.Manifest.DeployerNonce != 83 || payloads.ValidatorEvidence.Manifest.Address != crypto.CreateAddress(payloads.Deployer, 83) {
		t.Fatal("second rebind lost the additive companion")
	}
	if _, exists := previousRuntime[crypto.CreateAddress(payloads.Deployer, 73)]; !exists {
		t.Fatal("successful rebind mutated a retained prior runtime map")
	}
	if _, exists := payloads.ExpectedRuntime[crypto.CreateAddress(payloads.Deployer, 73)]; exists {
		t.Fatal("second rebind retained a retired implementation runtime")
	}
}

func TestValidatorEvidenceInstallFailedNonceRebindIsAtomic(t *testing.T) {
	_, _, payloads := validatorEvidenceInstallTest(t)
	for _, operation := range []struct {
		name  string
		apply func() error
	}{
		{name: "evidence overflow after upgrade construction", apply: func() error { return configureCoordinatorUpgradeNonce(payloads, ^uint64(0)-1) }},
		{name: "batcher breaks exact predecessor", apply: func() error { return configureFleetBatcherNonce(payloads, payloads.FleetBatcherNonce+1) }},
	} {
		before, err := json.Marshal(payloads)
		if err != nil {
			t.Fatal(err)
		}
		if err := operation.apply(); err == nil {
			t.Fatalf("%s was accepted", operation.name)
		}
		after, err := json.Marshal(payloads)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("%s changed payloads after refusal", operation.name)
		}
	}
	if err := configureCoordinatorUpgradeNonce(payloads, 73); err != nil {
		t.Fatalf("refused rebind poisoned subsequent valid operation: %v", err)
	}
}

func TestValidatorEvidenceInstallProbeCannotAliasCompanion(t *testing.T) {
	_, _, payloads := validatorEvidenceInstallTest(t)
	before, err := json.Marshal(payloads)
	if err != nil {
		t.Fatal(err)
	}
	if err := configurePrecompileProbeNonce(payloads, payloads.ValidatorEvidence.Manifest.DeployerNonce); err == nil {
		t.Fatal("replacement probe aliases the reserved evidence companion")
	}
	after, err := json.Marshal(payloads)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("refused companion collision changed the probe or runtime map")
	}
	if err := configurePrecompileProbeNonce(payloads, 60); err != nil {
		t.Fatalf("separate replacement probe: %v", err)
	}
	if payloads.PrecompileProbeAddress == payloads.ValidatorEvidence.Manifest.Address {
		t.Fatal("accepted separate probe still aliases evidence")
	}
}

func TestValidatorEvidenceInstallSeparatesEveryImmutableDomain(t *testing.T) {
	_, _, original := validatorEvidenceInstallTest(t)
	baseline, err := buildValidatorEvidenceDeployment(original, testnetChainID, common.HexToHash(testnetGenesis), 521)
	if err != nil || baseline == nil || baseline.Manifest != original.ValidatorEvidence.Manifest || !bytes.Equal(baseline.Runtime, original.ValidatorEvidence.Runtime) {
		t.Fatalf("independent unchanged domain does not match the fixture: %v", err)
	}
	for _, field := range []string{"chain", "genesis", "netuid", "coordinator", "vault", "deployment"} {
		payloads := *original
		chain, genesis, netuid := uint64(testnetChainID), common.HexToHash(testnetGenesis), uint16(521)
		switch field {
		case "chain":
			chain++
		case "genesis":
			genesis[0] ^= 1
		case "netuid":
			netuid++
		case "coordinator":
			payloads.Manifest.CoordinatorProxy[19] ^= 1
		case "vault":
			payloads.Manifest.SettlementVault[19] ^= 1
		case "deployment":
			payloads.Manifest.DeploymentID += "-different"
		}
		actual, err := buildValidatorEvidenceDeployment(&payloads, chain, genesis, netuid)
		if err != nil {
			t.Fatalf("valid alternate %s domain: %v", field, err)
		}
		if actual.Manifest.RuntimeCodeHash == original.ValidatorEvidence.Manifest.RuntimeCodeHash {
			t.Fatalf("%s domain was not bound by runtime", field)
		}
		if err := validateValidatorEvidenceDeployment(&actual.Manifest, original.ValidatorEvidence); err == nil {
			t.Fatalf("foreign %s domain accepted", field)
		}
	}
}

func TestValidatorEvidenceInstallRejectsInvalidNonceAndDomain(t *testing.T) {
	_, _, original := validatorEvidenceInstallTest(t)
	for _, change := range []func(*DeploymentPayloads){
		func(p *DeploymentPayloads) { p.FleetBatcherNonce++ },
		func(p *DeploymentPayloads) { p.FleetBatcherAddress[19] ^= 1 },
		func(p *DeploymentPayloads) { p.CoordinatorUpgrade.Implementation[19] ^= 1 },
		func(p *DeploymentPayloads) {
			p.CoordinatorUpgrade.DeployerNonce = ^uint64(0) - 1
			p.FleetBatcherNonce = ^uint64(0)
		},
		func(p *DeploymentPayloads) { p.Manifest.DeploymentID = "" },
		func(p *DeploymentPayloads) { p.Manifest.CoordinatorProxy = p.Manifest.SettlementVault },
		func(p *DeploymentPayloads) { p.Deployer = common.Address{} },
	} {
		payloads := *original
		change(&payloads)
		if actual, err := buildValidatorEvidenceDeployment(&payloads, testnetChainID, common.HexToHash(testnetGenesis), 521); err == nil || actual != nil {
			t.Fatal("invalid evidence deployment accepted")
		}
	}
	for _, domain := range []struct {
		chain   uint64
		genesis common.Hash
		netuid  uint16
	}{
		{chain: 0, genesis: common.HexToHash(testnetGenesis), netuid: 521}, {chain: testnetChainID, netuid: 521}, {chain: testnetChainID, genesis: common.HexToHash(testnetGenesis)},
	} {
		if actual, err := buildValidatorEvidenceDeployment(original, domain.chain, domain.genesis, domain.netuid); err == nil || actual != nil {
			t.Fatal("incomplete evidence domain accepted")
		}
	}
}

func TestValidatorEvidenceInstallTransactionEnvelopeRejectsEveryDrift(t *testing.T) {
	_, roles, payloads := validatorEvidenceInstallTest(t)
	owner, err := roles.EVMAddress("testnet-owner")
	if err != nil {
		t.Fatal(err)
	}
	evidence := payloads.ValidatorEvidence
	parameters, err := evidence.actionParameters(validatorEvidenceDeployActionID, owner)
	if err != nil {
		t.Fatal(err)
	}
	action := Action{ID: validatorEvidenceDeployActionID, Kind: "evm-transaction", Target: evidence.Manifest.Address.Hex(), Parameters: parameters}
	if err := validateApprovedEVMTransactionFields(action, payloads.Deployer, evidence.Manifest.DeployerNonce, nil, new(big.Int), evidence.Creation); err != nil {
		t.Fatal(err)
	}
	for key := range parameters {
		// Plan/source metadata is checked by the independently rebuilt plan,
		// not by a transaction-only decoder with no artifact input.
		if key == validatorEvidenceManifestParameter || key == validatorEvidenceArtifactParameter || key == "validator_evidence" || key == "coordinator" || key == "runtime_code_hash" {
			continue
		}
		changed := action
		changed.Parameters = maps.Clone(parameters)
		changed.Parameters[key] = "invalid"
		if err := validateApprovedEVMTransactionFields(changed, payloads.Deployer, evidence.Manifest.DeployerNonce, nil, new(big.Int), evidence.Creation); err == nil {
			t.Fatalf("%s drift accepted", key)
		}
	}
	anchor, err := evidence.actionParameters(validatorEvidenceAnchorActionID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if anchor[validatorEvidenceManifestParameter] != parameters[validatorEvidenceManifestParameter] || anchor["anchor_owner"] != owner.Hex() || anchor["expected_nonce"] != "" {
		t.Fatal("anchor did not preserve separate owner nonce authority")
	}
}

// Builds the real approval graph, not a hand-assembled action subset.
func validatorEvidenceInstallPlanTest(t *testing.T) *SetupPlan {
	t.Helper()
	cfg := testResolvedConfig(t)
	roles, err := derivePublicRoles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildPlan(cfg, testSetupFacts(), roles, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePlanBudget(plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestValidatorEvidenceInstallPlanRequiresDeploymentAnchorAndBudget(t *testing.T) {
	plan := validatorEvidenceInstallPlanTest(t)
	if plan.Schema != currentSetupPlanSchema || plan.ValidatorEvidence == nil {
		t.Fatal("current plan lacks evidence")
	}
	byID := map[string]Action{}
	for _, action := range plan.Actions {
		byID[action.ID] = action
	}
	for _, id := range []string{validatorEvidenceDeployActionID, validatorEvidenceAnchorActionID} {
		if byID[id].Spend.EVMGasWei == "" || byID[id].Spend.EVMGasWei == "0" || byID[id].Parameters[evmMaximumGasUnitsParameter] == "" {
			t.Fatalf("%s has no approved gas", id)
		}
	}
	if !slices.Contains(byID["config.render"].DependsOn, validatorEvidenceAnchorActionID) || slices.Contains(byID["fleet.refresh.deploy-batcher"].DependsOn, "campaign.voluntary-conviction.1") {
		t.Fatal("startup evidence nonce graph has a late campaign dependency")
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"validator_evidence"`)) {
		t.Fatal("public plan omits companion identity")
	}
}

func TestValidatorEvidenceInstallPlanRejectsMissingActionsAndAuthorityDrift(t *testing.T) {
	plan := validatorEvidenceInstallPlanTest(t)
	if err := validateValidatorEvidencePlan(plan); err != nil {
		t.Fatalf("complete generated plan prerequisite: %v", err)
	}
	for _, field := range []string{"manifest", "address", "runtime", "deploy-action", "anchor-action", "anchor-owner", "render-dependency", "nonce", "deploy-source-artifact", "anchor-source-artifact"} {
		encoded, err := json.Marshal(plan)
		if err != nil {
			t.Fatal(err)
		}
		var changed SetupPlan
		if err := json.Unmarshal(encoded, &changed); err != nil {
			t.Fatal(err)
		}
		switch field {
		case "manifest":
			changed.ValidatorEvidence = nil
		case "address":
			changed.ValidatorEvidence.Address[19] ^= 1
		case "runtime":
			changed.ValidatorEvidence.RuntimeCodeHash[0] ^= 1
		default:
			for index := range changed.Actions {
				action := &changed.Actions[index]
				switch {
				case field == "deploy-action" && action.ID == validatorEvidenceDeployActionID:
					action.ID += "-missing"
				case field == "anchor-action" && action.ID == validatorEvidenceAnchorActionID:
					action.ID += "-missing"
				case field == "anchor-owner" && action.ID == validatorEvidenceAnchorActionID:
					action.Parameters["anchor_owner"] = plan.Roles.Deployer
				case field == "render-dependency" && action.ID == "config.render":
					action.DependsOn = nil
				case field == "nonce" && action.ID == validatorEvidenceDeployActionID:
					action.Parameters["expected_nonce"] = strconv.FormatUint(plan.ValidatorEvidence.DeployerNonce+1, 10)
				case field == "deploy-source-artifact" && action.ID == validatorEvidenceDeployActionID,
					field == "anchor-source-artifact" && action.ID == validatorEvidenceAnchorActionID:
					digest, err := decodeHex32("fixture source artifact", action.Parameters[validatorEvidenceArtifactParameter])
					if err != nil {
						t.Fatal(err)
					}
					digest[0] ^= 1
					action.Parameters[validatorEvidenceArtifactParameter] = common.Hash(digest).Hex()
				}
			}
		}
		if err := validateValidatorEvidencePlan(&changed); err == nil {
			t.Fatalf("%s drift accepted", field)
		}
	}
}

func TestValidatorEvidenceInstallHistoricalPlanDoesNotGainAnUnsignedJournal(t *testing.T) {
	plan := validatorEvidenceInstallPlanTest(t)
	plan.Schema = setupPlanSchemaV11
	if err := validateValidatorEvidencePlan(plan); err == nil {
		t.Fatal("v11 plan accepted unsigned evidence extension")
	}
	plan.ValidatorEvidence = nil
	plan.ValidatorEvidenceSource, plan.ValidatorEvidenceCarry = nil, nil
	plan.Actions = slices.DeleteFunc(plan.Actions, func(action Action) bool {
		return action.ID == validatorEvidenceDeployActionID || action.ID == validatorEvidenceAnchorActionID
	})
	if err := validateValidatorEvidencePlan(plan); err != nil {
		t.Fatalf("historical evidence-free approval cannot be read: %v", err)
	}
	if !planUsesRuntimeConfigIdentityEnvelope(setupPlanSchemaV11) || !planUsesRevisionRecoveryEnvelope(setupPlanSchemaV11) {
		t.Fatal("v12 dropped historical v11 guards")
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"validator_evidence":`)) {
		t.Fatal("nil extension changed historical wire shape")
	}
}

// Historical v11 approvals have no journal authority to carry. A first v12
// candidate can still acquire and rebind its first companion before approval.
func TestValidatorEvidenceInstallV11RevisionCanAcquireAndRebindFirstJournal(t *testing.T) {
	cfg := testResolvedConfig(t)
	roles, err := derivePublicRoles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := buildPlan(cfg, testSetupFacts(), roles, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	prior.Schema, prior.ValidatorEvidence = setupPlanSchemaV11, nil
	prior.ValidatorEvidenceSource, prior.ValidatorEvidenceCarry = nil, nil
	prior.Actions = slices.DeleteFunc(prior.Actions, func(action Action) bool {
		return action.ID == validatorEvidenceDeployActionID || action.ID == validatorEvidenceAnchorActionID
	})
	originalRenderDependencies := slices.Clone(actionByID(t, prior, "config.render").DependsOn)
	for index := range prior.Actions {
		action := &prior.Actions[index]
		// The real plan's campaign reserve and renderer share a setup-prefix
		// backing array. A fixture rebind must own its slice before filtering.
		action.DependsOn = slices.DeleteFunc(slices.Clone(action.DependsOn), func(id string) bool {
			return id == validatorEvidenceDeployActionID || id == validatorEvidenceAnchorActionID
		})
		if action.ID == "campaign.evm-gas-reserve" && !slices.Equal(actionByID(t, prior, "config.render").DependsOn, originalRenderDependencies) {
			t.Fatal("v11 campaign dependency filtering mutated the retained renderer prefix")
		}
		action.IntentHash, err = actionIntentHash(*action)
		if err != nil {
			t.Fatal(err)
		}
	}
	prior.MaximumSpend, err = maximumActionSpend(prior.Actions)
	if err != nil {
		t.Fatal(err)
	}
	prior.PlanHash, err = prior.hash()
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePlanBudget(prior); err != nil {
		t.Fatalf("historical approval prerequisite: %v", err)
	}
	revised, err := buildPlanRevisionFromFacts(cfg, t.TempDir(), prior, partialRevisionFacts(t, cfg, 0), nil, time.Unix(2, 0))
	if err != nil || revised == nil || revised.ValidatorEvidence == nil {
		t.Fatalf("v11 first companion revision: %v", err)
	}
	secrets, err := BuildRoleSecrets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	payloads, err := buildDeploymentPayloads(cfg, secrets, revised.Deployment.InitialNonce)
	if err != nil {
		t.Fatal(err)
	}
	if err := configureCoordinatorUpgradeNonce(payloads, revised.CoordinatorUpgrade.DeployerNonce+3); err != nil {
		t.Fatal(err)
	}
	if err := rebindPlanCoordinatorUpgrade(revised, payloads); err != nil {
		t.Fatalf("fresh v12 candidate was mistaken for installed history: %v", err)
	}
	if revised.ValidatorEvidence == nil || *revised.ValidatorEvidence != payloads.ValidatorEvidence.Manifest {
		t.Fatal("first-journal rebind did not retain exact payload authority")
	}
}

// Until retained source/runtime/receipt carry is implemented, a validated
// prior v12 approval cannot be silently redirected to another CREATE.
func TestValidatorEvidenceInstallV12RevisionRefusesCompanionRelocationBeforeMutation(t *testing.T) {
	cfg := testResolvedConfig(t)
	roles, err := derivePublicRoles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := buildPlan(cfg, testSetupFacts(), roles, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	current := partialRevisionFacts(t, cfg, 0)
	continued, err := buildPlanRevisionFromFacts(cfg, stateDir, prior, current, nil, time.Unix(2, 0))
	if err != nil || continued == nil || *continued.ValidatorEvidence != *prior.ValidatorEvidence {
		t.Fatalf("same-companion continuation prerequisite: %v", err)
	}
	before, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := BuildRoleSecrets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	payloads, err := buildDeploymentPayloads(cfg, secrets, prior.Deployment.InitialNonce)
	if err != nil {
		t.Fatal(err)
	}
	if err := configureCoordinatorUpgradeNonce(payloads, prior.CoordinatorUpgrade.DeployerNonce+3); err != nil {
		t.Fatal(err)
	}
	migration := &coordinatorUpgradeMigration{Deployment: prior.Deployment, Upgrade: payloads.CoordinatorUpgrade}
	actual, err := buildPlanRevisionFromFactsWithMigration(cfg, stateDir, prior, current, nil, time.Unix(3, 0), migration)
	if err == nil || actual != nil || !strings.Contains(err.Error(), "requires authenticated existing-companion carry") {
		t.Fatalf("approved v12 companion silently moved: plan=%v error=%v", actual != nil, err)
	}
	after, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("refused revision mutated the authenticated prior approval")
	}
	files, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatal("read-only revision refusal wrote state before companion carry was authenticated")
	}
}
