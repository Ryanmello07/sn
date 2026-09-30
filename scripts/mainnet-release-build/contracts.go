// Preserve retained transaction bytes beside independently rebuilt artifacts.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type buildContract struct {
	Name                   string `json:"name"`
	FoundryArtifactId      string `json:"foundry_artifact_id"`
	FoundryArtifactSha256  string `json:"foundry_artifact_sha256"`
	RetainedCreationSha256 string `json:"retained_creation_sha256"`
	RetainedRuntimeSha256  string `json:"retained_runtime_sha256"`
	RebuiltCreationSha256  string `json:"rebuilt_creation_sha256"`
	RebuiltRuntimeSha256   string `json:"rebuilt_runtime_sha256"`
	RetainedRuntimeHash    string `json:"retained_runtime_hash"`
	RetainedArtifactHash   string `json:"retained_artifact_hash"`
	StorageLayoutHash      string `json:"storage_layout_hash"`
	CreationBytes          int    `json:"creation_bytes"`
	RuntimeBytes           int    `json:"runtime_bytes"`
	ExactBytes             bool   `json:"exact_bytes"`
}

type releaseContract struct {
	Name              string `json:"name"`
	Abi               string `json:"abi"`
	Creation          string `json:"creation"`
	Runtime           string `json:"runtime"`
	RuntimeHash       string `json:"runtime_hash"`
	ArtifactHash      string `json:"artifact_hash"`
	StorageLayoutHash string `json:"storage_layout_hash"`
}

func releaseContractPaths() map[string]string {
	return map[string]string{"ReserveSink": "STReserveSink.sol/STReserveSink.json", "SettlementVault": "STSettlementVault.sol/STSettlementVault.json", "Coordinator": "STCoordinator.sol/STCoordinator.json", "ValidatorEvidence": "STValidatorEvidence.sol/STValidatorEvidence.json", "ERC1967Proxy": "ERC1967Proxy.sol/ERC1967Proxy.json"}
}

// Placeholder links, empty bytecode and EVM size violations cannot be sealed.
func hashBuildBytecode(value string, maximum int) (string, int, error) {
	value = strings.TrimPrefix(value, "0x")
	if len(value) == 0 || len(value) > 2*maximum {
		return "", 0, errors.New("contract bytecode exceeds deployment bound")
	}
	raw, err := hex.DecodeString(value)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(hash[:]), len(raw), nil
}

// The existing generator checks source, ABI, layout, immutables and permitted
// metadata drift before this function binds its retained bytes to fresh output.
func captureBuildContracts(config buildConfig, exportPath string) ([]buildContract, []buildArtifact, error) {
	retained, err := readBuildInput(filepath.Join(config.Workspace, "sn/sim-testnet/contracts_gen.go"), 4*1024*1024)
	if err != nil {
		return nil, nil, err
	}
	constants, err := retainedContractConstants(retained)
	if err != nil {
		return nil, nil, err
	}
	raw, err := readBuildInput(exportPath, 4*1024*1024)
	if err != nil {
		return nil, nil, err
	}
	var envelope struct {
		Schema    string            `json:"schema"`
		Artifacts []releaseContract `json:"artifacts"`
	}
	if err := validateBuildJson(raw); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, nil, err
	}
	wanted := releaseContractPaths()
	if envelope.Schema != "urnetwork-contract-release-artifacts-v1" || len(envelope.Artifacts) != len(wanted) {
		return nil, nil, errors.New("contract export census differs")
	}
	result := []buildContract{}
	artifacts := []buildArtifact{}
	for _, contract := range envelope.Artifacts {
		if err := validateRetainedContract(contract, constants); err != nil {
			return nil, nil, err
		}
		path, ok := wanted[contract.Name]
		if !ok {
			return nil, nil, errors.New("duplicate or unknown release contract")
		}
		delete(wanted, contract.Name)
		artifact, err := retainBuildInput(config, "contract-"+contract.Name+".json", "foundry-contract", filepath.Join(config.Workspace, "sn/evm/out", path))
		if err != nil {
			return nil, nil, err
		}
		artifacts = append(artifacts, artifact)
		fresh, err := readBuildInput(filepath.Join(config.Output, artifact.Path), 16*1024*1024)
		if err != nil {
			return nil, nil, err
		}
		var foundry struct {
			Bytecode struct {
				Object string `json:"object"`
			} `json:"bytecode"`
			DeployedBytecode struct {
				Object string `json:"object"`
			} `json:"deployedBytecode"`
		}
		if err := json.Unmarshal(fresh, &foundry); err != nil {
			return nil, nil, err
		}
		entry := buildContract{Name: contract.Name, FoundryArtifactId: artifact.Id, FoundryArtifactSha256: artifact.Sha256, RetainedRuntimeHash: contract.RuntimeHash, RetainedArtifactHash: contract.ArtifactHash, StorageLayoutHash: contract.StorageLayoutHash}
		entry.RetainedCreationSha256, entry.CreationBytes, err = hashBuildBytecode(contract.Creation, 49152)
		if err != nil {
			return nil, nil, fmt.Errorf("%s retained creation: %w", contract.Name, err)
		}
		entry.RetainedRuntimeSha256, entry.RuntimeBytes, err = hashBuildBytecode(contract.Runtime, 24576)
		if err != nil {
			return nil, nil, fmt.Errorf("%s retained runtime: %w", contract.Name, err)
		}
		entry.RebuiltCreationSha256, _, err = hashBuildBytecode(foundry.Bytecode.Object, 49152)
		if err != nil {
			return nil, nil, err
		}
		entry.RebuiltRuntimeSha256, _, err = hashBuildBytecode(foundry.DeployedBytecode.Object, 24576)
		if err != nil {
			return nil, nil, err
		}
		entry.ExactBytes = entry.RetainedCreationSha256 == entry.RebuiltCreationSha256 && entry.RetainedRuntimeSha256 == entry.RebuiltRuntimeSha256
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, artifacts, nil
}

// An export may choose fresh bytes after a real executable change. Composition
// must refuse that choice until a successor reviewed binding is checked in.
func retainedContractConstants(raw []byte) (map[string]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "retained-contracts.go", raw, 0)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		for _, spec := range group.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}
			name := value.Names[0].Name
			if _, ok := result[name]; ok {
				return nil, errors.New("duplicate retained contract constant")
			}
			decoded, err := strconv.Unquote(literal.Value)
			if err != nil {
				return nil, err
			}
			result[name] = decoded
		}
	}
	return result, nil
}

func validateRetainedContract(contract releaseContract, constants map[string]string) error {
	for suffix, actual := range map[string]string{"ABI": contract.Abi, "CreationBytecode": contract.Creation, "RuntimeBytecode": contract.Runtime, "RuntimeBytecodeHash": contract.RuntimeHash, "FoundryArtifactHash": contract.ArtifactHash, "StorageLayoutHash": contract.StorageLayoutHash} {
		retained, ok := constants[contract.Name+suffix]
		if !ok || retained == "" || actual != retained {
			return fmt.Errorf("%s export differs from retained %s; a successor binding requires review", contract.Name, suffix)
		}
	}
	return nil
}

// A metadata-tolerant generator success is never promoted to byte equality.
func validateContractCensus(contracts []buildContract, artifacts map[string]buildArtifact, exact bool) error {
	wanted := releaseContractPaths()
	if len(contracts) != len(wanted) {
		return errors.New("release requires all five production contracts")
	}
	allExact := true
	for _, contract := range contracts {
		if _, ok := wanted[contract.Name]; !ok {
			return errors.New("duplicate or unknown production contract")
		}
		delete(wanted, contract.Name)
		artifact, ok := artifacts[contract.FoundryArtifactId]
		if !ok || artifact.Kind != "foundry-contract" || artifact.Sha256 != contract.FoundryArtifactSha256 {
			return errors.New("contract source artifact binding differs")
		}
		isExact := contract.RetainedCreationSha256 == contract.RebuiltCreationSha256 && contract.RetainedRuntimeSha256 == contract.RebuiltRuntimeSha256
		if contract.RetainedCreationSha256 == "" || contract.RetainedRuntimeSha256 == "" || contract.RebuiltCreationSha256 == "" || contract.RebuiltRuntimeSha256 == "" || contract.ExactBytes != isExact || contract.CreationBytes <= 0 || contract.CreationBytes > 49152 || contract.RuntimeBytes <= 0 || contract.RuntimeBytes > 24576 {
			return errors.New("contract bytecode identity, equality or size differs")
		}
		allExact = allExact && isExact
	}
	if exact != allExact {
		return errors.New("source-to-bytecode equality claim differs")
	}
	return nil
}
