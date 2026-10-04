// Provider launch files derive complete signing domains from the admitted
// installation graph. This optional local export does not revise preparation.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/urfoundation/sn/protocol"
	"github.com/urfoundation/sn/validator"
)

const bootstrapProviderRoleSchema = "urnetwork-mainnet-provider-role-config-v1"

// Host inventory supplies an explicit operator selection and retained provider
// state path. It supplies no domain override or new client registration grant.
type bootstrapProviderRoleRequest struct {
	Schema          string                      `json:"schema"`
	PreparationHash string                      `json:"preparation_hash"`
	Providers       []bootstrapProviderRoleHost `json:"providers"`
}

// Route and identity selection are independent. Both routes must match the
// selected operator in every original signed validator declaration.
type bootstrapProviderRoleHost struct {
	Id             string `json:"id"`
	NoId           uint64 `json:"no_id"`
	ApiUrl         string `json:"api_url"`
	ConnectUrl     string `json:"connect_url"`
	StateDirectory string `json:"state_directory"`
}

// Arguments are passed as a vector, never interpreted as shell code. The file
// is directly consumed by the existing provide/auth-provide domain option.
type bootstrapProviderRoleLaunch struct {
	Host             bootstrapProviderRoleHost       `json:"host"`
	Domain           protocol.ClientKeyHistoryDomain `json:"domain"`
	DomainHash       string                          `json:"domain_hash"`
	DomainFile       planFileReference               `json:"domain_file"`
	Arguments        []string                        `json:"arguments"`
	Environment      map[string]string               `json:"environment"`
	EnrollmentDomain protocol.ClientKeyHistoryDomain `json:"client_key_enrollment_domain"`
}

// These declarations do not prove installed chain state, registration, full
// work coverage or financial conformance. Existing role activation stays separate.
type bootstrapProviderRoleConfig struct {
	Schema               string                        `json:"schema"`
	PreparationHash      string                        `json:"preparation_hash"`
	ContractRolePlanHash string                        `json:"contract_role_plan_hash"`
	Request              planFileReference             `json:"request"`
	ProviderDeclarations []bootstrapProviderRoleLaunch `json:"provider_declarations"`
	ActivationReady      bool                          `json:"activation_ready"`
	NetworkEffects       bool                          `json:"network_effects"`
	ContentHash          string                        `json:"content_hash"`
}

// Every original input is reread and content checked before any output file is
// published. No pair of mutually matching foreign declarations can select a domain.
func loadBootstrapProviderRoleConfig(ctx context.Context, configPath, requestPath, outputDirectory string) (bootstrapProviderRoleConfig, error) {
	var result bootstrapProviderRoleConfig
	preparation, err := loadBootstrapChainPreparation(ctx, configPath)
	if err != nil {
		return result, err
	}
	roles, err := loadBootstrapContractRolePlan(ctx, configPath)
	if err != nil || roles.PreparationHash != preparation.Plan.ContentHash {
		return result, errors.Join(errors.New("provider launch original preparation or contract-role graph differs"), err)
	}
	raw, requestHash, err := readBootstrapRootFile(ctx, requestPath, 256*1024)
	if err != nil {
		return result, err
	}
	var request bootstrapProviderRoleRequest
	if err := decodePlanJson(raw, &request); err != nil {
		return result, err
	}
	if request.Schema != bootstrapProviderRoleSchema || request.PreparationHash != preparation.Plan.ContentHash || len(request.Providers) == 0 || len(request.Providers) > 64 || !bootstrapRootAbsolutePath(outputDirectory) {
		return result, errors.New("provider launch requires the exact original preparation, bounded hosts and absolute output directory")
	}
	reserved := []string{configPath, requestPath, preparation.Plan.Config.RunDirectory}
	for _, inspection := range preparation.Plan.ValidatorInspections {
		reserved = append(reserved, inspection.DeclaredPaths...)
	}
	for _, host := range request.Providers {
		reserved = append(reserved, host.StateDirectory)
	}
	for _, path := range reserved {
		if path == outputDirectory || strings.HasPrefix(path, outputDirectory+string(filepath.Separator)) || strings.HasPrefix(outputDirectory, path+string(filepath.Separator)) {
			return result, errors.New("provider domain output overlaps original configuration, preparation or identity custody")
		}
	}
	declared := map[uint64]validator.ProductionProviderDomain{}
	for index, original := range preparation.Plan.Config.Validators {
		configRaw, err := readBootstrapChainInput(ctx, original.Config, 2*1024*1024)
		if err != nil {
			return result, err
		}
		domains, err := validator.InspectProductionProviderDomains(ctx, original.Config.Path, configRaw)
		if err != nil {
			return result, err
		}
		current := map[uint64]validator.ProductionProviderDomain{}
		for _, domain := range domains {
			if _, exists := current[domain.NoId]; exists {
				return result, errors.New("provider launch operator declaration is duplicated")
			}
			current[domain.NoId] = domain
			if index == 0 {
				declared[domain.NoId] = domain
			} else if declared[domain.NoId] != domain {
				return result, errors.New("provider launch signed operator domain or routes disagree")
			}
		}
		if len(current) != len(declared) {
			return result, errors.New("provider launch signed operator census differs")
		}
	}
	ids, states := map[string]bool{}, map[string]bool{}
	for _, host := range request.Providers {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`).MatchString(host.Id) || ids[host.Id] || !bootstrapRootAbsolutePath(host.StateDirectory) || states[host.StateDirectory] {
			return result, errors.New("provider launch identities and absolute retained state directories must be distinct")
		}
		ids[host.Id], states[host.StateDirectory] = true, true
		op, exists := declared[host.NoId]
		if !exists || host.ApiUrl != op.ApiUrl || host.ConnectUrl != op.ConnectUrl {
			return result, errors.New("provider launch operator or routes differ from the original signed selection")
		}
		domain := op.Domain
		if domain.ChainID != preparation.Plan.Config.Network.EvmChainId || domain.GenesisHash != common.HexToHash(preparation.Plan.Config.Network.GenesisHash) || domain.Netuid != preparation.Plan.Config.Netuid ||
			domain.Coordinator != roles.CoordinatorProxy || domain.SettlementVault != roles.SettlementVault || domain.PolicyHash != roles.InitialPolicyHash || domain.DeploymentIDHash != sha256.Sum256([]byte(preparation.Plan.Config.DeploymentId)) {
			return result, errors.New("provider launch domain differs from independent original installation graph")
		}
		digest, err := domain.Digest()
		if err != nil {
			return result, err
		}
		encoded, err := json.Marshal(domain)
		if err != nil {
			return result, err
		}
		content := sha256.Sum256(encoded)
		path := filepath.Join(outputDirectory, host.Id+".close-domain.json")
		result.ProviderDeclarations = append(result.ProviderDeclarations, bootstrapProviderRoleLaunch{Host: host, Domain: domain,
			DomainHash: "sha256:" + hex.EncodeToString(digest[:]), DomainFile: planFileReference{Path: path, Sha256: "sha256:" + hex.EncodeToString(content[:])},
			Arguments:   []string{"provide", "--api_url=" + op.ApiUrl, "--connect_url=" + op.ConnectUrl, "--close-report-domain=" + path},
			Environment: map[string]string{"URNETWORK_STATE_DIR": host.StateDirectory}, EnrollmentDomain: domain})
	}
	result.Schema, result.PreparationHash, result.ContractRolePlanHash = bootstrapProviderRoleSchema, preparation.Plan.ContentHash, roles.ContentHash
	result.Request = planFileReference{Path: requestPath, Sha256: requestHash}
	result.ContentHash = rootObjectHash(result)
	return result, nil
}

// Local file generation is separate from preparation and daemon activation.
// Exact retries accept retained files; changed or foreign files are never replaced.
func runBootstrapProviderRoleCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("provider-role-config", flag.ContinueOnError)
	flags.SetOutput(stderr)
	config := flags.String("config", "", "exact existing chain preparation configuration")
	providers := flags.String("providers", "", "explicit provider/operator launch inventory")
	outputDirectory := flags.String("output-dir", "", "precreated private directory for optional public domain files")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *config == "" || *providers == "" || *outputDirectory == "" {
		fmt.Fprintln(stderr, "provider-role-config requires --config, --providers and --output-dir")
		return 2
	}
	result, err := loadBootstrapProviderRoleConfig(ctx, *config, *providers, *outputDirectory)
	if err != nil {
		fmt.Fprintln(stderr, "provider role declarations:", err)
		return 2
	}
	if err := publishBootstrapProviderDomains(ctx, *outputDirectory, result.ProviderDeclarations); err != nil {
		fmt.Fprintln(stderr, "provider role local files; retain exact completed files for retry:", err)
		return 3
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		fmt.Fprintln(stderr, "provider role output; repeat for the same retained files:", err)
		return 1
	}
	return 0
}
