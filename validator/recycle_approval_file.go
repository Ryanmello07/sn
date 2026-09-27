//go:build linux || darwin

// Approval installation uses the existing immutable, no-replace descriptor
// custody owner. It authenticates provided signatures and never generates one.
package validator

import (
	"context"
	"path/filepath"
)

// Retains one exact independently signed successor, with file and directory
// fsync. Repetition is idempotent; a different successor requires a separately
// implemented policy-history migration, not overwriting this fixed record.
// The configuration is borrowed and must remain immutable during the call.
func RetainOwnerRecycleApproval(ctx context.Context, cfg *ReleaseConfig) (ReleaseEvidenceV2File, error) {
	if err := validateOwnerRecycleApprovalSelection(cfg); err != nil {
		return ReleaseEvidenceV2File{}, err
	}
	if isOwnerRecycleProductionConfig(cfg) {
		if err := validateOwnerRecycleProductionConfig(cfg); err != nil {
			return ReleaseEvidenceV2File{}, err
		}
		return WriteReleaseEvidenceV2File(ctx, filepath.Join(cfg.StateDir, retainedOwnerRecycleApprovalName), cfg.ownerRecycleProduction.encoded, maximumOwnerRecycleApprovalBytes)
	}
	raw, err := ReadReleaseEvidenceV2File(ctx, cfg.OwnerRecycleApproval.Approval, maximumOwnerRecycleApprovalBytes)
	if err != nil {
		return ReleaseEvidenceV2File{}, err
	}
	if _, err := decodeOwnerRecycleApproval(cfg, raw); err != nil {
		return ReleaseEvidenceV2File{}, err
	}
	return WriteReleaseEvidenceV2File(ctx, filepath.Join(cfg.StateDir, retainedOwnerRecycleApprovalName), raw, maximumOwnerRecycleApprovalBytes)
}
