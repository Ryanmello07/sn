// transport_verify_dial_test.go pins the address family of a hop's /verify
// dial: IPv4 only, through the wrapper, the real tunnel and the production
// wiring.
package validator

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/urnetwork/connect"
)

// The HTTP transport asks for "tcp". The tunnel must see only "tcp4", with
// the address untouched, and its result must pass through unchanged.
func TestVerifyTunnelDialNarrowsStreamsToIPv4(t *testing.T) {
	sentinel := errors.New("tunnel dial result")
	for _, network := range []string{"tcp", "tcp4"} {
		var gotNetworks, gotAddresses []string
		dial := verifyTunnelDialContext(func(_ context.Context, network string, address string) (net.Conn, error) {
			gotNetworks, gotAddresses = append(gotNetworks, network), append(gotAddresses, address)
			return nil, sentinel
		})
		conn, err := dial(context.Background(), network, "api.example:443")
		if conn != nil || err != sentinel {
			t.Fatalf("%s: dial result = %v, %v; want the tunnel's own result", network, conn, err)
		}
		if len(gotNetworks) != 1 || gotNetworks[0] != "tcp4" || gotAddresses[0] != "api.example:443" {
			t.Fatalf("%s: tunnel dialed %v %v, want one tcp4 dial of api.example:443", network, gotNetworks, gotAddresses)
		}
	}
}

// An IPv6-only or non-stream network never reaches the tunnel, so it cannot
// resolve or dial an AAAA answer on the verify path.
func TestVerifyTunnelDialRefusesOtherNetworksBeforeTheTunnel(t *testing.T) {
	calls := 0
	dial := verifyTunnelDialContext(func(context.Context, string, string) (net.Conn, error) {
		calls++
		return nil, errors.New("tunnel dialed")
	})
	conn, err := dial(context.Background(), "tcp6", "api.example:443")
	if conn != nil || !errors.Is(err, syscall.EAFNOSUPPORT) {
		t.Fatalf("tcp6 dial = %v, %v; want an unsupported address family", conn, err)
	}
	for _, network := range []string{"udp", "udp4", "udp6", "unix", "ip4:tcp", ""} {
		if conn, err := dial(context.Background(), network, "api.example:443"); conn != nil || err == nil {
			t.Fatalf("%q dial = %v, %v; want a refusal", network, conn, err)
		}
	}
	if calls != 0 {
		t.Fatalf("refused networks reached the tunnel %d times", calls)
	}
}

// The first TCP segment a tunnel netstack sends: its IP version, destination
// and flags. Anything that is not TCP is skipped.
type verifyTunnelSegment struct {
	version     int
	destination netip.AddrPort
	flags       byte
}

// Starts one dial through a fresh IPv6-capable tunnel and returns the first
// TCP segment it sends, or nil when the dial ends without sending one.
func firstVerifyTunnelSegment(t *testing.T, narrowed bool, address string) (*verifyTunnelSegment, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tun, err := connect.CreateTunWithDefaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()
	if !tun.Ipv6Enabled() {
		t.Fatal("default tunnel has no IPv6; the test would not exercise the narrowing")
	}
	dial := verifyTunnelDialFunction(tun.DialContext)
	if narrowed {
		dial = verifyTunnelDialContext(tun.DialContext)
	}

	segments := make(chan *verifyTunnelSegment, 1)
	go func() {
		for {
			packet, err := tun.Read()
			if err != nil {
				return
			}
			segment := parseVerifyTunnelSegment(packet)
			connect.MessagePoolReturn(packet)
			if segment != nil {
				segments <- segment
				return
			}
		}
	}()
	dialErrors := make(chan error, 1)
	dialCtx, dialCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dialCancel()
	go func() {
		conn, err := dial(dialCtx, "tcp", address)
		if conn != nil {
			conn.Close()
		}
		dialErrors <- err
	}()
	select {
	case segment := <-segments:
		dialCancel()
		<-dialErrors
		return segment, nil
	case err := <-dialErrors:
		// A refused dial has returned; give a stray segment a moment to show.
		select {
		case segment := <-segments:
			return segment, err
		case <-time.After(200 * time.Millisecond):
			return nil, err
		}
	}
}

func parseVerifyTunnelSegment(packet []byte) *verifyTunnelSegment {
	if len(packet) == 0 {
		return nil
	}
	var headerLength int
	var destination netip.Addr
	switch version := int(packet[0] >> 4); version {
	case 4:
		headerLength = int(packet[0]&0x0f) * 4
		if len(packet) < headerLength+14 || packet[9] != 6 {
			return nil
		}
		destination, _ = netip.AddrFromSlice(packet[16:20])
	case 6:
		headerLength = 40
		if len(packet) < headerLength+14 || packet[6] != 6 {
			return nil
		}
		destination, _ = netip.AddrFromSlice(packet[24:40])
	default:
		return nil
	}
	return &verifyTunnelSegment{
		version:     int(packet[0] >> 4),
		destination: netip.AddrPortFrom(destination, binary.BigEndian.Uint16(packet[headerLength+2:headerLength+4])),
		flags:       packet[headerLength+13],
	}
}

// Through the real tunnel netstack, which has IPv6 enabled: an unnarrowed
// dial does send an IPv6 SYN, the narrowed dial refuses the same target
// without a segment, and the narrowed dial of an IPv4 target sends an IPv4
// SYN. The family therefore comes from the narrowing alone.
func TestVerifyTunnelDialKeepsIPv6OutOfTheTunnel(t *testing.T) {
	ipv6Target := netip.MustParseAddrPort("[2001:db8::1]:443")
	ipv4Target := netip.MustParseAddrPort("192.0.2.1:443")

	control, _ := firstVerifyTunnelSegment(t, false, ipv6Target.String())
	if control == nil || control.version != 6 || control.destination != ipv6Target || control.flags&0x12 != 0x02 {
		t.Fatalf("unnarrowed tunnel dial sent %+v, want an IPv6 SYN to %s", control, ipv6Target)
	}

	refused, err := firstVerifyTunnelSegment(t, true, ipv6Target.String())
	if !errors.Is(err, syscall.EAFNOSUPPORT) {
		t.Fatalf("narrowed IPv6 target = %v, want an unsupported address family", err)
	}
	if refused != nil {
		t.Fatalf("narrowed IPv6 target still sent %+v through the tunnel", refused)
	}

	narrowed, _ := firstVerifyTunnelSegment(t, true, ipv4Target.String())
	if narrowed == nil || narrowed.version != 4 || narrowed.destination != ipv4Target || narrowed.flags&0x12 != 0x02 {
		t.Fatalf("narrowed tunnel dial sent %+v, want an IPv4 SYN to %s", narrowed, ipv4Target)
	}
}

// A verify POST built like production reaches a dual-stack name over IPv4:
// net/http asks for "tcp" and the dialer underneath is told "tcp4".
func TestVerifyTunnelTransportPostsOverIPv4(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	remotes := make(chan string, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remotes <- r.RemoteAddr
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("ok"))
	})}
	go server.Serve(listener)
	defer server.Close()

	var networks []string
	var networksLock sync.Mutex
	httpTransport := &http.Transport{
		DialContext: verifyTunnelDialContext(func(ctx context.Context, network string, address string) (net.Conn, error) {
			networksLock.Lock()
			networks = append(networks, network)
			networksLock.Unlock()
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}),
		DisableKeepAlives: true,
		ForceAttemptHTTP2: false,
	}
	defer httpTransport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:"+port+"/verify", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Transport: httpTransport}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("verify post status = %d", response.StatusCode)
	}
	networksLock.Lock()
	defer networksLock.Unlock()
	if len(networks) != 1 || networks[0] != "tcp4" {
		t.Fatalf("verify post dialed %v, want one tcp4 dial", networks)
	}
	remote, err := netip.ParseAddrPort(<-remotes)
	if err != nil || !remote.Addr().Is4() {
		t.Fatalf("verify post arrived from %v (%v), want an IPv4 source", remote, err)
	}
}

// The production transport cannot drift back to the tunnel's dual-stack dial:
// postVerify's one HTTP transport dials through the IPv4 narrowing.
func TestTunnelTransportVerifyPostDialsThroughIPv4Narrowing(t *testing.T) {
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, "transport.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var postVerify *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "postVerify" {
			postVerify = function
		}
	}
	if postVerify == nil || postVerify.Body == nil {
		t.Fatal("transport.go has no postVerify")
	}
	var dials []string
	ast.Inspect(postVerify.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		selector, ok := literal.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Transport" {
			return true
		}
		if owner, ok := selector.X.(*ast.Ident); !ok || owner.Name != "http" {
			return true
		}
		dial := "<default>"
		for _, element := range literal.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := pair.Key.(*ast.Ident); ok && key.Name == "DialContext" {
				var printed bytes.Buffer
				if err := printer.Fprint(&printed, files, pair.Value); err != nil {
					t.Fatal(err)
				}
				dial = printed.String()
			}
		}
		dials = append(dials, dial)
		return true
	})
	if len(dials) != 1 || dials[0] != "verifyTunnelDialContext(tun.DialContext)" {
		t.Fatalf("postVerify HTTP transports dial through %q, want one verifyTunnelDialContext(tun.DialContext)", dials)
	}
}
