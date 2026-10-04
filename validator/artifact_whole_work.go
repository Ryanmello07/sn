// The actual public reader acquires original whole-work companions without
// rewriting a signed artifact or obtaining authority from its transport source.
package validator

import (
	"context"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/urfoundation/sn/payoutartifact"
)

// Safe for concurrent reads; each call owns its complete finite acquisition.
type HttpWholeWorkInventoryReader struct {
	reader *HTTPArtifactReader
}

// Reuse the production bounded GET transport, including redirect refusal and
// original cancellation-cause preservation. This reader has no signing port.
func NewHttpWholeWorkInventoryReader(apiUrl, deploymentId string, netuid uint16) (*HttpWholeWorkInventoryReader, error) {
	reader, err := NewHTTPArtifactReader(apiUrl, deploymentId, netuid)
	if err != nil {
		return nil, err
	}
	return &HttpWholeWorkInventoryReader{reader: reader}, nil
}

// The exact signed artifact and independent policy select every query field.
// Missing optional originals remain unavailable, including historical windows.
func (self *HttpWholeWorkInventoryReader) Read(ctx context.Context, artifact *payoutartifact.Artifact, expected payoutartifact.WholeWorkExpectation) (*payoutartifact.WholeWorkInventory, *payoutartifact.VerifiedWholeWorkInventory, error) {
	if ctx == nil {
		return nil, nil, errors.New("whole-work public read requires an owner")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, errors.Join(err, context.Cause(ctx))
	}
	if self == nil || self.reader == nil || artifact == nil || expected.AuthoritySigner == (common.Address{}) || expected.ClientKeyRootSigner == (common.Address{}) {
		return nil, nil, payoutartifact.ErrClosedWorkUnavailable
	}
	owner, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	if err := payoutartifact.VerifyWithContext(owner, artifact); err != nil {
		return nil, nil, err
	}
	if artifact.DeploymentID != self.reader.deploymentID || artifact.Netuid != self.reader.netuid {
		return nil, nil, payoutartifact.ErrClosedWorkIntegrity
	}
	domain, err := payoutartifact.ClosedWorkReportDomain(artifact)
	if err != nil {
		return nil, nil, err
	}
	domainHash, err := domain.Digest()
	if err != nil {
		return nil, nil, err
	}
	artifactHash := strings.TrimPrefix(artifact.ContentHash, "sha256:")
	if len(artifactHash) != 64 || strings.ToLower(artifactHash) != artifactHash {
		return nil, nil, payoutartifact.ErrClosedWorkIntegrity
	}
	if _, err := hex.DecodeString(artifactHash); err != nil {
		return nil, nil, payoutartifact.ErrClosedWorkIntegrity
	}
	query := url.Values{"domain": {hex.EncodeToString(domainHash[:])}, "epoch": {strconv.FormatUint(artifact.Epoch, 10)}, "artifact": {artifactHash}}
	if expected.AuthorityHash != "" {
		if !payoutartifact.IsDigest(expected.AuthorityHash, "sha256:") {
			return nil, nil, payoutartifact.ErrClosedWorkIntegrity
		}
		query.Set("authority", strings.TrimPrefix(expected.AuthorityHash, "sha256:"))
	}
	endpoint := self.reader.endpoint("/provider-work/v1/windows", query)
	raw, err := self.reader.get(owner, endpoint, payoutartifact.MaxWholeWorkInventoryBytes)
	if err != nil {
		if owner.Err() != nil {
			return nil, nil, errors.Join(err, owner.Err(), context.Cause(owner))
		}
		var status *releaseHttpGetStatusError
		if errors.As(err, &status) && (status.status == 404 || status.status == 204) {
			return nil, nil, errors.Join(payoutartifact.ErrClosedWorkUnavailable, err)
		}
		return nil, nil, err
	}
	inventory, err := payoutartifact.DecodeWholeWorkInventory(owner, raw)
	if err != nil {
		return nil, nil, err
	}
	verified, err := payoutartifact.VerifyWholeWorkInventoryWithWitness(owner, artifact, inventory, expected)
	if err != nil {
		return nil, nil, err
	}
	return inventory, verified, owner.Err()
}
