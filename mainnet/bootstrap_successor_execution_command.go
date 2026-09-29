// Local commands reconstruct original custody and independently approve exact
// execution bytes. No public command constructs an unauthenticated chain adapter.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// The request is signable only after actual original journals, completed local
// preparation, pinned Safe release and both signature files are reconstructed.
func loadBootstrapSuccessorExecution(ctx context.Context, configPath, directory, accepted, requestPath, safeRequestPath, executionRequestPath, approvalPath string) (_ bootstrapSuccessorExecutionPlan, _ *safeExecutionProfile, _ *bootstrapChainReadinessState, resultErr error) {
	var result bootstrapSuccessorExecutionPlan
	var request bootstrapSuccessorExecutionRequest
	raw, requestHash, err := readBootstrapRootFile(ctx, executionRequestPath, 16*1024)
	if err == nil {
		err = decodePlanJson(raw, &request)
	}
	if err == nil {
		err = request.validate()
	}
	if err != nil {
		return result, nil, nil, err
	}
	var safeRequest bootstrapSuccessorSafeRequest
	raw, safeHash, err := readBootstrapRootFile(ctx, safeRequestPath, 16*1024)
	if err == nil {
		err = decodePlanJson(raw, &safeRequest)
	}
	if err == nil {
		err = safeRequest.validate()
	}
	if err != nil {
		return result, nil, nil, err
	}
	plan, retained, err := loadBootstrapSuccessorPreparation(ctx, configPath, directory, accepted, requestPath, approvalPath,
		safeRequestPath, executionRequestPath, safeRequest.Archive.Path, request.SafeSignatures.Path, request.RelayerTransaction.Path)
	if err != nil {
		return result, nil, nil, err
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, retained.close())
		}
	}()
	for _, path := range []string{configPath, requestPath, safeRequestPath, executionRequestPath, approvalPath, safeRequest.Archive.Path, request.SafeSignatures.Path, request.RelayerTransaction.Path} {
		if path == request.RegistryDirectory || strings.HasPrefix(path, request.RegistryDirectory+"/") {
			return result, nil, nil, errors.New("successor execution input overlaps the nonce registry")
		}
	}
	reader, record, err := openBootstrapSuccessorPreparationReader(ctx, plan, nil)
	if err != nil {
		return result, nil, nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, reader.close()) }()
	rawArchive, archiveHash, err := readPlanFile(ctx, safeRequest.Archive.Path, maximumSafeReleaseArchiveBytes)
	if err != nil || archiveHash != safeRequest.Archive.Sha256 {
		return result, nil, nil, errors.Join(errors.New("successor execution archive pin differs"), err)
	}
	review, err := buildBootstrapSuccessorSafeReview(ctx, plan, record, safeRequest, planFileReference{Path: safeRequestPath, Sha256: safeHash}, rawArchive)
	if err != nil {
		return result, nil, nil, err
	}
	pin, err := loadSafeReleasePin(safeRequest.Version, safeRequest.Variant)
	if err != nil {
		return result, nil, nil, err
	}
	members, err := readSafeReleaseMembers(ctx, rawArchive, pin, safeRequest.Variant)
	if err != nil {
		return result, nil, nil, err
	}
	var profile *safeExecutionProfile
	for _, artifact := range pin.Artifacts {
		if artifact.Name == safeRequest.Variant {
			profile, err = newSafeExecutionProfile(safeRequest.Version, safeRequest.Variant, members[artifact.ArchivePath])
		}
	}
	if err != nil || profile == nil {
		return result, nil, nil, errors.Join(errors.New("successor execution static profile is absent"), err)
	}
	result, err = buildBootstrapSuccessorExecution(ctx, review, request, planFileReference{Path: executionRequestPath, Sha256: requestHash}, profile)
	if err != nil {
		return result, nil, nil, err
	}
	return result, profile, retained, reader.checkpoint("execution-preview-reconstructed")
}

// Claim and resume retain signatures and conditional execution custody only.
// Network flags are absent until a reviewed canonical adapter is implemented.
func runBootstrapSuccessorExecutionCommand(ctx context.Context, args []string, stdout, stderr io.Writer) (resultCode int) {
	if len(args) == 0 || args[0] != "contract-successor-execution-preview" && args[0] != "contract-successor-execution-claim" && args[0] != "contract-successor-execution-resume" {
		fmt.Fprintln(stderr, "unknown successor execution custody command")
		return 2
	}
	preview := args[0] == "contract-successor-execution-preview"
	flags := flag.NewFlagSet("bootstrap-chain "+args[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "original private v3 preparation config")
	directory := flags.String("run-dir", "", "original signed physical custody directory")
	accepted := flags.String("accept-plan-hash", "", "original accepted v3 preparation hash")
	requestPath := flags.String("request", "", "original successor preparation request")
	safeRequestPath := flags.String("safe-request", "", "retained pinned Safe review request")
	executionRequestPath := flags.String("execution-request", "", "private exact execution custody request")
	approvalPath := flags.String("approval", "", "independent successor execution approval")
	approvalHash := flags.String("approval-sha256", "", "exact approval file digest")
	executionHash := flags.String("accept-execution-hash", "", "exact previewed execution plan hash")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *configPath == "" || *directory == "" || !planSha256(*accepted) ||
		*requestPath == "" || *safeRequestPath == "" || *executionRequestPath == "" ||
		preview && (*approvalPath != "" || *approvalHash != "" || *executionHash != "") ||
		!preview && (*approvalPath == "" || !planSha256(*approvalHash) || !planSha256(*executionHash)) {
		fmt.Fprintln(stderr, "successor execution requires original --config, --run-dir, --accept-plan-hash, --request, --safe-request and --execution-request; claim/resume also require --approval, --approval-sha256 and --accept-execution-hash")
		return 2
	}
	plan, profile, retained, err := loadBootstrapSuccessorExecution(ctx, *configPath, *directory, *accepted, *requestPath, *safeRequestPath, *executionRequestPath, *approvalPath)
	if err != nil {
		fmt.Fprintln(stderr, "successor execution original custody or exact inputs unresolved:", err)
		return 1
	}
	var owner *bootstrapSuccessorExecutionStore
	defer func() {
		if err := errors.Join(owner.close(), retained.close()); err != nil {
			fmt.Fprintln(stderr, "successor execution ownership close:", err)
			resultCode = 1
		}
	}()
	if preview {
		message, err := plan.signingBytes(profile)
		if err == nil {
			err = json.NewEncoder(stdout).Encode(struct {
				Schema       string                          `json:"schema"`
				Plan         bootstrapSuccessorExecutionPlan `json:"plan"`
				PlanHash     string                          `json:"execution_plan_hash"`
				SigningBytes string                          `json:"execution_signing_bytes"`
			}{Schema: "urnetwork-mainnet-successor-execution-preview-v1", Plan: plan, PlanHash: plan.hash(), SigningBytes: "0x" + hex.EncodeToString(message)})
		}
		if err != nil {
			fmt.Fprintln(stderr, "successor execution preview output:", err)
			return 1
		}
		return 0
	}
	if plan.hash() != *executionHash {
		fmt.Fprintln(stderr, "successor execution accepted hash differs from reconstructed original custody")
		return 3
	}
	raw, digest, err := readBootstrapRootFile(ctx, *approvalPath, maximumBootstrapSuccessorExecutionBytes)
	var approval bootstrapSuccessorExecutionApproval
	if err == nil && digest == *approvalHash {
		err = decodePlanJson(raw, &approval)
	} else {
		err = errors.Join(errors.New("successor execution approval file digest differs"), err)
	}
	if err == nil {
		err = approval.validate(plan, profile)
	}
	if err != nil {
		fmt.Fprintln(stderr, "successor independent execution approval:", err)
		return 2
	}
	owner, err = openBootstrapSuccessorExecutionStore(ctx, plan, approval, profile, args[0] == "contract-successor-execution-claim", nil)
	if err != nil {
		fmt.Fprintln(stderr, "successor execution custody unresolved; retain original and nonce registry files:", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(owner.result()); err != nil {
		fmt.Fprintln(stderr, "successor execution output failed; resume exact custody:", err)
		return 1
	}
	return 0
}
