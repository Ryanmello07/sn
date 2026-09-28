package clientauth

// Registration is an immutable operation before its first request, then an
// exact server-issued identity before credential installation. Restart never
// guesses a legacy client ID or creates a second operation to clear an error.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/urnetwork/connect"
	"github.com/urnetwork/sdk"
)

const registrationRecordSchema = "urnetwork-durable-client-registration-v1"

// Stable deployment and client-key ownership selects the operation. No whole
// config hash, executable, token bytes, token timestamp or mutable policy does.
type RegistrationScope struct {
	Endpoint     string `json:"endpoint"`
	DeploymentId string `json:"deployment_id"`
	ChainId      uint64 `json:"chain_id"`
	GenesisHash  string `json:"genesis_hash"`
	Netuid       uint16 `json:"netuid"`
	ValidatorId  uint64 `json:"validator_id"`
	OperatorNoId uint64 `json:"operator_no_id"`
	ClientKey    string `json:"client_key"`
}

// Application refusals require operator/configuration recovery. Keeping their
// closed code observable never grants replacement registration authority.
type RegistrationRefusedError struct{ Code string }

func (self *RegistrationRefusedError) Error() string {
	return "client registration requires recovery: " + self.Code
}

// A retained token is custody, not evidence that a currently unreachable API
// still authorizes it. Native recovery remains independent of this readiness.
type RegistrationRefreshUnavailableError struct{ cause error }

func (self *RegistrationRefreshUnavailableError) Error() string {
	return "original client refresh remains unavailable"
}
func (self *RegistrationRefreshUnavailableError) Unwrap() error { return self.cause }

// Only a completed remote reply can produce this cause. Local registration or
// credential custody failures retain their original hard errors separately.
type RegistrationResponseIdentityError struct{}

func (self *RegistrationResponseIdentityError) Error() string {
	return "client response changed its original owned identity"
}

type registrationRecord struct {
	Schema    string                        `json:"schema"`
	Scope     RegistrationScope             `json:"scope"`
	Principal registrationPrincipal         `json:"principal"`
	Request   sdk.RegisterNetworkClientArgs `json:"request"`
	ClientId  string                        `json:"client_id,omitempty"`
	DeviceId  string                        `json:"device_id,omitempty"`
}

// Test hooks can fail only after actual durable ownership boundaries. They do
// not replace a read, write, identity validation, server allocation or fsync.
type registrationHooks struct {
	afterRequest func() error
	afterBinding func() error
}

// Production callers authorize creation only for an independently known new
// operator. A retained deployment with lost legacy credentials remains blocked
// for new API work while its native-liability observer can continue separately.
func LoadOrRegisterClientJwt(ctx context.Context, api *sdk.Api, networkPath, clientPath, description string, scope RegistrationScope, allowCreate bool) (string, connect.Id, error) {
	return loadOrRegisterClientJwt(ctx, api, networkPath, clientPath, description, scope, allowCreate, registrationHooks{})
}

func loadOrRegisterClientJwt(ctx context.Context, api *sdk.Api, networkPath, clientPath, description string, scope RegistrationScope, allowCreate bool, hooks registrationHooks) (_ string, _ connect.Id, returnErr error) {
	if ctx == nil || api == nil {
		return "", connect.Id{}, errors.New("registration has no operation owner")
	}
	if err := ctx.Err(); err != nil {
		return "", connect.Id{}, err
	}
	endpoint, err := api.NetworkClientRegistrationEndpoint()
	if err != nil {
		return "", connect.Id{}, err
	}
	if scope.Endpoint != endpoint || scope.DeploymentId == "" || scope.ChainId == 0 || scope.Netuid == 0 || scope.OperatorNoId == 0 || scope.ValidatorId == 0 {
		return "", connect.Id{}, errors.New("registration scope differs from its configured API or deployment")
	}
	for _, value := range []string{scope.GenesisHash, scope.ClientKey} {
		raw, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
		if err != nil || len(raw) != 32 || bytes.Equal(raw, make([]byte, 32)) || value != "0x"+hex.EncodeToString(raw) {
			return "", connect.Id{}, errors.New("registration scope has an invalid genesis or client key")
		}
	}
	owner, err := openRegistrationStore(clientPath)
	if err != nil {
		return "", connect.Id{}, err
	}
	defer func() { returnErr = errors.Join(returnErr, owner.close()) }()
	clientName, recordName := filepath.Base(clientPath), filepath.Base(clientPath)+".registration"
	anchorName := recordName + ".started"
	legacyName := recordName + ".existing"
	var record *registrationRecord
	raw, err := owner.read(recordName)
	if err == nil {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return "", connect.Id{}, fmt.Errorf("decode original registration: %w", err)
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return "", connect.Id{}, errors.New("original registration has trailing bytes")
		}
		if err := validateRegistrationRecord(record, scope, description); err != nil {
			return "", connect.Id{}, err
		}
		canonical, err := json.Marshal(record)
		if err != nil || !bytes.Equal(canonical, raw) {
			return "", connect.Id{}, errors.New("original registration is not its canonical unambiguous encoding")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", connect.Id{}, err
	}
	anchor, anchorErr := owner.read(anchorName)
	if anchorErr == nil {
		if record == nil {
			return "", connect.Id{}, &RegistrationRefusedError{Code: "registration_custody_missing_after_send"}
		}
		expected, err := registrationOperationAnchor(record)
		if err != nil || !bytes.Equal(anchor, expected) {
			return "", connect.Id{}, errors.New("registration request differs from its durable first-send anchor")
		}
	} else if !errors.Is(anchorErr, os.ErrNotExist) {
		return "", connect.Id{}, anchorErr
	}
	legacy, legacyErr := owner.read(legacyName)
	if legacyErr != nil && !errors.Is(legacyErr, os.ErrNotExist) {
		return "", connect.Id{}, legacyErr
	}
	if record != nil && legacyErr == nil {
		return "", connect.Id{}, errors.New("registration has both original creation and existing-client custody")
	}
	if raw, err := owner.read(clientName); err == nil {
		token := strings.TrimSpace(string(raw))
		if record != nil {
			if err := validateRegistrationCredential(record, token, record.ClientId, record.DeviceId); err != nil {
				return "", connect.Id{}, err
			}
		} else {
			identity, err := registrationIdentity(token)
			if err != nil || !validRegistrationIdentityId(identity.clientId) || !validRegistrationIdentityId(identity.deviceId) {
				return "", connect.Id{}, errors.New("existing client custody has invalid identity")
			}
			// Observing existing custody consumes no creation authority. Its
			// stable marker survives token loss and excludes bearer timestamps.
			expected, err := json.Marshal(struct {
				Schema    string                `json:"schema"`
				Scope     RegistrationScope     `json:"scope"`
				Principal registrationPrincipal `json:"principal"`
				ClientId  string                `json:"client_id"`
				DeviceId  string                `json:"device_id"`
			}{Schema: "urnetwork-existing-client-v1", Scope: scope, Principal: identity.registrationPrincipal, ClientId: identity.clientId, DeviceId: identity.deviceId})
			if err != nil {
				return "", connect.Id{}, err
			}
			if legacyErr == nil && !bytes.Equal(legacy, expected) {
				return "", connect.Id{}, errors.New("existing client differs from its original identity custody")
			}
			if errors.Is(legacyErr, os.ErrNotExist) {
				if err := owner.write(legacyName, expected); err != nil {
					return "", connect.Id{}, err
				}
			}
		}
		return refreshStoredClientJwtWithReadiness(ctx, api, networkPath, clientPath, token, func(token string) error { return owner.write(clientName, []byte(token)) }, false)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", connect.Id{}, err
	}
	if legacyErr == nil {
		return "", connect.Id{}, &RegistrationRefusedError{Code: "legacy_identity_requires_explicit_recovery"}
	}
	if record == nil && !allowCreate {
		return "", connect.Id{}, &RegistrationRefusedError{Code: "legacy_identity_requires_explicit_recovery"}
	}
	bootstrap, err := ReadToken(networkPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", connect.Id{}, &RegistrationRefusedError{Code: "network_authentication_missing"}
		}
		return "", connect.Id{}, err
	}
	identity, err := registrationIdentity(bootstrap)
	if err != nil || !validRegistrationIdentityId(identity.NetworkId) || !validRegistrationIdentityId(identity.UserId) || identity.clientId != "" || identity.deviceId != "" {
		return "", connect.Id{}, errors.New("registration bootstrap does not name a network principal")
	}
	if marker, err := ReadToken(rejectionPath(clientPath)); err == nil {
		if marker == "blocked" || marker == networkCredentialFingerprint(networkPath, bootstrap) {
			return "", connect.Id{}, &RegistrationRefusedError{Code: "client_revoked"}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", connect.Id{}, err
	}
	if record == nil {
		var requestId [32]byte
		if _, err := rand.Read(requestId[:]); err != nil {
			return "", connect.Id{}, err
		}
		scopeRaw, err := json.Marshal(scope)
		if err != nil {
			return "", connect.Id{}, err
		}
		digest := sha256.Sum256(scopeRaw)
		record = &registrationRecord{Schema: registrationRecordSchema, Scope: scope, Principal: identity.registrationPrincipal,
			Request: sdk.RegisterNetworkClientArgs{Schema: sdk.NetworkClientRegistrationSchema, RegistrationId: hex.EncodeToString(requestId[:]), ScopeSha256: hex.EncodeToString(digest[:]), DeviceDescription: description}}
		if err := validateRegistrationRecord(record, scope, description); err != nil {
			return "", connect.Id{}, err
		}
		raw, err := json.Marshal(record)
		if err != nil {
			return "", connect.Id{}, err
		}
		if err := owner.write(recordName, raw); err != nil {
			return "", connect.Id{}, err
		}
		if hooks.afterRequest != nil {
			if err := hooks.afterRequest(); err != nil {
				return "", connect.Id{}, err
			}
		}
	}
	if !record.Principal.equal(identity.registrationPrincipal) {
		return "", connect.Id{}, errors.New("registration bootstrap differs from the original principal")
	}
	if errors.Is(anchorErr, os.ErrNotExist) {
		anchor, err := registrationOperationAnchor(record)
		if err != nil {
			return "", connect.Id{}, err
		}
		if err := owner.write(anchorName, anchor); err != nil {
			return "", connect.Id{}, err
		}
	}
	api.SetByJwt(bootstrap)
	result, err := api.RegisterNetworkClientSyncWithContext(ctx, &record.Request)
	if err != nil {
		return "", connect.Id{}, err
	}
	if result.Error != nil {
		return "", connect.Id{}, &RegistrationRefusedError{Code: result.Error.Code}
	}
	clientId, deviceId := result.ClientId.String(), result.DeviceId.String()
	if err := validateRegistrationCredential(record, result.ByClientJwt, clientId, deviceId); err != nil {
		return "", connect.Id{}, &RegistrationResponseIdentityError{}
	}
	if record.ClientId == "" {
		record.ClientId, record.DeviceId = clientId, deviceId
		raw, err := json.Marshal(record)
		if err != nil {
			return "", connect.Id{}, err
		}
		if err := owner.write(recordName, raw); err != nil {
			return "", connect.Id{}, err
		}
		if hooks.afterBinding != nil {
			if err := hooks.afterBinding(); err != nil {
				return "", connect.Id{}, err
			}
		}
	}
	if err := owner.write(clientName, []byte(result.ByClientJwt)); err != nil {
		return "", connect.Id{}, err
	}
	if err := clearRejection(clientPath); err != nil {
		return "", connect.Id{}, err
	}
	api.SetByJwt(result.ByClientJwt)
	id, _ := connect.ParseId(clientId)
	return result.ByClientJwt, id, nil
}

// Independent pre-send custody distinguishes a never-sent initialization from
// a missing operation after possible server mutation. Server-side unique scope
// additionally refuses duplicate allocation if all local custody is lost.
func registrationOperationAnchor(record *registrationRecord) ([]byte, error) {
	original := *record
	original.ClientId, original.DeviceId = "", ""
	raw, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	return []byte(registrationRecordSchema + "\n" + record.Request.RegistrationId + "\n" + hex.EncodeToString(digest[:])), nil
}

// Every restart checks the complete original scope and payload, including the
// old description. Configuration changes cannot silently regenerate either.
func validateRegistrationRecord(record *registrationRecord, scope RegistrationScope, description string) error {
	if record == nil || record.Schema != registrationRecordSchema || !reflect.DeepEqual(record.Scope, scope) || record.Request.DeviceDescription != description || record.Request.DeviceSpec != "" || !validRegistrationIdentityId(record.Principal.NetworkId) || !validRegistrationIdentityId(record.Principal.UserId) {
		return errors.New("original registration scope or principal is invalid or changed")
	}
	raw, err := json.Marshal(scope)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	if record.Request.ScopeSha256 != hex.EncodeToString(digest[:]) {
		return errors.New("original registration scope hash differs")
	}
	if _, err := sdk.EncodeNetworkClientRegistration(&record.Request); err != nil {
		return err
	}
	if (record.ClientId != "" || record.DeviceId != "") && (!validRegistrationIdentityId(record.ClientId) || !validRegistrationIdentityId(record.DeviceId)) {
		return errors.New("original registration has a partial client/device binding")
	}
	return nil
}

// The server-issued client/device claims must agree with both its response
// and any earlier durable binding. There is no response-selected replacement.
func validateRegistrationCredential(record *registrationRecord, token, clientId, deviceId string) error {
	identity, err := registrationIdentity(token)
	if err != nil || !validRegistrationIdentityId(clientId) || !validRegistrationIdentityId(deviceId) || identity.clientId != clientId || identity.deviceId != deviceId || !record.Principal.equal(identity.registrationPrincipal) || record.ClientId != "" && (record.ClientId != clientId || record.DeviceId != deviceId) {
		return errors.New("registered credential differs from its original principal or client/device binding")
	}
	return nil
}
