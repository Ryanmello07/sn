package validator

// transport_verify_dial.go — the address family of a hop's /verify dial.
//
// The server attributes a /verify request to a provider by the request's
// exact source address (VALIDATOR.md §8.1): it must be the one live egress
// address the provider's platform transport registered (§8.2). A source that
// resolves to no provider seeds a synthetic hop 0, which the engine refuses
// as "assign rewrites confirmed hop 0", and it fails an EXTEND.
//
// The tunnel's default dial races the API host's A and AAAA answers and
// starts with whichever arrives first, so through a hop that can egress IPv6
// the request may leave from the hop's IPv6 address. A request that left that
// way resolved to no provider, while requests through the same kind of
// tunnel over IPv4 resolved. Dialing IPv4 only keeps the source on the family
// the platform assumes every provider carries: the tunnel then resolves the A
// record alone and never dials an AAAA answer.
//
// A hop whose only registered egress is an IPv6 address cannot confirm a
// request dialed this way. The direct API and platform dials, the tunnel's
// own DoH resolver and every signed policy parameter are unchanged.

import (
	"context"
	"fmt"
	"net"
	"syscall"
)

// The dial an http.Transport asks its tunnel for.
type verifyTunnelDialFunction func(ctx context.Context, network string, address string) (net.Conn, error)

// Narrows a hop's verify stream dial to IPv4 before the tunnel resolves or
// connects anything. The HTTP transport asks for "tcp". An IPv6-only network
// is refused as an unsupported family, and any other network was never a
// verify stream.
func verifyTunnelDialContext(dial verifyTunnelDialFunction) verifyTunnelDialFunction {
	return func(ctx context.Context, network string, address string) (net.Conn, error) {
		switch network {
		case "tcp", "tcp4":
			return dial(ctx, "tcp4", address)
		case "tcp6":
			return nil, &net.OpError{Op: "dial", Net: network, Err: syscall.EAFNOSUPPORT}
		default:
			return nil, fmt.Errorf("verify tunnel dial does not carry network %s", network)
		}
	}
}
