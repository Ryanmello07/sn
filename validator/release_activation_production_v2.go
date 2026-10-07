//go:build linux || darwin

// A signed schema-3 config whose evidence_v2 inputs are activation-pending is
// activated into a separate successor. The successor pins the rendered inputs,
// selects the original config and approval as immutable authority history and
// carries the original approval re-signed for its own complete-config hash.
package validator

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"slices"

	"gopkg.in/yaml.v3"
)

// ReleaseProductionSuccessorOptions names the three successor files and, once
// the approval key has signed the printed message, its Ed25519 signature.
type ReleaseProductionSuccessorOptions struct {
	RenderedConfigPath    string
	SuccessorApprovalPath string
	OriginalAuthorityPath string
	ApprovalSignature     string
}

// Each path is canonical and stands alone; none may contain another.
func (self ReleaseProductionSuccessorOptions) validate() error {
	paths := []string{self.RenderedConfigPath, self.SuccessorApprovalPath, self.OriginalAuthorityPath}
	for index, path := range paths {
		if path == "" {
			return errors.New("a signed production activation requires --rendered-config, --successor-approval and --original-authority")
		}
		if err := ValidateReleaseEvidenceV2Path(path); err != nil {
			return fmt.Errorf("successor path %s: %w", path, err)
		}
		for _, prior := range paths[:index] {
			if productionOriginalRequestPathsOverlap(path, prior) {
				return errors.New("successor config, approval and original authority paths must be distinct and unnested")
			}
		}
	}
	if self.ApprovalSignature != "" {
		if _, err := decodeReleaseProductionSuccessorSignature(self.ApprovalSignature); err != nil {
			return err
		}
	}
	return nil
}

// The envelope stores exactly this canonical spelling, so it is required here.
func decodeReleaseProductionSuccessorSignature(value string) ([]byte, error) {
	signature, err := hex.DecodeString(value)
	if err != nil || len(signature) != ed25519.SignatureSize || hex.EncodeToString(signature) != value {
		return nil, errors.New("--approval-signature must be the approval key's Ed25519 signature as 128 lowercase hex characters without a prefix")
	}
	return signature, nil
}

// Successor files never replace or nest within the signed original, its
// approval, earlier history or any declared state, credential or evidence path.
func validateReleaseProductionSuccessorPaths(original *ReleaseConfig, originalPath string, options ReleaseProductionSuccessorOptions) error {
	if err := options.validate(); err != nil {
		return err
	}
	if original == nil || productionEconomicSelection(original) == nil {
		return errors.New("production successor has no signed original selection")
	}
	abs, err := filepath.Abs(originalPath)
	if err != nil {
		return err
	}
	reserved := append([]string{abs, productionEconomicSelection(original).Approval.Path}, productionBootstrapDeclaredPaths(original)...)
	for _, reference := range append(slices.Clone(original.ProductionAuthorityHistory), original.ProductionRuntimeApprovals...) {
		reserved = append(reserved, reference.Path)
	}
	for _, path := range []string{options.RenderedConfigPath, options.SuccessorApprovalPath, options.OriginalAuthorityPath} {
		for _, other := range reserved {
			if productionOriginalRequestPathsOverlap(path, other) {
				return fmt.Errorf("successor path %s overlaps the signed original, its approval or history, or declared state, credentials or evidence", path)
			}
		}
	}
	return nil
}

// The unsigned successor of one signed activation-pending config. Its config
// selects only the successor approval's path until the signature completes it.
type releaseProductionSuccessor struct {
	original  *ReleaseConfig
	options   ReleaseProductionSuccessorOptions
	config    *ReleaseConfig
	authority []byte
	reference ReleaseEvidenceV2File
	approval  OwnerRecycleApproval
	message   []byte
}

// Builds the successor from the loaded original and the rendered entries.
// The approval is the original verbatim except its complete-config hash, and
// the rendering rule the producer loader applies is checked before any output.
func buildReleaseProductionSuccessor(ctx context.Context, original *ReleaseConfig, originalPath string, rendered []ReleaseEvidenceV2OperatorConfig, options ReleaseProductionSuccessorOptions) (*releaseProductionSuccessor, error) {
	if ctx == nil {
		return nil, errors.New("production successor context is absent")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if original == nil || original.SchemaVersion != ReleaseMainnetProductionSchemaVersion {
		return nil, errors.New("production successor requires a signed schema 3 original")
	}
	if err := errors.Join(validateReleaseProductionAuthorityHistory(original), requireReleaseEvidenceV2ProductionPending(original.EvidenceV2.Operators)); err != nil {
		return nil, err
	}
	if err := validateReleaseProductionSuccessorPaths(original, originalPath, options); err != nil {
		return nil, err
	}
	if len(original.ProductionAuthorityHistory) >= maximumProductionAuthorityHistory {
		return nil, errors.New("production successor exhausts the original authority history count")
	}
	approved, err := ownerRecycleProductionApproval(original)
	if err != nil {
		return nil, err
	}
	authority, err := BuildOwnerRecycleProductionAuthority(ctx, original)
	if err != nil {
		return nil, err
	}
	reference := ReleaseEvidenceV2File{Path: options.OriginalAuthorityPath, Bytes: uint64(len(authority)), SHA256: attemptHex32(sha256.Sum256(authority))}
	raw, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var next ReleaseConfig
	if err := json.Unmarshal(raw, &next); err != nil {
		return nil, err
	}
	next.EvidenceV2.Operators = slices.Clone(rendered)
	next.ProductionAuthorityHistory = append(next.ProductionAuthorityHistory, reference)
	selection := &ReleaseOwnerRecycleApprovalConfig{Signer: productionEconomicSelection(original).Signer, Approval: ReleaseEvidenceV2File{Path: options.SuccessorApprovalPath}}
	if original.TreasuryApproval != nil {
		next.TreasuryApproval = selection
	} else {
		next.OwnerRecycleApproval = selection
	}
	// Nested runtime bounds keep their YAML names. Sign only what the strict
	// document decoder reproduces, as the capacity revision preview does.
	document, err := yaml.Marshal(&next)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeReleaseConfigDocument(options.RenderedConfigPath, document)
	if err != nil {
		return nil, err
	}
	beforeHash, beforeErr := OwnerRecycleConfigHash(&next)
	afterHash, afterErr := OwnerRecycleConfigHash(decoded)
	if beforeErr != nil || afterErr != nil || beforeHash != afterHash {
		return nil, errors.Join(errors.New("production successor changes under strict document decoding"), beforeErr, afterErr)
	}
	approval := approved.Approval
	approval.ConfigHash = afterHash
	if err := validateProductionEvidenceRendering(original, decoded, approved.Approval, approval); err != nil {
		return nil, err
	}
	message, err := approval.SigningMessage()
	if err != nil {
		return nil, err
	}
	return &releaseProductionSuccessor{original: original, options: options, config: decoded, authority: authority, reference: reference,
		approval: approval, message: message}, ctx.Err()
}

// The original authority bundle is immutable public input. Writing it is
// idempotent and grants nothing until a signed successor selects its hash.
func (self *releaseProductionSuccessor) retainOriginalAuthority(ctx context.Context) error {
	written, err := WriteReleaseEvidenceV2File(ctx, self.reference.Path, self.authority, maximumProductionAuthorityBundleBytes)
	if err != nil {
		return err
	}
	if written != self.reference {
		return errors.New("original authority bundle differs from its successor reference")
	}
	return nil
}

// Exactly what the approval key reviews and signs: the unsigned approval and
// the 32-byte digest of its signature domain and canonical JSON.
func (self *releaseProductionSuccessor) print(output io.Writer) {
	approval, _ := json.Marshal(self.approval)
	original := productionEconomicSelection(self.original).Approval
	fmt.Fprintf(output, "successor: original authority %s (%d bytes, sha256 %s) binds the signed config and its approval %s (sha256 %s)\n",
		self.reference.Path, self.reference.Bytes, self.reference.SHA256, original.Path, original.SHA256)
	fmt.Fprintf(output, "successor: %s selects the re-approval at %s; complete-config hash %s\n",
		self.options.RenderedConfigPath, self.options.SuccessorApprovalPath, attemptHex32(self.approval.ConfigHash))
	fmt.Fprintf(output, "successor approval (unsigned, %s): %s\n", self.approval.Schema, approval)
	fmt.Fprintf(output, "successor approval signing message: 0x%x\n", self.message)
}

// Admits only the pinned approval key's signature over the exact successor
// message, writes the envelope and successor through the private no-replace
// owner, and proves the producer loader authenticates the complete lineage
// before the successor file exists. Exact retries are idempotent.
func (self *releaseProductionSuccessor) publish(ctx context.Context, signatureHex string) ([]byte, error) {
	signature, err := decodeReleaseProductionSuccessorSignature(signatureHex)
	if err != nil {
		return nil, err
	}
	signer, err := canonicalAttemptHex32("production successor approval signer", productionEconomicSelection(self.config).Signer, false)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(signer[:], self.message, signature) {
		return nil, errors.New("--approval-signature does not verify the successor approval message under the pinned approval key")
	}
	if err := self.retainOriginalAuthority(ctx); err != nil {
		return nil, err
	}
	envelope, err := json.Marshal(OwnerRecycleApprovalEnvelope{Approval: self.approval, Signature: signatureHex})
	if err != nil {
		return nil, err
	}
	reference, err := WriteReleaseEvidenceV2File(ctx, self.options.SuccessorApprovalPath, append(envelope, '\n'), maximumOwnerRecycleApprovalBytes)
	if err != nil {
		return nil, err
	}
	final := *self.config
	selection := *productionEconomicSelection(self.config)
	selection.Approval = reference
	if final.TreasuryApproval != nil {
		final.TreasuryApproval = &selection
	} else {
		final.OwnerRecycleApproval = &selection
	}
	document, err := yaml.Marshal(&final)
	if err != nil {
		return nil, err
	}
	loaded, err := decodeReleaseConfigBytes(self.options.RenderedConfigPath, document)
	if err != nil {
		return nil, fmt.Errorf("successor configuration is not admitted by the producer loader: %w", err)
	}
	approved, err := ownerRecycleProductionApproval(loaded)
	if err != nil || !reflect.DeepEqual(approved.Approval, self.approval) || len(loaded.ProductionAuthorityHistory) != len(self.original.ProductionAuthorityHistory)+1 {
		return nil, errors.Join(errors.New("successor configuration changes its re-approved authority or lineage"), err)
	}
	if _, err := WriteReleaseEvidenceV2File(ctx, self.options.RenderedConfigPath, document, maximumReleaseConfigBytes); err != nil {
		return nil, err
	}
	return document, ctx.Err()
}

// completeProduction renders the inputs at the paths the signed config
// pre-declared and builds its successor; the signed original is never
// rewritten. Without the approval key's signature it stops after printing the
// successor approval and its exact signing message.
func (self *releaseActivationSetup) completeProduction(ctx context.Context, prepared *ReleaseActivationSetupPreparedV2, completed *ReleaseActivationSetupCompletedV2, options ReleaseProductionSuccessorOptions) error {
	if err := validateReleaseProductionSuccessorPaths(self.cfg, self.configPath, options); err != nil {
		return err
	}
	rendered, err := self.renderInputs(ctx, prepared, completed)
	if err != nil {
		return err
	}
	successor, err := buildReleaseProductionSuccessor(ctx, self.cfg, self.configPath, rendered, options)
	if err != nil {
		return err
	}
	if err := successor.retainOriginalAuthority(ctx); err != nil {
		return err
	}
	successor.print(self.output)
	if options.ApprovalSignature == "" {
		fmt.Fprintf(self.output, "waiting: sign exactly the 32 signing-message bytes with the approval key, then re-run with --approval-signature=<128 hex> --apply\n")
		return nil
	}
	if _, err := successor.publish(ctx, options.ApprovalSignature); err != nil {
		return err
	}
	fmt.Fprintf(self.output, "configuration: successor written to %s; the signed original %s is unchanged\n", options.RenderedConfigPath, self.configPath)
	cfg, err := LoadReleaseConfig(options.RenderedConfigPath)
	if err != nil {
		return fmt.Errorf("successor configuration does not load strictly: %w", err)
	}
	if _, err := loadReleaseEvidenceV2ActivationInputs(ctx, cfg, self.chain, self.native, self.hotkey.PublicKey()); err != nil {
		return fmt.Errorf("rendered activation inputs do not load: %w", err)
	}
	fmt.Fprintf(self.output, "activation complete: the producer loader authenticates the successor and its signed original; start `validator run --config=%s`\n", options.RenderedConfigPath)
	return nil
}
