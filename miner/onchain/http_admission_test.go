// The CLI submit/view constructor must retain the common physical HTTP bound.
package onchain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/urfoundation/sn/evmrpc"
)

func TestEvmHttpSubmitDialRefusesOversizedIdentity(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var call struct {
			Id json.RawMessage `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Error(err)
			return
		}
		writer.Header().Set("Content-Length", fmt.Sprint(40*1024*1024))
		_ = json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": call.Id, "result": "0x3b1"})
	}))
	defer server.Close()
	client, chain, err := dialOne(t.Context(), server.URL)
	if client != nil {
		client.Close()
	}
	if client != nil || chain != nil || !errors.Is(err, evmrpc.ErrResponseLimit) || calls.Load() != 1 {
		t.Fatalf("submit dial bypassed response admission: client=%v chain=%v error=%v calls=%d", client, chain, err, calls.Load())
	}
}
