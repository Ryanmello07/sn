// One queue writer publishes sanitized immutable snapshots only after its
// existing durable acknowledgement. HTTP readers never access writer state.
package miner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/urfoundation/sn/protocol"
)

// Concurrent-safe publication has its own lifecycle. It owns no signing key,
// filesystem descriptor, repair authority or callback into the queue writer.
type claimProgressOwner struct {
	stateLock sync.Mutex
	closed    bool
	value     protocol.ClaimProgress
}

func newClaimProgressOwner(member string, pool *protocol.ClaimProgressPool) *claimProgressOwner {
	var instance [16]byte
	if _, err := rand.Read(instance[:]); err != nil {
		return nil
	}
	var declared *protocol.ClaimProgressPool
	if pool != nil && pool.Validate() == nil {
		copy := *pool
		declared = &copy
	}
	return &claimProgressOwner{value: protocol.ClaimProgress{Schema: protocol.ClaimProgressSchema, Member: member, Status: "unknown", InstanceId: hex.EncodeToString(instance[:]), StartedAt: time.Now().UTC().Format(time.RFC3339Nano), DeclaredPool: declared}}
}

func claimProgressStatus(value string) string {
	switch value {
	case "pending", "retry", "submitting", "uncertain", "finalized", "no-claim":
		return value
	}
	return "unknown"
}

func claimProgressPending(entry *ClaimQueueEntry) bool {
	return entry.Status != "finalized" && entry.Status != "no-claim"
}

// Selection is bounded while traversing the already byte-bounded queue.
// Unresolved liabilities precede recent terminal epochs; nothing is deleted.
func claimProgressEntries(queue *ClaimQueue, pool *protocol.ClaimProgressPool) ([]protocol.ClaimProgressEntry, uint64) {
	selected := make([]*ClaimQueueEntry, 0, protocol.MaxClaimProgressEntries)
	total := uint64(0)
	for _, entry := range queue.Entries {
		if entry == nil || entry.Epoch < 0 {
			continue
		}
		total++
		position := sort.Search(len(selected), func(i int) bool {
			left, right := claimProgressPending(entry), claimProgressPending(selected[i])
			if left != right {
				return left
			}
			return entry.Epoch > selected[i].Epoch
		})
		if position >= protocol.MaxClaimProgressEntries {
			continue
		}
		selected = slices.Insert(selected, position, entry)
		if len(selected) > protocol.MaxClaimProgressEntries {
			selected = selected[:protocol.MaxClaimProgressEntries]
		}
	}
	entries := make([]protocol.ClaimProgressEntry, 0, len(selected))
	for _, entry := range selected {
		projected := protocol.ClaimProgressEntry{Epoch: entry.Epoch, QueueStatus: claimProgressStatus(entry.Status), ObservationStatus: "unknown", DomainStatus: "unknown"}
		if observation := entry.PublicObservation; observation != nil && observation.Epoch == entry.Epoch && observation.Validate() == nil {
			copy := *observation
			if observation.LeafClaimed != nil {
				value := *observation.LeafClaimed
				copy.LeafClaimed = &value
			}
			projected.Observation = &copy
			projected.ObservationStatus = "retained"
			if pool != nil && copy.EvidenceKind != "api-no-claim" {
				projected.DomainStatus = "identity"
				if *pool == copy.Pool {
					projected.DomainStatus = "match"
				}
			}
		}
		entries = append(entries, projected)
	}
	return entries, total - uint64(len(entries))
}

// A failed save never calls this method. It accepts only the exact queue
// representation just acknowledged by the original descriptor/head owner.
func (self *claimProgressOwner) acknowledge(queue *ClaimQueue, digest [32]byte, omitted uint64) {
	if self == nil {
		return
	}
	var pool *protocol.ClaimProgressPool
	func() {
		self.stateLock.Lock()
		defer self.stateLock.Unlock()
		if self.value.DeclaredPool != nil {
			copy := *self.value.DeclaredPool
			pool = &copy
		}
	}()
	entries, omittedEntries := claimProgressEntries(queue, pool)
	now := time.Now().UTC()
	self.stateLock.Lock()
	defer self.stateLock.Unlock()
	if self.closed {
		return
	}
	start, _ := time.Parse(time.RFC3339Nano, self.value.StartedAt)
	previous, _ := time.Parse(time.RFC3339Nano, self.value.PublishedAt)
	if self.value.Sequence == math.MaxUint64 || now.Before(start) || now.Before(previous) {
		self.value.Status = "unavailable"
		return
	}
	self.value.Sequence++
	self.value.Status = "active"
	self.value.PublishedAt = now.Format(time.RFC3339Nano)
	self.value.QueueSha256 = hex.EncodeToString(digest[:])
	self.value.Entries = entries
	self.value.OmittedEntries = omittedEntries
	self.value.OmittedObservations = omitted
}

func (self *claimProgressOwner) unavailable() {
	if self == nil {
		return
	}
	self.stateLock.Lock()
	defer self.stateLock.Unlock()
	if !self.closed {
		self.value.Status = "unavailable"
	}
}
func (self *claimProgressOwner) close() {
	if self == nil {
		return
	}
	self.stateLock.Lock()
	defer self.stateLock.Unlock()
	self.closed = true
	self.value.Status = "closed"
}

// The encoded bytes are owned by the caller and never share mutable entries.
func (self *claimProgressOwner) snapshot(ctx context.Context) ([]byte, bool) {
	if self == nil || ctx == nil || ctx.Err() != nil {
		return nil, false
	}
	self.stateLock.Lock()
	value := self.value
	self.stateLock.Unlock()
	// Entries and observations are immutable once published; replacement under
	// the lock never mutates their backing array or nested observation values.
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > protocol.MaxClaimProgressBytes || value.Validate() != nil || ctx.Err() != nil {
		return nil, false
	}
	return raw, value.Status == "active"
}

func (self *ClaimSwarm) serveClaimProgress(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet || request.Context().Err() != nil || len(request.URL.RawQuery) > 256 {
		http.Error(writer, "invalid claim observation request", http.StatusBadRequest)
		return
	}
	query := request.URL.Query()
	values, ok := query["id"]
	if !ok || len(query) != 1 || len(values) != 1 || len(values[0]) == 0 || len(values[0]) > 128 {
		http.Error(writer, "invalid claim member", http.StatusBadRequest)
		return
	}
	id := values[0]
	self.stateLock.Lock()
	owner, known := self.progress[id]
	self.stateLock.Unlock()
	if !known {
		http.NotFound(writer, request)
		return
	}
	raw, active := owner.snapshot(request.Context())
	if raw == nil {
		raw, _ = json.Marshal(protocol.ClaimProgress{Schema: protocol.ClaimProgressSchema, Member: id, Status: "unknown"})
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	if !active {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}
	_, _ = writer.Write(raw)
}
