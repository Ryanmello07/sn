// Optional original-request custody is opened by each real authenticated
// operator. Legacy configurations keep unknown coverage without a new gate.
package validator

import (
	"context"
	"crypto/ed25519"
	"errors"
	"strings"

	"github.com/urfoundation/sn/protocol"
	"github.com/urnetwork/connect"
)

// The hash-pinned preparation input is separate from mutable JWT and directory
// state. The authenticated client and original ledger must both match it.
func openReleaseProviderAttemptRequests(ctx context.Context, cfg *ReleaseConfig, op OperatorConfig, ledger *AttemptLedger, clientId connect.Id, key ed25519.PrivateKey) (*ProviderAttemptRequestJournal, error) {
	if op.RequestPreparation == nil {
		return nil, nil
	}
	if cfg == nil || ledger == nil {
		return nil, errors.New("provider request runtime has no original ledger owner")
	}
	raw, err := ReadReleaseEvidenceV2File(ctx, *op.RequestPreparation, 4096)
	if err != nil {
		return nil, err
	}
	var expected ProviderAttemptRequestPreparation
	if err := attemptStoreDecode(raw, &expected); err != nil {
		return nil, err
	}
	policyHash, err := parseHash32("provider request policy", cfg.PolicyHash)
	if err != nil {
		return nil, err
	}
	if expected.Identity.Ledger != ledger.identity || expected.Identity.ClientId != clientId || expected.Identity.PolicyHash != policyHash || expected.Identity.Coordinator != strings.ToLower(cfg.Coordinator) {
		return nil, errors.Join(protocol.ErrProviderAttemptsIntegrity, errors.New("provider request prepared identity differs from authenticated operator"))
	}
	return OpenProviderAttemptRequestJournal(ctx, op.StateDir, expected, key)
}
