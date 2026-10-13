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

// IPV6.md C7. A retry after an ordinary failure tries the same family first:
// only the server's source-egress-unresolved answer moves a step to the other
// family.
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

// scriptedTrailTransport answers a step's attempts in turn from a script and
// records, per attempt, the hop, the body, the family tried first and the
// deadline.
type scriptedTrailTransport struct {
	answers   []scriptedTrailAnswer
	hops      []connect.Id
	bodies    [][]byte
	families  []int
	deadlines []time.Time
}

type scriptedTrailAnswer struct {
	body []byte
	err  error
	// block holds the attempt until its context ends, then answers
	block bool
}

func (self *scriptedTrailTransport) PostVerify(ctx context.Context, hop connect.Id, body []byte) ([]byte, error) {
	self.hops = append(self.hops, hop)
	self.bodies = append(self.bodies, body)
	self.families = append(self.families, hopFirstIpFamilyOf(ctx))
	deadline, _ := ctx.Deadline()
	self.deadlines = append(self.deadlines, deadline)
	answer := self.answers[len(self.families)-1]
	if answer.block {
		<-ctx.Done()
	}
	return answer.body, answer.err
}

// the transport's error for the server's retryable answer
func sourceEgressUnresolvedAnswer(hop connect.Id) scriptedTrailAnswer {
	return scriptedTrailAnswer{err: verifyPostError(hop, http.StatusMisdirectedRequest, []byte(verifySourceEgressUnresolvedBody))}
}

func scriptedTrailStep(t *testing.T, firstIpFamily int, answers ...scriptedTrailAnswer) (*scriptedTrailTransport, connect.Id, []byte, error) {
	t.Helper()
	transport := &scriptedTrailTransport{answers: answers}
	engine := &TrailEngine{transport: transport, cfg: TrailEngineConfig{StepTimeout: time.Second, ExtendAttempts: 3}}
	hop := connect.NewId()
	response, err := engine.postStep(context.Background(), hop, []byte("extend"), firstIpFamily, nil)
	for i := range transport.hops {
		if transport.hops[i] != hop || string(transport.bodies[i]) != "extend" {
			t.Fatalf("attempt %d went to %s with %q, want the same hop and the identical body", i, transport.hops[i], transport.bodies[i])
		}
	}
	return transport, hop, response, err
}

// VALIDATOR.md §8.2. The server answers a hop request it resolved to no
// provider with HTTP 421 and exactly "source-egress-unresolved\n"; only that
// answer is the retryable one. Any other body on a 421, and that body on any
// other status, is an ordinary failed attempt.
func TestVerifyPostErrorRecognizesOnlyTheExactRetryableAnswer(t *testing.T) {
	hop := connect.NewId()
	if err := verifyPostError(hop, http.StatusMisdirectedRequest, []byte("source-egress-unresolved\n")); !errors.Is(err, errSourceEgressUnresolved) {
		t.Fatalf("421 with the exact body = %v, want the retryable answer", err)
	}
	for _, c := range []struct {
		status int
		body   string
	}{
		{http.StatusMisdirectedRequest, "source-egress-unresolved"},
		{http.StatusMisdirectedRequest, "source-egress-unresolved\n\n"},
		{http.StatusMisdirectedRequest, " source-egress-unresolved\n"},
		{http.StatusMisdirectedRequest, "Source-Egress-Unresolved\n"},
		{http.StatusMisdirectedRequest, "source-egress-mismatch\n"},
		{http.StatusMisdirectedRequest, ""},
		{http.StatusBadRequest, "source-egress-unresolved\n"},
		{http.StatusConflict, "source-egress-unresolved\n"},
		{http.StatusTooManyRequests, "source-egress-unresolved\n"},
		{http.StatusServiceUnavailable, "source-egress-unresolved\n"},
		{http.StatusBadRequest, "400 trail failed"},
	} {
		err := verifyPostError(hop, c.status, []byte(c.body))
		if err == nil || errors.Is(err, errSourceEgressUnresolved) {
			t.Fatalf("%d %q = %v, want an ordinary failure", c.status, c.body, err)
		}
	}
}

// VALIDATOR.md §8.2. A step answered source-egress-unresolved is resent once,
// identically and through the same hop, trying the other family first, and
// the retry's success is the step's answer.
func TestTrailStepRetriesTheOtherFamilyOnceOnSourceEgressUnresolved(t *testing.T) {
	for _, family := range []int{4, 6} {
		transport, hop, response, err := scriptedTrailStep(t, family,
			sourceEgressUnresolvedAnswer(connect.Id{}),
			scriptedTrailAnswer{body: []byte("assign")},
		)
		if err != nil || string(response) != "assign" {
			t.Fatalf("step through %s = %q, %v, want the retry's answer", hop, response, err)
		}
		if want := []int{family, otherIpFamily(family)}; !reflect.DeepEqual(transport.families, want) {
			t.Fatalf("attempts tried %v first, want %v", transport.families, want)
		}
	}
}

// VALIDATOR.md §8.2. The server answers at most two of a step's attempts
// source-egress-unresolved: each moves the next attempt to the other family,
// and the third attempt takes the server's terminal answer, an EXTEND's
// failure or a SEED's poisoned ASSIGN, and sends nothing more.
func TestTrailStepTakesTheTerminalAnswerAfterTwoSourceEgressUnresolved(t *testing.T) {
	terminal := errors.New("verify post: http 400: 400 trail failed")
	transport, _, _, err := scriptedTrailStep(t, 4,
		sourceEgressUnresolvedAnswer(connect.Id{}),
		sourceEgressUnresolvedAnswer(connect.Id{}),
		scriptedTrailAnswer{err: terminal},
	)
	if !errors.Is(err, terminal) || errors.Is(err, errSourceEgressUnresolved) {
		t.Fatalf("step = %v, want the terminal answer", err)
	}
	if want := []int{4, 6, 4}; !reflect.DeepEqual(transport.families, want) {
		t.Fatalf("attempts tried %v first, want %v", transport.families, want)
	}
	transport, _, response, err := scriptedTrailStep(t, 6,
		sourceEgressUnresolvedAnswer(connect.Id{}),
		sourceEgressUnresolvedAnswer(connect.Id{}),
		scriptedTrailAnswer{body: []byte("poisoned assign")},
	)
	if err != nil || string(response) != "poisoned assign" {
		t.Fatalf("step = %q, %v, want the third attempt's answer", response, err)
	}
	if want := []int{6, 4, 6}; !reflect.DeepEqual(transport.families, want) {
		t.Fatalf("attempts tried %v first, want %v", transport.families, want)
	}
}

// VALIDATOR.md §8.2. A step whose attempts fail any other way, a 421 with
// another body among them, never moves to the other family.
func TestTrailStepNeverRetriesTheOtherFamilyOnOtherFailures(t *testing.T) {
	hop := connect.NewId()
	transport, _, _, err := scriptedTrailStep(t, 6,
		scriptedTrailAnswer{err: verifyPostError(hop, http.StatusMisdirectedRequest, []byte("source-egress-unresolved"))},
		scriptedTrailAnswer{err: verifyPostError(hop, http.StatusBadRequest, []byte("400 trail failed"))},
		scriptedTrailAnswer{err: errors.New("verify post: connection reset")},
	)
	if err == nil {
		t.Fatal("failing attempts unexpectedly succeeded")
	}
	if want := []int{6, 6, 6}; !reflect.DeepEqual(transport.families, want) {
		t.Fatalf("attempts tried %v first, want %v", transport.families, want)
	}
}

// VALIDATOR.md §8.2. The server counts a SEED's retry against its rate
// meters, so the retry waits for the shared SEED pace like every other SEED
// attempt: the pace, at three quarters of the per-vpk hard limit, holds the
// retries inside it instead of adding to it.
func TestTrailEngineSeedPaceMetersTheSourceEgressRetry(t *testing.T) {
	transport := &scriptedTrailTransport{answers: []scriptedTrailAnswer{
		sourceEgressUnresolvedAnswer(connect.Id{}),
		{body: []byte("assign")},
	}}
	engine := &TrailEngine{transport: transport, cfg: TrailEngineConfig{StepTimeout: time.Second, ExtendAttempts: 3}}
	paced := 0
	pace := func(context.Context) error {
		paced++
		return nil
	}
	if _, err := engine.postStep(context.Background(), connect.NewId(), []byte("seed"), 4, pace); err != nil {
		t.Fatal(err)
	}
	if paced != 2 || len(transport.families) != 2 {
		t.Fatalf("SEED pace/transport attempts = %d/%d, want 2/2", paced, len(transport.families))
	}
}

// VALIDATOR.md §8.2. The retry runs inside the step's deadline: it carries
// the first attempt's deadline, and an answer that arrives once the deadline
// has passed is not retried at all.
func TestTrailStepRetryStaysWithinTheStepDeadline(t *testing.T) {
	start := time.Now()
	transport, _, _, err := scriptedTrailStep(t, 4,
		sourceEgressUnresolvedAnswer(connect.Id{}),
		scriptedTrailAnswer{body: []byte("assign")},
	)
	end := time.Now()
	if err != nil {
		t.Fatal(err)
	}
	if len(transport.deadlines) != 2 || transport.deadlines[0].IsZero() || !transport.deadlines[1].Equal(transport.deadlines[0]) {
		t.Fatalf("attempt deadlines %v, want the step's one deadline on both", transport.deadlines)
	}
	// the step timeout, one second, from when the step began
	if deadline := transport.deadlines[0]; deadline.Before(start.Add(time.Second)) || end.Add(time.Second).Before(deadline) {
		t.Fatalf("step deadline %v, want one second after the step began (%v to %v)", deadline, start, end)
	}

	transport, _, _, err = scriptedTrailStep(t, 4,
		scriptedTrailAnswer{err: verifyPostError(connect.Id{}, http.StatusMisdirectedRequest, []byte(verifySourceEgressUnresolvedBody)), block: true},
		scriptedTrailAnswer{body: []byte("assign")},
	)
	if err == nil || len(transport.families) != 1 {
		t.Fatalf("an answer after the step deadline was retried: %d attempts, %v", len(transport.families), err)
	}
}

// IPV6.md C7, VALIDATOR.md §8.2. Retries never move the alternation: a hop
// whose first attempt is answered source-egress-unresolved retries on the
// other family, the next hop still takes its position's family, and the next
// trail still starts on the other family from this one.
func TestTrailHopFamiliesFollowPositionsAcrossRetries(t *testing.T) {
	server, validatorKey, clientId := newMockVerifyServer(t, 12)
	transport := &unresolvedOnceTrailTransport{server: server, positions: map[int]bool{0: true, 2: true}}
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
		TrailEngineConfig{M: 4, StepTimeout: 2 * time.Second, ExtendAttempts: 3, Pace: time.Millisecond},
	)
	if _, err := engine.RunTrail(context.Background()); err != nil {
		t.Fatal(err)
	}
	// positions 0 and 2 answered source-egress-unresolved once
	if want := []int{4, 6, 6, 4, 6, 6}; !reflect.DeepEqual(transport.families, want) {
		t.Fatalf("the first trail's attempts tried %v first, want %v", transport.families, want)
	}
	transport.families, transport.positions = nil, nil
	if _, err := engine.RunTrail(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []int{6, 4, 6, 4}; !reflect.DeepEqual(transport.families, want) {
		t.Fatalf("the second trail's attempts tried %v first, want %v", transport.families, want)
	}
}

// unresolvedOnceTrailTransport is the mock /verify server, answering the
// first attempt of the hops at the given positions source-egress-unresolved
// before the server sees it, and recording the family each attempt tries
// first.
type unresolvedOnceTrailTransport struct {
	server    *mockVerifyServer
	positions map[int]bool
	position  int
	retried   bool
	families  []int
}

func (self *unresolvedOnceTrailTransport) PostVerify(ctx context.Context, hop connect.Id, body []byte) ([]byte, error) {
	self.families = append(self.families, hopFirstIpFamilyOf(ctx))
	if self.positions[self.position] && !self.retried {
		self.retried = true
		return nil, verifyPostError(hop, http.StatusMisdirectedRequest, []byte(verifySourceEgressUnresolvedBody))
	}
	self.position++
	self.retried = false
	return self.server.PostVerify(ctx, hop, body)
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
