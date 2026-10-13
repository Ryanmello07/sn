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

// IPV6.md C7. Families alternate along a trail, and each trail starts on the
// family the trail before it did not: trail 0 starts on IPv4, and the rule
// holds across the counter's wrap.
func TestHopFirstIpFamilyAlternatesAlongAndAcrossTrails(t *testing.T) {
	for _, c := range []struct {
		trail uint64
		want  []int
	}{
		{0, []int{4, 6, 4, 6, 4}},
		{1, []int{6, 4, 6, 4, 6}},
		{2, []int{4, 6, 4, 6, 4}},
		{math.MaxUint64, []int{6, 4, 6, 4, 6}},
	} {
		got := []int{}
		for position := range c.want {
			got = append(got, hopFirstIpFamily(c.trail, position))
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("trail %d tries %v first, want %v", c.trail, got, c.want)
		}
	}
}

// trailFamilyTransport is the mock /verify server, recording the family each
// hop request tries first.
type trailFamilyTransport struct {
	server          *mockVerifyServer
	firstIpFamilies []int
}

func (self *trailFamilyTransport) PostVerify(ctx context.Context, hop connect.Id, body []byte) ([]byte, error) {
	self.firstIpFamilies = append(self.firstIpFamilies, hopFirstIpFamilyOf(ctx))
	return self.server.PostVerify(ctx, hop, body)
}

// IPV6.md C7. Through the engine, with an even and an odd depth: every hop of
// a trail tries the other family first from the hop before it, and the second
// trail starts on the family the first did not, so over the two trails every
// position -- the SEED and each recorded hop -- tries each family first once.
func TestTrailHopsAlternateFirstIpFamilyAlongAndAcrossTrails(t *testing.T) {
	for _, m := range []int{4, 5} {
		server, validatorKey, clientId := newMockVerifyServer(t, 12)
		transport := &trailFamilyTransport{server: server}
		seedHop := server.providers[0]
		engine := NewTrailEngine(
			clientId,
			validatorKey,
			transport,
			NewStaticServerKeyRing(server.serverPublicKeys()),
			func(ctx context.Context) (connect.Id, error) { return seedHop, nil },
			NewStatsEngine(StatsConfig{AMin: 1}),
			nil,
			func() uint64 { return 42 },
			TrailEngineConfig{M: m, StepTimeout: 2 * time.Second, ExtendAttempts: 3, Pace: time.Millisecond},
		)
		trails := [][]int{}
		for range 2 {
			before := len(transport.firstIpFamilies)
			if _, err := engine.RunTrail(context.Background()); err != nil {
				t.Fatalf("M=%d: %v", m, err)
			}
			trails = append(trails, transport.firstIpFamilies[before:])
		}
		for i, trail := range trails {
			if len(trail) != m {
				t.Fatalf("M=%d: trail %d made %d hop requests, want %d", m, i, len(trail), m)
			}
			for position := 1; position < m; position++ {
				if trail[position] == trail[position-1] {
					t.Fatalf("M=%d: trail %d tries %v first: hops %d and %d try the same family", m, i, trail, position-1, position)
				}
			}
		}
		if trails[0][0] != 4 || trails[1][0] != 6 {
			t.Fatalf("M=%d: the trails' SEEDs try %d and %d first, want 4 and then 6", m, trails[0][0], trails[1][0])
		}
		counts := map[int]int{}
		for position := range m {
			if trails[0][position] == trails[1][position] {
				t.Fatalf("M=%d: position %d tries %d first in both trails (%v, %v)", m, position, trails[0][position], trails[0], trails[1])
			}
			if 0 < position {
				// the hops the stats record; the SEED is never recorded
				counts[trails[0][position]]++
				counts[trails[1][position]]++
			}
		}
		if counts[4] != m-1 || counts[6] != m-1 {
			t.Fatalf("M=%d: recorded hops tried IPv4 first %d times and IPv6 %d, want %d each", m, counts[4], counts[6], m-1)
		}
	}
}

// IPV6.md C7. Every attempt of a step, retries included, tries the step's
// family first: the hop's position decides it, not how often it was tried.
func TestTrailEngineRetriesKeepTheHopsFirstIpFamily(t *testing.T) {
	for _, family := range []int{4, 6} {
		transport := &failingTrailTransport{}
		engine := &TrailEngine{transport: transport, cfg: TrailEngineConfig{StepTimeout: time.Second, ExtendAttempts: 3}}
		if _, err := engine.postStep(context.Background(), connect.NewId(), []byte("extend"), family, nil); err == nil {
			t.Fatal("failing retries unexpectedly succeeded")
		}
		if want := []int{family, family, family}; !reflect.DeepEqual(transport.firstIpFamilies, want) {
			t.Fatalf("attempts tried %v first, want %v", transport.firstIpFamilies, want)
		}
	}
}

// IPV6.md C7. The transport hands each request's client the family the
// engine marked the request with, and no preference to a request the engine
// did not mark.
func TestTunnelTransportTriesTheRequestsFirstIpFamily(t *testing.T) {
	transport := newTestTunnelTransport(t)
	client := &tunnelTestClient{ctx: t.Context(), close: func(context.Context) error { return nil }}
	client.post = func(context.Context, connect.Id, []byte) ([]byte, error) { return []byte("ok"), nil }
	transport.newClient = func(context.Context, connect.Id) (tunnelTransportClient, error) { return client, nil }
	marked := []int{4, 6, 6, 4}
	for _, family := range marked {
		if _, err := transport.PostVerify(withHopFirstIpFamily(t.Context(), family), connect.NewId(), nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := transport.PostVerify(t.Context(), connect.NewId(), nil); err != nil {
		t.Fatal(err)
	}
	if want := append(marked, 0); !reflect.DeepEqual(client.firstIpFamilies, want) {
		t.Fatalf("first families = %v, want %v", client.firstIpFamilies, want)
	}
}

// IPV6.md C7. Through the real registered client, every hop's tunnel is the
// default tunnel with that request's first family, or none for a request
// without one.
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
	families := []int{4, 6, 0}
	for _, family := range families {
		requestCtx := ctx
		if family != 0 {
			requestCtx = withHopFirstIpFamily(ctx, family)
		}
		if _, err := transport.PostVerify(requestCtx, connect.NewId(), []byte("{}")); !errors.Is(err, tunErr) {
			t.Fatalf("hop request = %v, want the captured tunnel", err)
		}
	}
	if len(captured) != len(families) {
		t.Fatalf("tunnels = %d, want %d", len(captured), len(families))
	}
	for index, want := range families {
		expected := connect.DefaultTunSettings()
		expected.DialFirstIpFamily = want
		if !reflect.DeepEqual(captured[index], expected) {
			t.Fatalf("tunnel %d settings = %+v, want the defaults with first family %d", index, captured[index], want)
		}
	}
}
