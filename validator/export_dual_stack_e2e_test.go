//go:build dualstacke2e

package validator

import (
	"context"

	"github.com/urnetwork/connect"
)

// SetTunnelTransportE2EHooks replaces the two seams the dual-stack end-to-end
// test needs: each hop's tun constructor, and a callback that receives every
// registered client so the test can route it to an in-process provider
// instead of the platform. Call before the first request.
func SetTunnelTransportE2EHooks(
	transport *TunnelTransport,
	createTun func(context.Context, *connect.TunSettings) (*connect.Tun, error),
	clientCreated func(*connect.Client),
) {
	transport.createTun = createTun
	newClient := transport.newClient
	transport.newClient = func(ctx context.Context, firstHop connect.Id) (tunnelTransportClient, error) {
		owned, err := newClient(ctx, firstHop)
		if registered, ok := owned.(*registeredTunnelClient); ok && err == nil && registered.client != nil {
			clientCreated(registered.client)
		}
		return owned, err
	}
}
