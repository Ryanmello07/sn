// The provide command owns its long-lived status worker and diagnostic sink.
// Optional log failures never decide authentication, custody or cancellation.
package miner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/urnetwork/connect"
)

// Configuration is copied before workers start and is immutable for one run.
type providerRunSettings struct {
	apiUrl           string
	connectUrl       string
	port             int
	proxySettings    []*connect.ProxySettings
	memoryPlan       providerMemoryPlan
	testEgressDialer *connect.DialContextSettings
}

// This owner starts exactly one Serve goroutine after listener admission. Its
// Close interrupts network I/O and joins Serve before the exporter can close.
type providerStatusServer struct {
	server *http.Server
	done   chan error
}

// A disabled status port creates no worker. A failed bind is returned before
// starting providers, preserving the existing service-level shutdown decision.
func newProviderStatusServer(port int, output *providerDiagnostics, cancel context.CancelFunc) (*providerStatusServer, error) {
	if port <= 0 {
		return nil, nil
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	return startProviderStatusServer(listener, output, cancel), nil
}

// The supplied listener transfers to this concrete HTTP owner. Tests can own
// an ephemeral loopback socket without racing a close/rebind port selection.
func startProviderStatusServer(listener net.Listener, output *providerDiagnostics, cancel context.CancelFunc) *providerStatusServer {
	self := &providerStatusServer{
		server: &http.Server{Handler: &Status{diagnostics: output}, ReadHeaderTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, ErrorLog: log.New(providerStatusLogWriter{diagnostics: output}, "", 0)},
		done:   make(chan error, 1),
	}
	go func() {
		err := self.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		if err != nil {
			output.observe(providerStatusFailed, 0, false, err, 0, nil)
		}
		cancel()
		self.done <- err
	}()
	output.observe(providerStatusStarted, 0, false, nil, 0, nil)
	return self
}

// Only the enclosing run calls this once, after its provider workers join.
func (self *providerStatusServer) close() error {
	if self == nil {
		return nil
	}
	return errors.Join(self.server.Close(), <-self.done)
}

// Standard HTTP diagnostics are acknowledged as a closed notice. Their raw
// request/error text is never retained or copied into the output queues.
type providerStatusLogWriter struct{ diagnostics *providerDiagnostics }

// The standard library owns incoming bytes; this callback borrows them only.
func (self providerStatusLogWriter) Write(raw []byte) (int, error) {
	self.diagnostics.observe(providerStatusFailed, 0, false, nil, 0, nil)
	return len(raw), nil
}
