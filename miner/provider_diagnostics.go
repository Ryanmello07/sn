// Miner daemon diagnostics borrow one bounded exporter. Callback output never
// waits on stdout and never retains credentials, identities or raw errors.
package miner

import (
	"context"
	"encoding/json"
	"io"
	"syscall"
	"time"

	"github.com/urfoundation/sn/diagnostics"
)

const providerDiagnosticSchema = "urnetwork-provider-diagnostics-v1"

// Fixed events select their own domain; provider indices are numeric facts,
// never dynamic queue names or labels. These facts grant no serving authority.
type providerDiagnosticEvent uint8

const (
	providerAuthenticationWait providerDiagnosticEvent = iota
	providerJwtSaveFailed
	providerRejectionSaveFailed
	providerAuthenticationRejected
	providerClientKeySaveFailed
	providerTlsKeySaveFailed
	providerExtenderKeySaveFailed
	providerExtenderIdentityInvalid
	providerIdentityReady
	providerExtenderObserved
	providerStarted
	providerWorkerFailed
	providerStatusStarted
	providerStatusFailed
	providerStatusNotice
)

// Immutable scalar status can be compared without formatting SDK error strings.
type providerExtenderObservation struct {
	Known            bool `json:"known"`
	Enabled          bool `json:"enabled"`
	Listening        bool `json:"listening"`
	ListenFailed     bool `json:"listen_failed"`
	ActivatedV4      bool `json:"activated_v4"`
	ActivatedV6      bool `json:"activated_v6"`
	ActivationFailed bool `json:"activation_failed"`
	Revoked          bool `json:"revoked"`
}

// Optional operational delivery evidence is independent of status=ok, which
// retains its existing process-liveness meaning. Counters describe one run.
type providerDiagnosticStatus struct {
	Schema         string               `json:"schema"`
	Authentication diagnostics.Snapshot `json:"authentication"`
	Keys           diagnostics.Snapshot `json:"keys"`
	Extender       diagnostics.Snapshot `json:"extender"`
	Runtime        diagnostics.Snapshot `json:"runtime"`
}

// Tests observe actual construction and joined closure, without replacing the
// authentication, file or lifecycle operations that own these events.
type providerDiagnosticHooks struct {
	afterCreate func(*providerDiagnostics)
	afterClose  func(*providerDiagnostics, error)
}

// A private key avoids collisions with protocol and authentication contexts.
type providerDiagnosticHooksKey struct{}

// Concurrent producers share only this explicit instance's four fixed queues.
type providerDiagnostics struct{ exporter *diagnostics.Exporter }

// Refused sinks remain visible through snapshots; no synchronous fallback is
// attempted. The owning run must close this object on every subsequent exit.
func newProviderDiagnostics(ctx context.Context, writer io.Writer) (*providerDiagnostics, error) {
	exporter, err := diagnostics.New(ctx, writer, []string{"authentication", "keys", "extender", "runtime"})
	if err != nil {
		return nil, err
	}
	return &providerDiagnostics{exporter: exporter}, nil
}

// The original sink remains caller-owned; exporter descriptors and worker join.
func (self *providerDiagnostics) close() error {
	if self == nil {
		return nil
	}
	return self.exporter.Close()
}

// Missing publication is omitted, never synthesized as successful delivery.
func (self *providerDiagnostics) snapshot() *providerDiagnosticStatus {
	if self == nil {
		return nil
	}
	return &providerDiagnosticStatus{Schema: providerDiagnosticSchema, Authentication: self.exporter.Snapshot("authentication"), Keys: self.exporter.Snapshot("keys"), Extender: self.exporter.Snapshot("extender"), Runtime: self.exporter.Snapshot("runtime")}
}

// Walk a bounded error tree without Error, String or custom Is methods. Mixed
// or unknown joined causes stay unknown instead of disguising a hard fault.
func providerDiagnosticCause(err error) string {
	if err == nil {
		return "none"
	}
	remaining := 32
	var classify func(error) string
	classify = func(current error) string {
		if current == nil || remaining == 0 {
			return "unknown"
		}
		remaining--
		switch current {
		case context.Canceled:
			return "canceled"
		case context.DeadlineExceeded, syscall.ETIMEDOUT:
			return "timeout"
		case syscall.EACCES, syscall.EPERM:
			return "permission"
		case syscall.ENOSPC, syscall.ENOENT:
			return "unavailable"
		}
		switch value := current.(type) {
		case interface{ Unwrap() []error }:
			cause := ""
			for _, next := range value.Unwrap() {
				observed := classify(next)
				if observed == "unknown" || cause != "" && cause != observed {
					return "unknown"
				}
				cause = observed
			}
			if cause != "" {
				return cause
			}
		case interface{ Unwrap() error }:
			return classify(value.Unwrap())
		}
		return "unknown"
	}
	return classify(err)
}

// Serialization has fixed field/count bounds before queue admission. An accepted
// offer means queued; only exporter snapshots report completed sink writes.
func (self *providerDiagnostics) observe(event providerDiagnosticEvent, provider uint64, known bool, err error, retry time.Duration, extender *providerExtenderObservation) {
	if self == nil {
		return
	}
	domain, code := "runtime", "unknown"
	switch event {
	case providerAuthenticationWait:
		domain, code = "authentication", "retry_wait"
	case providerJwtSaveFailed:
		domain, code = "authentication", "jwt_save_failed"
	case providerRejectionSaveFailed:
		domain, code = "authentication", "rejection_save_failed"
	case providerAuthenticationRejected:
		domain, code = "authentication", "rejected"
	case providerClientKeySaveFailed:
		domain, code = "keys", "client_key_save_failed"
	case providerTlsKeySaveFailed:
		domain, code = "keys", "tls_key_save_failed"
	case providerExtenderKeySaveFailed:
		domain, code = "keys", "extender_key_save_failed"
	case providerExtenderIdentityInvalid:
		domain, code = "keys", "extender_identity_invalid"
	case providerIdentityReady:
		domain, code = "keys", "identity_available"
	case providerExtenderObserved:
		domain, code = "extender", "observed"
	case providerStarted:
		code = "provider_started"
	case providerWorkerFailed:
		code = "provider_worker_failed"
	case providerStatusStarted:
		code = "status_started"
	case providerStatusFailed:
		code = "status_failed"
	case providerStatusNotice:
		code = "http_notice"
	}
	if event != providerExtenderObserved {
		extender = nil
	}
	if retry < 0 {
		retry = 0
	}
	record := struct {
		Schema        string                       `json:"schema"`
		Domain        string                       `json:"domain"`
		Event         string                       `json:"event"`
		Provider      uint64                       `json:"provider"`
		ProviderKnown bool                         `json:"provider_known"`
		Cause         string                       `json:"cause"`
		RetryMs       uint64                       `json:"retry_ms"`
		Extender      *providerExtenderObservation `json:"extender,omitempty"`
	}{Schema: providerDiagnosticSchema, Domain: domain, Event: code, Provider: provider, ProviderKnown: known, Cause: providerDiagnosticCause(err), RetryMs: uint64(retry / time.Millisecond), Extender: extender}
	raw, encodeErr := json.Marshal(record)
	if encodeErr != nil {
		return // All fields above are scalar; no arbitrary marshaler is invoked.
	}
	self.exporter.Offer(domain, append(raw, '\n'))
}
