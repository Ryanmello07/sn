// Publication custody binds the complete approved operator roster and original
// request births. It never derives identity or allowances from retained cuts.
package validator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/urfoundation/sn/protocol"
)

const ProviderAttemptPublicationNamespaceAttribute = "user.urnetwork.validator.publications.v1"
const ProviderAttemptPublicationNamespaceSchema = "urnetwork-provider-attempt-publication-custody-v1"

// The actual runtime config supplies the same full profile as offline approval.
// Bounds belong to existing evidence settings, not to the files being restored.
type ProviderAttemptPublicationPreparation struct {
	StateDir        string                               `json:"state_dir"`
	MaxWindowBytes  uint64                               `json:"max_window_bytes"`
	MaxHistoryBytes uint64                               `json:"max_history_bytes"`
	MaxFiles        uint64                               `json:"max_files"`
	Operators       []ProviderAttemptPublicationOperator `json:"operators"`
}

// Each original request owner has its independently approved public close scope.
type ProviderAttemptPublicationOperator struct {
	Preparation  ProviderAttemptRequestPreparation    `json:"preparation"`
	ReceiptScope protocol.ProviderAttemptReceiptScope `json:"receipt_scope"`
}

// Only unsigned physical coordinates change when original custody is copied.
type ProviderAttemptPublicationNamespaceCheckpoint struct {
	Schema          string `json:"schema"`
	ProfileSha256   string `json:"profile_sha256"`
	DirectoryDevice uint64 `json:"directory_device"`
	DirectoryInode  uint64 `json:"directory_inode"`
}

// The fixed directory cannot be selected by a publication filename or cut.
func (self ProviderAttemptPublicationPreparation) Directory() string {
	return filepath.Join(self.StateDir, "provider-attempt-publications")
}

// Validate all original scopes before opening any publication namespace.
func (self ProviderAttemptPublicationPreparation) Validate() error {
	if !filepath.IsAbs(self.StateDir) || filepath.Clean(self.StateDir) != self.StateDir || filepath.Dir(self.StateDir) == self.StateDir || self.MaxWindowBytes == 0 || self.MaxWindowBytes > self.MaxHistoryBytes || self.MaxHistoryBytes >= uint64(^uint(0)>>1) || self.MaxFiles == 0 || self.MaxFiles >= uint64(^uint(0)>>1) || len(self.Operators) == 0 {
		return errors.New("provider publication requires its fixed directory, complete roster and finite evidence bounds")
	}
	noIdKVs := map[uint64]bool{}
	for _, operator := range self.Operators {
		if err := operator.Preparation.Validate(); err != nil {
			return err
		}
		identity, scope := operator.Preparation.Identity, operator.ReceiptScope
		genesis, err := canonicalAttemptHex32("provider publication genesis", identity.Ledger.GenesisHash, false)
		if err != nil {
			return err
		}
		if noIdKVs[identity.Ledger.NoID] || scope.NoId != identity.Ledger.NoID || scope.GenesisHash != genesis || scope.PolicyHash != identity.PolicyHash || scope.Netuid != uint64(identity.Ledger.Netuid) || scope.DeploymentId != identity.Ledger.DeploymentID || scope.Profile == "" || scope.DeploymentKey == "" {
			return errors.New("provider publication original operator and receipt scopes differ or repeat")
		}
		noIdKVs[identity.Ledger.NoID] = true
	}
	raw, err := json.Marshal(self)
	if err != nil || len(raw) > 256*1024 {
		return errors.Join(errors.New("provider publication full approval profile exceeds its finite bound"), err)
	}
	return nil
}

// This digest covers exact typed approved identity, roster order and capacity.
func (self ProviderAttemptPublicationPreparation) Digest() (string, error) {
	if err := self.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(self)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}
