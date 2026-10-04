// Explicit launch profiles bind whole-work capture to retained provider keys
// and private outboxes. A profile grants no completeness or payment authority.
package miner

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urnetwork/connect"
	"github.com/urnetwork/sdk"

	"github.com/urfoundation/sn/protocol"
)

const ProviderWorkCaptureSchema = "urnetwork-provider-whole-work-capture-v1"
const maximumProviderWorkCaptureBytes = 256 * 1024

// The independently reviewed public request key is fixed before transport
// begins. Every provider entry names an already retained registration identity.
type ProviderWorkCaptureProfile struct {
	Schema           string                     `json:"schema"`
	ApiUrl           string                     `json:"api_url"`
	RequestPublicKey [32]byte                   `json:"request_public_key"`
	Providers        []ProviderWorkCaptureOwner `json:"providers"`
}

// Generation is deliberately absent: each actual manager creates a fresh one.
// Restart keeps the original outbox and cannot sign a missing prior generation.
type ProviderWorkCaptureOwner struct {
	Slot            string                          `json:"slot"`
	ClientId        [16]byte                        `json:"client_id"`
	PublicKey       [32]byte                        `json:"public_key"`
	Domain          protocol.ClientKeyHistoryDomain `json:"domain"`
	OutboxDirectory string                          `json:"outbox_directory"`
}

// Both CLI and embedded roles use this descriptor-bound public profile reader.
// An absent optional profile remains unknown; a partial reference is an error.
func ReadProviderWorkCaptureProfile(ctx context.Context, path, expectedSha256 string, required bool) (*ProviderWorkCaptureProfile, error) {
	if ctx == nil {
		return nil, errors.New("whole-work launch profile requires an owner")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "" && expectedSha256 == "" && !required {
		return nil, nil
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(expectedSha256) != len("sha256:")+64 || !strings.HasPrefix(expectedSha256, "sha256:") || strings.ToLower(expectedSha256) != expectedSha256 {
		return nil, errors.New("whole-work launch requires an absolute profile and its reviewed sha256 digest")
	}
	expected, err := hex.DecodeString(strings.TrimPrefix(expectedSha256, "sha256:"))
	if err != nil || len(expected) != sha256.Size {
		return nil, errors.New("whole-work launch profile digest is malformed")
	}
	file, err := openProviderCloseReportDomain(path)
	if err != nil {
		return nil, err
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumProviderWorkCaptureBytes {
		return nil, errors.Join(errors.New("whole-work launch profile must be a bounded regular file"), statErr, file.Close())
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maximumProviderWorkCaptureBytes+1))
	if err := errors.Join(readErr, file.Close(), ctx.Err()); err != nil {
		return nil, err
	}
	actual := sha256.Sum256(raw)
	if len(raw) > maximumProviderWorkCaptureBytes || !bytes.Equal(actual[:], expected) {
		return nil, errors.New("whole-work launch profile differs from its reviewed original")
	}
	if err := protocol.ValidateUniqueJsonKeys(raw); err != nil {
		return nil, err
	}
	var result ProviderWorkCaptureProfile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	if err := result.Validate(); err != nil {
		return nil, err
	}
	for _, provider := range result.Providers {
		if strings.HasPrefix(path, provider.OutboxDirectory+string(filepath.Separator)) {
			return nil, errors.New("whole-work launch profile overlaps original outbox custody")
		}
	}
	return &result, nil
}

// Structural admission preserves the full declared roster and requires an
// already provisioned outbox. No file, request, key or identity is created here.
func (self ProviderWorkCaptureProfile) Validate() error {
	endpoint, err := url.Parse(self.ApiUrl)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Opaque != "" || endpoint.Fragment != "" || endpoint.Path != "" || endpoint.RawPath != "" {
		return errors.New("whole-work launch requires an exact https operator origin")
	}
	if self.Schema != ProviderWorkCaptureSchema || self.RequestPublicKey == ([32]byte{}) || len(self.Providers) == 0 || len(self.Providers) > 64 {
		return errors.New("whole-work launch requires its original request authority and between one and 64 providers")
	}
	slotKVs, clientKVs := map[string]bool{}, map[[16]byte]bool{}
	outboxInfos := make([]os.FileInfo, 0, len(self.Providers))
	for index, provider := range self.Providers {
		if provider.Slot == "" || len(provider.Slot) > 96 || strings.TrimSpace(provider.Slot) != provider.Slot || strings.ContainsAny(provider.Slot, "/\\\x00\r\n") || slotKVs[provider.Slot] || provider.ClientId == ([16]byte{}) || clientKVs[provider.ClientId] || provider.PublicKey == ([32]byte{}) || provider.PublicKey == self.RequestPublicKey {
			return errors.New("whole-work launch provider identity is missing, repeated or shares request signing authority")
		}
		slotKVs[provider.Slot], clientKVs[provider.ClientId] = true, true
		if _, err := provider.Domain.Digest(); err != nil {
			return err
		}
		path := provider.OutboxDirectory
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return errors.New("whole-work launch outbox must be a canonical absolute directory")
		}
		for _, prior := range self.Providers[:index] {
			if prior.OutboxDirectory == path || strings.HasPrefix(path, prior.OutboxDirectory+string(filepath.Separator)) || strings.HasPrefix(prior.OutboxDirectory, path+string(filepath.Separator)) {
				return errors.New("whole-work launch outboxes overlap")
			}
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
			return errors.Join(errors.New("whole-work launch requires a precreated private original outbox"), err)
		}
		if err := validateProviderWorkCaptureDirectory(info); err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || resolved != path {
			return errors.Join(errors.New("whole-work launch outbox contains a path alias"), err)
		}
		root, err := os.OpenRoot(path)
		if err != nil {
			return err
		}
		opened, statErr := root.Stat(".")
		closeErr := root.Close()
		if statErr != nil || !os.SameFile(info, opened) {
			return errors.Join(errors.New("whole-work launch outbox changed during admission"), statErr, closeErr)
		}
		if closeErr != nil {
			return closeErr
		}
		for _, prior := range outboxInfos {
			if os.SameFile(prior, opened) {
				return errors.New("whole-work launch outboxes share physical custody")
			}
		}
		outboxInfos = append(outboxInfos, opened)
	}
	return nil
}

// A launcher must declare the actual complete role census before workers start.
// Extra entries cannot silently be treated as providers that were observed idle.
func (self *ProviderWorkCaptureProfile) validateRole(apiUrl string, slots []string, domainHash [32]byte) error {
	if self == nil {
		return nil
	}
	if self.ApiUrl != apiUrl || len(self.Providers) != len(slots) {
		return errors.New("whole-work launch operator or complete provider census differs")
	}
	for _, slot := range slots {
		matched := false
		for _, provider := range self.Providers {
			if provider.Slot != slot {
				continue
			}
			matched = true
			digest, err := provider.Domain.Digest()
			if err != nil || domainHash != ([32]byte{}) && domainHash != digest {
				return errors.New("whole-work launch differs from the original close-report domain")
			}
		}
		if !matched {
			return errors.New("whole-work launch omits an actual provider slot")
		}
	}
	return nil
}

// The actual retained SDK key and authenticated client select their one entry.
// Mutable SDK settings are fresh per owner; no profile can replace key material.
func (self *ProviderWorkCaptureProfile) apply(settings *sdk.DeviceLocalSettings, slot string, clientId connect.Id) error {
	if self == nil {
		return nil
	}
	if settings == nil || settings.ContractManagerSettings == nil || settings.KeyMaterial == nil {
		return errors.New("whole-work launch has no retained provider settings or key")
	}
	seed := settings.KeyMaterial.GetClientKeySeed()
	if len(seed) != ed25519.SeedSize {
		return errors.New("whole-work launch client key is not an original 32-byte seed")
	}
	key := ed25519.NewKeyFromSeed(seed)
	for _, provider := range self.Providers {
		if provider.Slot != slot {
			continue
		}
		if provider.ClientId != [16]byte(clientId) || !bytes.Equal(provider.PublicKey[:], key[ed25519.SeedSize:]) {
			return errors.New("whole-work launch differs from retained provider identity")
		}
		domainHash, err := provider.Domain.Digest()
		if err != nil || settings.ContractManagerSettings.CloseReportDomainHash != ([32]byte{}) && settings.ContractManagerSettings.CloseReportDomainHash != domainHash {
			return errors.Join(errors.New("whole-work launch differs from retained provider domain"), err)
		}
		settings.ContractManagerSettings.CloseReportDomainHash = domainHash
		settings.ContractManagerSettings.OriginalWorkCapture = &connect.OriginalWorkCaptureSettings{ApiUrl: self.ApiUrl, OutboxDirectory: provider.OutboxDirectory, RequestPublicKey: self.RequestPublicKey, PollInterval: 15 * time.Second}
		settings.ClientKeyRegistrationRequired = true
		return nil
	}
	return fmt.Errorf("whole-work launch has no original owner for provider slot %q", slot)
}

// Both production provider roles enter this constructor. The evidence worker
// receives the same owner's dial and trust settings, with its own finite pool.
func newProviderDeviceLocal(networkSpace *sdk.NetworkSpace, strategySettings *connect.ClientStrategySettings, token, description string, settings *sdk.DeviceLocalSettings, profile *ProviderWorkCaptureProfile, slot string, clientId connect.Id) (*sdk.DeviceLocal, error) {
	if err := profile.apply(settings, slot, clientId); err != nil {
		return nil, err
	}
	var transport *http.Transport
	if capture := settings.ContractManagerSettings.OriginalWorkCapture; capture != nil {
		if strategySettings == nil {
			return nil, errors.New("whole-work provider transport owner is absent")
		}
		connectSettings := strategySettings.ConnectSettings
		transport = &http.Transport{DialContext: connectSettings.DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 60 * time.Second}
		if connectSettings.TlsConfig != nil {
			transport.TLSClientConfig = connectSettings.TlsConfig.Clone()
		}
		capture.HttpClient = &http.Client{Transport: transport, Timeout: 60 * time.Second}
	}
	device, err := sdk.NewDeviceLocal(networkSpace, token, description, "", RequireVersion(), sdk.NewId(), settings)
	if err != nil && transport != nil {
		transport.CloseIdleConnections()
	}
	return device, err
}
