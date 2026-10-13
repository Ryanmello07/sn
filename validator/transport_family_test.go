package validator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/urnetwork/connect"
)

// IPV6.md C7. Even requests try IPv4 first and odd ones IPv6, so the very
// first request is IPv4-first and the families never drift apart.
func TestHopFirstIpFamilyAlternatesFromIpv4(t *testing.T) {
	for request, want := range map[uint64]int{0: 4, 1: 6, 2: 4, 3: 6, math.MaxUint64 - 1: 4, math.MaxUint64: 6} {
		if got := hopFirstIpFamily(request); got != want {
			t.Fatalf("hopFirstIpFamily(%d) = %d, want %d", request, got, want)
		}
	}
}

// IPV6.md C7. Consecutive hop requests alternate their first family across
// hops, failures and retries of one hop. A request refused before it reaches
// a client, or whose registration fails, consumes no family.
func TestTunnelTransportAlternatesFirstIpFamilyAcrossHops(t *testing.T) {
	transport := newTestTunnelTransport(t)
	setupErr := errors.New("processed registration failed")
	hopErr := errors.New("hop request failed")
	failingHop, retriedHop := connect.NewId(), connect.NewId()
	client := &tunnelTestClient{ctx: t.Context(), close: func(context.Context) error { return nil }}
	client.post = func(_ context.Context, hop connect.Id, _ []byte) ([]byte, error) {
		if hop == failingHop {
			return nil, hopErr
		}
		return []byte("ok"), nil
	}
	creations := 0
	transport.newClient = func(context.Context, connect.Id) (tunnelTransportClient, error) {
		creations++
		if creations == 1 {
			return nil, setupErr
		}
		return client, nil
	}
	if _, err := transport.PostVerify(t.Context(), connect.NewId(), nil); !errors.Is(err, setupErr) {
		t.Fatalf("failed registration = %v", err)
	}
	if _, err := transport.PostVerify(t.Context(), transport.cfg.SourceClientId, nil); err == nil {
		t.Fatal("request to the validator itself was admitted")
	}
	for _, hop := range []connect.Id{connect.NewId(), failingHop, retriedHop, retriedHop, connect.NewId()} {
		_, err := transport.PostVerify(t.Context(), hop, nil)
		if hop == failingHop && !errors.Is(err, hopErr) || hop != failingHop && err != nil {
			t.Fatalf("hop %s = %v", hop, err)
		}
	}
	if want := []int{4, 6, 4, 6, 4}; !reflect.DeepEqual(client.firstIpFamilies, want) {
		t.Fatalf("first families = %v, want %v", client.firstIpFamilies, want)
	}
}

// IPV6.md C7. Through the real registered client, every hop's tunnel is the
// default tunnel with that request's first family, alternating from IPv4.
func TestTunnelTransportHopTunTriesAssignedFamily(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	claims, err := json.Marshal(map[string]string{"client_id": connect.NewId().String()})
	if err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims) + "."
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/network/auth-client":
			_ = json.NewEncoder(w).Encode(map[string]string{"by_client_jwt": token})
		case "/connect/control":
			_, _ = io.WriteString(w, `{"pack":"","error":null}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer endpoint.Close()
	settings := connect.DefaultClientStrategySettings()
	settings.EnableResilient = false
	strategy := connect.NewClientStrategy(ctx, settings)
	defer strategy.Close()
	transport := NewTunnelTransport(ctx, strategy, TunnelTransportConfig{ApiUrl: endpoint.URL, ConnectUrl: endpoint.URL, ByClientJwt: func() string { return token }, SourceClientId: connect.NewId()})
	defer func() {
		if err := transport.CloseAndWait(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	tunErr := errors.New("tunnel captured")
	var captured []*connect.TunSettings
	transport.createTun = func(_ context.Context, settings *connect.TunSettings) (*connect.Tun, error) {
		captured = append(captured, settings)
		return nil, tunErr
	}
	for range 3 {
		if _, err := transport.PostVerify(ctx, connect.NewId(), []byte("{}")); !errors.Is(err, tunErr) {
			t.Fatalf("hop request = %v, want the captured tunnel", err)
		}
	}
	if len(captured) != 3 {
		t.Fatalf("tunnels = %d, want 3", len(captured))
	}
	for index, want := range []int{4, 6, 4} {
		expected := connect.DefaultTunSettings()
		expected.DialFirstIpFamily = want
		if !reflect.DeepEqual(captured[index], expected) {
			t.Fatalf("tunnel %d settings = %+v, want the defaults with first family %d", index, captured[index], want)
		}
	}
}
