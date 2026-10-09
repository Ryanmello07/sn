//go:build linux || darwin

package clientauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// networkCredentialFingerprint is the path fingerprint that markers written
// before readNetworkCredential hold: the token and the modification time of
// a separate stat of the path.
func networkCredentialFingerprint(path string, byJwt string) string {
	info, err := os.Stat(path)
	if err != nil {
		return networkCredentialFingerprintOf(byJwt, 0, false)
	}
	return networkCredentialFingerprintOf(byJwt, info.ModTime().UnixNano(), true)
}

func networkTokenTestJwt(t *testing.T, claims gojwt.MapClaims) string {
	t.Helper()
	token, err := gojwt.NewWithClaims(gojwt.SigningMethodNone, claims).SignedString(gojwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func networkTokenTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// a bootstrap server whose client refresh is rejected and whose bootstrap
// counts every new-client request it receives
type networkTokenBootstrapServer struct {
	stateLock    sync.Mutex
	bootstraps   []string
	replacement  string
	refreshValue string
}

func (self *networkTokenBootstrapServer) serve(t *testing.T) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hello":
			w.WriteHeader(http.StatusOK)
		case "/auth/refresh":
			http.Error(w, "not authorized", http.StatusUnauthorized)
		case "/network/auth-client":
			self.stateLock.Lock()
			self.bootstraps = append(self.bootstraps, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			replacement := self.replacement
			self.stateLock.Unlock()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"by_client_jwt":%q}`, replacement)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func (self *networkTokenBootstrapServer) bootstrapCount() int {
	self.stateLock.Lock()
	defer self.stateLock.Unlock()
	return len(self.bootstraps)
}

// The marker hand-off. A client the server rejected stays blocked however many
// times its network token is renewed, because each renewal keeps the token in
// its lineage; only an explicit sign-in unblocks it, as before renewals.
func TestRenewedNetworkTokenKeepsARevokedClientBlocked(t *testing.T) {
	bootstrap := &networkTokenBootstrapServer{replacement: testClientJwt(t, "replacement")}
	server := bootstrap.serve(t)
	dir := networkTokenTestDir(t)
	networkPath := filepath.Join(dir, "jwt")
	clientPath := filepath.Join(dir, "client.jwt")
	if err := WriteNetworkToken(networkPath, "network-jwt"); err != nil {
		t.Fatal(err)
	}
	if err := WriteToken(clientPath, testClientJwt(t, "rejected")); err != nil {
		t.Fatal(err)
	}
	api, closeApi := testApi(t, server.URL)
	defer closeApi()
	if _, _, err := LoadOrCreateClientJwt(context.Background(), api, networkPath, clientPath, "test"); err == nil {
		t.Fatal("confirmed client rejection silently recreated the revoked client")
	}

	previous := "network-jwt"
	for i := range 3 {
		renewed := fmt.Sprintf("network-jwt-renewed-%d", i)
		ok, err := RenewNetworkToken(networkPath, previous, renewed)
		if err != nil || !ok {
			t.Fatalf("renewal %d: ok=%t err=%v", i, ok, err)
		}
		previous = renewed
		if _, _, err := LoadOrCreateClientJwt(context.Background(), api, networkPath, clientPath, "test"); err == nil || !strings.Contains(err.Error(), "run the auth command") {
			t.Fatalf("after renewal %d the revoked client was not refused: %v", i, err)
		}
		if got := bootstrap.bootstrapCount(); got != 0 {
			t.Fatalf("after renewal %d a renewed sign-in recreated the revoked client (%d bootstraps)", i, got)
		}
	}

	// an explicit sign-in is the deliberate recovery, exactly as before
	if err := WriteNetworkToken(networkPath, "new-network-jwt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(networkPath + ".lineage"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an explicit sign-in kept the old lineage: %v", err)
	}
	if _, _, err := LoadOrCreateClientJwt(context.Background(), api, networkPath, clientPath, "test"); err != nil {
		t.Fatalf("explicit sign-in did not unblock: %v", err)
	}
	if got := bootstrap.bootstrapCount(); got != 1 {
		t.Fatalf("bootstraps = %d, want 1 with the new sign-in", got)
	}
}

// The same rule on the registration path a production validator uses (path
// custody): a renewed bootstrap never replays the revoked client's original
// registration.
func TestRenewedNetworkTokenKeepsARevokedRegistrationBlocked(t *testing.T) {
	fixture := newRegistrationTestFixture(t)
	if _, err := fixture.register(t.Context(), true, registrationHooks{}); err != nil {
		t.Fatal(err)
	}
	fixture.stateLock.Lock()
	fixture.refreshStatus = http.StatusUnauthorized
	fixture.stateLock.Unlock()
	var refused *RegistrationRefusedError
	if _, err := fixture.register(t.Context(), true, registrationHooks{}); !errors.As(err, &refused) || refused.Code != "client_revoked" {
		t.Fatalf("rejected client was not revoked: %v", err)
	}
	posts := fixture.postCount()

	original, err := ReadToken(fixture.networkPath)
	if err != nil {
		t.Fatal(err)
	}
	renewed := registrationTestToken(t, "", "renewed")
	if ok, err := RenewNetworkToken(fixture.networkPath, original, renewed); err != nil || !ok {
		t.Fatalf("renewal: ok=%t err=%v", ok, err)
	}
	if _, err := fixture.register(t.Context(), true, registrationHooks{}); !errors.As(err, &refused) || refused.Code != "client_revoked" {
		t.Fatalf("a renewed bootstrap replayed the revoked registration: %v", err)
	}
	if got := fixture.postCount(); got != posts {
		t.Fatalf("registration posts = %d, want %d", got, posts)
	}
}

// A crash at each durable step of a renewal leaves a state that still blocks,
// and the next renewal completes from it.
func TestNetworkTokenRenewalCrashAtEachStepNeverUnblocks(t *testing.T) {
	for _, step := range []string{"temporary", "lineage"} {
		t.Run(step, func(t *testing.T) {
			dir := networkTokenTestDir(t)
			networkPath := filepath.Join(dir, "jwt")
			clientPath := filepath.Join(dir, "client.jwt")
			if err := WriteNetworkToken(networkPath, "network-jwt"); err != nil {
				t.Fatal(err)
			}
			if err := MarkRejected(clientPath, networkPath); err != nil {
				t.Fatal(err)
			}
			blocked := func(t *testing.T) {
				t.Helper()
				_, fingerprint, err := readNetworkCredential(networkPath)
				if err != nil {
					t.Fatal(err)
				}
				marker, err := ReadToken(rejectionPath(clientPath))
				if err != nil {
					t.Fatal(err)
				}
				blocks, err := networkCredentialMarkerBlocks(networkPath, fingerprint, marker)
				if err != nil || !blocks {
					t.Fatalf("marker blocks=%t err=%v", blocks, err)
				}
			}

			stop := func() bool { return true }
			switch step {
			case "temporary":
				networkRenewalTestHooks.afterTemporary = stop
			case "lineage":
				networkRenewalTestHooks.afterLineage = stop
			}
			ok, err := RenewNetworkToken(networkPath, "network-jwt", "network-jwt-renewed")
			networkRenewalTestHooks.afterTemporary, networkRenewalTestHooks.afterLineage = nil, nil
			if ok || !errors.Is(err, errNetworkRenewalTestStop) {
				t.Fatalf("the hook did not stop the renewal: ok=%t err=%v", ok, err)
			}
			if token, _ := ReadToken(networkPath); token != "network-jwt" {
				t.Fatalf("a stopped renewal replaced the token: %q", token)
			}
			blocked(t)

			// the next renewal removes the leftover and completes
			ok, err = RenewNetworkToken(networkPath, "network-jwt", "network-jwt-renewed")
			if err != nil || !ok {
				t.Fatalf("resumed renewal: ok=%t err=%v", ok, err)
			}
			names, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range names {
				if strings.HasPrefix(entry.Name(), networkRenewalLeftoverPrefix("jwt")) {
					t.Fatalf("the leftover %s was not removed", entry.Name())
				}
			}
			blocked(t)
		})
	}
}

// Markers written before lineages existed, by the path fingerprint, keep
// blocking; "blocked" markers block whatever the token, as before.
func TestNetworkTokenRenewalHonorsOldMarkers(t *testing.T) {
	dir := networkTokenTestDir(t)
	networkPath := filepath.Join(dir, "jwt")
	if err := WriteToken(networkPath, "network-jwt"); err != nil {
		t.Fatal(err)
	}
	legacyMarker := networkCredentialFingerprint(networkPath, "network-jwt")
	if ok, err := RenewNetworkToken(networkPath, "network-jwt", "network-jwt-renewed"); err != nil || !ok {
		t.Fatalf("renewal: ok=%t err=%v", ok, err)
	}
	_, fingerprint, err := readNetworkCredential(networkPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{legacyMarker, "blocked"} {
		if blocks, err := networkCredentialMarkerBlocks(networkPath, fingerprint, marker); err != nil || !blocks {
			t.Fatalf("marker %.12s: blocks=%t err=%v", marker, blocks, err)
		}
	}
	// a marker of an unrelated earlier sign-in does not block the current one
	if blocks, err := networkCredentialMarkerBlocks(networkPath, fingerprint, strings.Repeat("ab", 32)); err != nil || blocks {
		t.Fatalf("an unrelated marker blocked: blocks=%t err=%v", blocks, err)
	}
	// "blocked" survives an explicit sign-in too
	if err := WriteNetworkToken(networkPath, "new-network-jwt"); err != nil {
		t.Fatal(err)
	}
	_, fingerprint, err = readNetworkCredential(networkPath)
	if err != nil {
		t.Fatal(err)
	}
	if blocks, _ := networkCredentialMarkerBlocks(networkPath, fingerprint, "blocked"); !blocks {
		t.Fatal(`an explicit sign-in unblocked a "blocked" marker`)
	}
	if blocks, _ := networkCredentialMarkerBlocks(networkPath, fingerprint, legacyMarker); blocks {
		t.Fatal("an explicit sign-in did not unblock the old sign-in's marker")
	}
}

// A renewal applies only to the token it renews; a sign-in that replaced the
// file first wins. A held registration owner lock is a busy, retryable answer.
func TestRenewNetworkTokenComparesAndRespectsTheOwnerLock(t *testing.T) {
	dir := networkTokenTestDir(t)
	networkPath := filepath.Join(dir, "jwt")
	if err := WriteNetworkToken(networkPath, "explicit-sign-in"); err != nil {
		t.Fatal(err)
	}
	if ok, err := RenewNetworkToken(networkPath, "older-token", "renewed"); err != nil || ok {
		t.Fatalf("stale renewal: ok=%t err=%v", ok, err)
	}
	if token, _ := ReadToken(networkPath); token != "explicit-sign-in" {
		t.Fatalf("a stale renewal replaced the sign-in: %q", token)
	}

	owner, err := openRegistrationStore(networkPath)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := RenewNetworkToken(networkPath, "explicit-sign-in", "renewed"); ok || !errors.Is(err, ErrNetworkTokenBusy) {
		t.Fatalf("renewal under a held owner lock: ok=%t err=%v", ok, err)
	}
	if err := owner.close(); err != nil {
		t.Fatal(err)
	}
	if ok, err := RenewNetworkToken(networkPath, "explicit-sign-in", "renewed"); err != nil || !ok {
		t.Fatalf("renewal after the lock: ok=%t err=%v", ok, err)
	}
	if token, _ := ReadToken(networkPath); token != "renewed" {
		t.Fatalf("token = %q", token)
	}
	for _, bad := range [][2]string{{"", "renewed"}, {"renewed", ""}, {"renewed", "renewed"}} {
		if ok, err := RenewNetworkToken(networkPath, bad[0], bad[1]); ok || err == nil {
			t.Fatalf("renewal %q -> %q: ok=%t err=%v", bad[0], bad[1], ok, err)
		}
	}
}

func TestNetworkTokenLineageIsBoundedAndStrict(t *testing.T) {
	members := []string{strings.Repeat("00", 32)}
	for i := 1; i <= maximumNetworkCredentialLineage+10; i++ {
		members = appendNetworkCredentialLineage(members, fmt.Sprintf("%064x", i))
	}
	if len(members) != maximumNetworkCredentialLineage || members[0] != strings.Repeat("00", 32) || members[len(members)-1] != fmt.Sprintf("%064x", maximumNetworkCredentialLineage+10) {
		t.Fatalf("bound kept %d members, root %.8s, newest %.8s", len(members), members[0], members[len(members)-1])
	}
	if again := appendNetworkCredentialLineage(members, members[5]); len(again) != len(members) {
		t.Fatal("a repeated member grew the lineage")
	}
	encoded, err := encodeNetworkCredentialLineage(members)
	if err != nil || len(encoded) > maximumRegistrationBytes {
		t.Fatalf("encoded lineage of %d bytes exceeds the store bound: %v", len(encoded), err)
	}
	if parsed, err := parseNetworkCredentialLineage(encoded); err != nil || !slices.Equal(parsed, members) {
		t.Fatalf("round trip: %v", err)
	}
	for _, bad := range []string{
		`{"schema":"other","members":["` + strings.Repeat("00", 32) + `"]}`,
		`{"schema":"urnetwork-network-jwt-lineage-v1","members":[]}`,
		`{"schema":"urnetwork-network-jwt-lineage-v1","members":["XYZ"]}`,
		`{"schema":"urnetwork-network-jwt-lineage-v1","members":["` + strings.Repeat("AB", 32) + `"]}`,
		`{"schema":"urnetwork-network-jwt-lineage-v1","members":["` + strings.Repeat("00", 32) + `"],"extra":1}`,
		`{"schema":"urnetwork-network-jwt-lineage-v1","members":["` + strings.Repeat("00", 32) + `"]} {}`,
	} {
		if _, err := parseNetworkCredentialLineage([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}

	// an unreadable lineage blocks and reports itself
	dir := networkTokenTestDir(t)
	networkPath := filepath.Join(dir, "jwt")
	if err := WriteToken(networkPath, "network-jwt"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(networkPath+".lineage", []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	_, fingerprint, err := readNetworkCredential(networkPath)
	if err != nil {
		t.Fatal(err)
	}
	if blocks, err := networkCredentialMarkerBlocks(networkPath, fingerprint, strings.Repeat("ab", 32)); err == nil || !blocks {
		t.Fatalf("malformed lineage: blocks=%t err=%v", blocks, err)
	}
}

// The fingerprint read from one open file equals the path fingerprint that
// existing markers hold.
func TestReadNetworkCredentialMatchesThePathFingerprint(t *testing.T) {
	dir := networkTokenTestDir(t)
	networkPath := filepath.Join(dir, "jwt")
	if err := WriteToken(networkPath, " network-jwt \n"); err != nil {
		t.Fatal(err)
	}
	token, fingerprint, err := readNetworkCredential(networkPath)
	if err != nil || token != "network-jwt" || fingerprint != networkCredentialFingerprint(networkPath, token) {
		t.Fatalf("token=%q fingerprint=%.12s err=%v", token, fingerprint, err)
	}
	if err := os.WriteFile(networkPath, []byte("  \n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readNetworkCredential(networkPath); err == nil {
		t.Fatal("an empty token was read")
	}
}

func TestValidateRenewedNetworkJwt(t *testing.T) {
	network := gojwt.MapClaims{"network_id": "00000000-0000-0000-0000-000000000301", "user_id": "00000000-0000-0000-0000-000000000401", "roles": []string{"validator", "operator"}, "principal": "owner"}
	with := func(changes gojwt.MapClaims) string {
		claims := gojwt.MapClaims{}
		for key, value := range network {
			claims[key] = value
		}
		for key, value := range changes {
			if value == nil {
				delete(claims, key)
			} else {
				claims[key] = value
			}
		}
		return networkTokenTestJwt(t, claims)
	}
	original := with(nil)
	if err := ValidateRenewedNetworkJwt(original, with(gojwt.MapClaims{"roles": []string{"operator", "validator"}, "iat": 2})); err != nil {
		t.Fatalf("a renewal of the same identity was refused: %v", err)
	}
	for name, renewed := range map[string]string{
		"another network": with(gojwt.MapClaims{"network_id": "00000000-0000-0000-0000-000000000302"}),
		"another user":    with(gojwt.MapClaims{"user_id": "00000000-0000-0000-0000-000000000402"}),
		"other roles":     with(gojwt.MapClaims{"roles": []string{"operator"}}),
		"other principal": with(gojwt.MapClaims{"principal": "someone"}),
		"a client token":  with(gojwt.MapClaims{"client_id": testClientId, "device_id": testDeviceId}),
		"not a jwt":       "renewed",
	} {
		if err := ValidateRenewedNetworkJwt(original, renewed); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if err := ValidateRenewedNetworkJwt(with(gojwt.MapClaims{"client_id": testClientId, "device_id": testDeviceId}), original); err == nil {
		t.Error("a client token was renewed as a network token")
	}
}

func TestNetworkJwtRenewable(t *testing.T) {
	network := gojwt.MapClaims{"network_id": "00000000-0000-0000-0000-000000000301", "user_id": "00000000-0000-0000-0000-000000000401"}
	if !NetworkJwtRenewable(networkTokenTestJwt(t, network)) {
		t.Fatal("a network token was not renewable")
	}
	for name, token := range map[string]string{
		"an api key":       "urn_" + strings.Repeat("a", 52),
		"a client token":   networkTokenTestJwt(t, gojwt.MapClaims{"network_id": network["network_id"], "user_id": network["user_id"], "client_id": testClientId, "device_id": testDeviceId}),
		"no network":       networkTokenTestJwt(t, gojwt.MapClaims{"user_id": network["user_id"]}),
		"no user":          networkTokenTestJwt(t, gojwt.MapClaims{"network_id": network["network_id"]}),
		"not a jwt":        "network-jwt",
		"an invalid claim": networkTokenTestJwt(t, gojwt.MapClaims{"network_id": 7, "user_id": network["user_id"]}),
	} {
		if NetworkJwtRenewable(token) {
			t.Errorf("%s was renewable", name)
		}
	}
}

// A confirmed 401 on the request that carried the network token is a typed
// rejection of the sign-in, on both bootstrap paths.
func TestNetworkTokenRejectionIsTyped(t *testing.T) {
	fixture := newRegistrationTestFixture(t)
	fixture.stateLock.Lock()
	fixture.status = http.StatusUnauthorized
	fixture.stateLock.Unlock()
	_, err := fixture.register(t.Context(), true, registrationHooks{})
	if !IsNetworkCredentialRejected(err) {
		t.Fatalf("registration 401 err = %v, want a network credential rejection", err)
	}
	fixture.stateLock.Lock()
	fixture.status = http.StatusServiceUnavailable
	fixture.stateLock.Unlock()
	if _, err := fixture.register(t.Context(), true, registrationHooks{}); IsNetworkCredentialRejected(err) {
		t.Fatalf("registration 503 was typed as a rejected sign-in: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hello":
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "not authorized", http.StatusUnauthorized)
		}
	}))
	defer server.Close()
	dir := networkTokenTestDir(t)
	networkPath := filepath.Join(dir, "jwt")
	if err := WriteNetworkToken(networkPath, "network-jwt"); err != nil {
		t.Fatal(err)
	}
	api, closeApi := testApi(t, server.URL)
	defer closeApi()
	if _, _, err := LoadOrCreateClientJwt(context.Background(), api, networkPath, filepath.Join(dir, "client.jwt"), "test"); !IsNetworkCredentialRejected(err) {
		t.Fatalf("bootstrap 401 err = %v, want a network credential rejection", err)
	}
}
