//go:build linux || darwin

// Optional startup wallet work uses the actual newly authenticated slot.
// The observer stops only after the signed consent was durably acknowledged.
package miner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/urfoundation/sn/clientauth"
	"github.com/urfoundation/sn/ss58"
	"github.com/urnetwork/connect"
	"github.com/urnetwork/sdk"
)

// Both the first direct allocation and a proxy allocation reach the real
// consent producer after registration; neither can borrow the bootstrap JWT.
func TestProviderStartupWalletUsesFirstDirectCredential(t *testing.T) {
	providerStartupWalletUsesAuthenticatedSlot(t, false)
}

// A proxy has its own retained credential even beside an unrelated direct one.
func TestProviderStartupWalletUsesProxyCredential(t *testing.T) {
	providerStartupWalletUsesAuthenticatedSlot(t, true)
}

// This helper runs one complete independent startup per top-level root.
func providerStartupWalletUsesAuthenticatedSlot(t *testing.T, proxySlot bool) {
	t.Helper()
	fixture := newProviderRegistrationFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var proxy *connect.ProxySettings
	unrelated := providerRegistrationTestToken(t, "00000000-0000-0000-0000-000000000999", "unrelated-direct")
	if proxySlot {
		proxy = &connect.ProxySettings{Network: "tcp", Address: "192.0.2.25:1080"}
		if err := clientauth.WriteToken(filepath.Join(fixture.dir, ".provider.jwt"), unrelated); err != nil {
			t.Fatal(err)
		}
	}
	selectedPath, err := providerClientJwtPath(proxy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(selectedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fixture did not begin before provider registration", err)
	}
	seedPath := writeTestSeedFile(t, testAliceSeedHex)
	coldkey, err := ss58.DecodeWithPrefix(testAliceAddress, ss58.BittensorPrefix)
	if err != nil {
		t.Fatal(err)
	}
	var ownerLock sync.Mutex
	var authenticated string
	var retainedSeed []byte
	var writes, challenges atomic.Int32
	fixture.stateLock.Lock()
	fixture.walletHandler = func(w http.ResponseWriter, request *http.Request) {
		ownerLock.Lock()
		token := authenticated
		ownerLock.Unlock()
		if token == "" || token == unrelated || request.Header.Get("Authorization") != "Bearer "+token {
			t.Error("wallet escaped before its actual provider authenticated")
			http.Error(w, "wrong provider", http.StatusForbidden)
			return
		}
		switch request.URL.Path {
		case "/sn/epoch":
			// This is the actual Server decimal-string no_id wire shape.
			_, _ = w.Write([]byte(`{"epoch":2,"no_id":"13"}`))
		case "/sn/wallet/consent":
			challenges.Add(1)
			var args sdk.SnWalletMappingChallengeArgs
			id, idErr := clientauth.ClientIdFromJwt(token)
			if err := json.NewDecoder(request.Body).Decode(&args); err != nil || idErr != nil || args.ClientId == nil || args.ClientId.String() != id.String() || args.ColdkeySs58 != testAliceAddress || args.FromEpoch != 3 || args.ThroughEpoch != 65538 {
				t.Error("startup wallet changed its provider or default earning interval", err)
				http.Error(w, "scope", 400)
				return
			}
			_ = json.NewEncoder(w).Encode(sdk.SnWalletMappingChallengeResult{Message: financialWalletMappingTestMessage(t, token, coldkey, args.FromEpoch, args.ThroughEpoch, 29)})
		case "/sn/wallet":
			var args sdk.SnSetWalletArgs
			if err := json.NewDecoder(request.Body).Decode(&args); err != nil {
				t.Error(err)
				http.Error(w, "body", 400)
				return
			}
			original, receipt, err := financialWalletMappingTestReceipt(request.Context(), &args)
			if err != nil {
				t.Error(err)
				http.Error(w, "signature", 400)
				return
			}
			var journal snWalletConsentJournal
			raw, readErr := os.ReadFile(filepath.Join(selectedPath+".wallet-consent", "history.json"))
			if readErr != nil || json.Unmarshal(raw, &journal) != nil || len(journal.Records) != 1 || journal.Records[0].Original != original || journal.Records[0].Acknowledged {
				t.Error("startup POST preceded original signed custody", readErr)
			}
			writes.Add(1)
			_ = json.NewEncoder(w).Encode(receipt)
		default:
			t.Error("startup used a projection-only wallet route", request.URL.Path)
			http.Error(w, "unexpected", 400)
		}
	}
	fixture.stateLock.Unlock()
	stop := errors.New("synthetic completed wallet handoff")
	hooks := providerRegistrationHooks{afterAuthenticated: func(token string, _ connect.Id, seed []byte) error {
		ownerLock.Lock()
		authenticated, retainedSeed = token, bytes.Clone(seed)
		ownerLock.Unlock()
		return nil
	}, afterWallet: func(err error) error {
		if err != nil {
			t.Error("actual startup wallet failed", err)
		}
		return stop
	}}
	settings := fixture.settings(true, proxy)
	settings.wallet, settings.walletProof = testAliceAddress, &snWalletProof{SeedFile: seedPath}
	if err := settings.run(context.WithValue(ctx, providerRegistrationHooksKey{}, hooks), &providerRefusedWriter{}); !errors.Is(err, stop) {
		t.Fatal("startup did not complete the real wallet handoff", err)
	}
	posts, legacy, allocations, _ := fixture.counts()
	if posts != 1 || legacy != 0 || allocations != 1 || writes.Load() != 1 || challenges.Load() != 1 {
		t.Fatal("startup wallet changed provider allocation or consent count", posts, legacy, allocations, writes.Load(), challenges.Load())
	}
	token, err := clientauth.ReadToken(selectedPath)
	if err != nil || token != authenticated {
		t.Fatal("wallet changed selected provider credential", err)
	}
	seed, err := os.ReadFile(filepath.Join(fixture.dir, ".provider.key"))
	if err != nil || !bytes.Equal(seed, retainedSeed) {
		t.Fatal("wallet changed provider key custody", err)
	}
	if proxySlot {
		other, err := os.ReadFile(filepath.Join(fixture.dir, ".provider.jwt"))
		if err != nil || strings.TrimSpace(string(other)) != unrelated {
			t.Fatal("proxy wallet rewrote unrelated direct credential", err)
		}
	}
}
