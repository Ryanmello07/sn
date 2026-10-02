package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfoundation/sn/internal/durablehead"
	"github.com/urfoundation/sn/internal/durablepath"
	"github.com/urfoundation/sn/protocol"
	"github.com/urnetwork/connect/durablevolume"
)

const monitorProviderCheckpointSchema = "urnetwork-mainnet-provider-checkpoint-v1"
const maxMonitorProviderCheckpointBytes = 64 * 1024

type monitorProviderCheckpointRecord struct {
	Schema      string               `json:"schema"`
	PolicyHash  string               `json:"policy_hash"`
	State       monitorProviderState `json:"state"`
	ContentHash string               `json:"content_hash"`
}

type monitorProviderWorker struct {
	policy     monitorProviderPolicy
	checkpoint *monitorCheckpointStore
	metrics    *monitorMetricsStore
	state      *monitorProviderState
	client     *http.Client
	storage    monitorStorageRecovery
}

func openMonitorProviderWorker(ctx context.Context, policy monitorProviderPolicy, expected identityExpectation, checkpoint, metrics string, hooks monitorServiceHooks) (*monitorProviderWorker, error) {
	if err := durablepath.Require(ctx); err != nil {
		return nil, err
	}
	if err := policy.validate(); err != nil {
		return nil, err
	}
	policy.Members = append([]monitorExpectedProviderMember(nil), policy.Members...)
	checkpointPath, metricsPath := monitorProviderPaths(checkpoint, metrics, policy.Role)
	owner, err := openMonitorCheckpoint(checkpointPath, expected, ctx)
	if err != nil {
		return nil, err
	}
	worker := &monitorProviderWorker{policy: policy, checkpoint: owner}
	metricOwner, err := openMonitorMetrics(metricsPath, ctx)
	if err != nil {
		return nil, errors.Join(err, owner.close())
	}
	worker.metrics = metricOwner
	worker.state, err = worker.load(ctx)
	if err != nil {
		return nil, errors.Join(err, closeMonitorServiceOwners(policy.Role, metricOwner, owner, hooks))
	}
	if hooks.syncDirectory != nil {
		owner.syncDirectory = func(file *os.File) error { return hooks.syncDirectory(policy.Role, "checkpoint", file) }
		metricOwner.syncDirectory = func(file *os.File) error { return hooks.syncDirectory(policy.Role, "metrics", file) }
	}
	worker.client = newMonitorProviderClient()
	return worker, nil
}

func hashMonitorProviderCheckpoint(record monitorProviderCheckpointRecord) (string, error) {
	record.ContentHash = ""
	raw, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (self *monitorProviderWorker) load(ctx context.Context) (*monitorProviderState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := self.checkpoint.requireOwner(); err != nil {
		return nil, err
	}
	raw, err := self.checkpoint.directory.read(filepath.Base(self.checkpoint.path), maxMonitorProviderCheckpointBytes, true)
	if monitorCheckpointAbsent(err) {
		return &monitorProviderState{Status: "starting"}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := protocol.ValidateUniqueJsonKeys(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var record monitorProviderCheckpointRecord
	if err := decoder.Decode(&record); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("provider checkpoint has trailing JSON")
	}
	hash, err := hashMonitorProviderCheckpoint(record)
	if err != nil || hash != record.ContentHash || record.Schema != monitorProviderCheckpointSchema || record.PolicyHash != self.policy.hash() {
		return nil, errors.New("provider checkpoint differs from retained policy or checksum")
	}
	if err := validateMonitorProviderState(self.policy, record.State); err != nil {
		return nil, err
	}
	// Only a new complete observation can make this process current again.
	record.State.current, record.State.ready = false, 0
	return &record.State, nil
}

func validateMonitorProviderState(policy monitorProviderPolicy, state monitorProviderState) error {
	if state.SampleAt.IsZero() || state.HighWaterAt.IsZero() || state.HighWaterAt.Before(state.SampleAt) {
		return errors.New("provider checkpoint lacks a completed bounded observation")
	}
	if _, ok := monitorProviderCodes[state.Status]; !ok || state.Status == "starting" {
		return errors.New("provider checkpoint status is invalid")
	}
	if (state.Record == nil) != state.LastAcceptedAt.IsZero() || state.Status != "ok" && state.OutageSince.IsZero() || state.Status == "ok" && !state.OutageSince.IsZero() {
		return errors.New("provider checkpoint lost its observation provenance")
	}
	if state.Record != nil {
		if err := state.Record.Validate(); err != nil {
			return err
		}
		if state.Record.Source != policy.ExpectedSource || len(state.Record.Members) != len(policy.Members) {
			return errors.New("provider checkpoint source or roster differs")
		}
		expected := map[string]string{}
		for _, member := range policy.Members {
			expected[member.Slot] = member.ClientId
		}
		for _, member := range state.Record.Members {
			identity, ok := expected[member.Slot]
			if !ok || member.ClientId != "" && member.ClientId != identity {
				return errors.New("provider checkpoint member identity differs")
			}
		}
	}
	return nil
}

func (self *monitorProviderWorker) save() error {
	if err := self.checkpoint.requireOwner(); err != nil {
		return err
	}
	if err := validateMonitorProviderState(self.policy, *self.state); err != nil {
		return err
	}
	record := monitorProviderCheckpointRecord{Schema: monitorProviderCheckpointSchema, PolicyHash: self.policy.hash(), State: *self.state}
	var err error
	record.ContentHash, err = hashMonitorProviderCheckpoint(record)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil || len(raw)+1 > maxMonitorProviderCheckpointBytes {
		return errors.New("provider checkpoint exceeds its byte bound")
	}
	return errors.Join(self.checkpoint.directory.publish(filepath.Base(self.checkpoint.path), append(raw, '\n'), 0600, self.checkpoint.syncDirectory), self.checkpoint.requireOwner())
}

var monitorProviderCodes = map[string]int{"starting": 0, "ok": 1, "not_ready": 2, "missing": 3, "unavailable": 4, "invalid": 5, "stale": 6, "clock": 7, "identity": 8, "authentication": 9}

func renderMonitorProviderMetrics(policy monitorProviderPolicy, state *monitorProviderState, checkpointCurrent bool) []byte {
	current := state.current && checkpointCurrent
	ready := 0
	if current {
		ready = state.ready
	}
	flag := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	stamp := func(v time.Time) int64 {
		if v.IsZero() {
			return 0
		}
		return v.Unix()
	}
	var output strings.Builder
	for _, metric := range []struct {
		name  string
		value any
	}{
		{"status", monitorProviderCodes[state.Status]}, {"sample_timestamp_seconds", stamp(state.SampleAt)}, {"read_current", flag(current)},
		{"read_last_success_timestamp_seconds", stamp(state.LastAcceptedAt)}, {"outage_started_timestamp_seconds", stamp(state.OutageSince)},
		{"expected_members", len(policy.Members)}, {"ready_members", ready}, {"all_expected_ready", flag(current && ready == len(policy.Members))},
		{"checkpoint_current", flag(checkpointCurrent)}, {"incidents_total", state.Incidents}, {"producer_restarts_total", state.Restarts},
		{"proof_progress_known", 0}, {"settlement_progress_known", 0},
	} {
		fmt.Fprintf(&output, "# TYPE sn_mainnet_provider_%s gauge\nsn_mainnet_provider_%s{role=%q} %v\n", metric.name, metric.name, policy.Role, metric.value)
	}
	return []byte(output.String())
}

func (self *monitorProviderWorker) close(hooks monitorServiceHooks) error {
	if self.client != nil {
		self.client.CloseIdleConnections()
	}
	return closeMonitorServiceOwners(self.policy.Role, self.metrics, self.checkpoint, hooks)
}

func (self *monitorProviderWorker) run(ctx context.Context, interval time.Duration, stdout, stderr io.Writer, now func() time.Time, hooks monitorServiceHooks) int {
	for ctx.Err() == nil {
		value, code := readMonitorProvider(ctx, self.client, self.policy)
		if ctx.Err() != nil {
			return 0
		}
		self.state.observe(self.policy, value, code, now().UTC())
		checkpointErr := self.save()
		raw := renderMonitorProviderMetrics(self.policy, self.state, checkpointErr == nil)
		raw = appendMonitorOutputMetrics(raw, "sn_mainnet_provider", self.policy.Role, monitorDiagnosticSnapshot(stdout, stderr))
		metricsErr := self.metrics.saveRaw(raw)
		combined := errors.Join(checkpointErr, metricsErr)
		var ownership *monitorOutputOwnershipError
		terminal := monitorProviderTerminal(self.state.Status) || errors.As(combined, &ownership) || errors.Is(combined, durablevolume.ErrIdentity)
		event := struct {
			Schema            string                `json:"schema"`
			Role              string                `json:"role"`
			Status            string                `json:"status"`
			Current           bool                  `json:"current"`
			CheckpointCurrent bool                  `json:"checkpoint_current"`
			State             *monitorProviderState `json:"state"`
		}{"urnetwork-mainnet-provider-event-v1", self.policy.Role, self.state.Status, self.state.current && checkpointErr == nil, checkpointErr == nil, self.state}
		if err := json.NewEncoder(stdout).Encode(event); err != nil {
			if ctx.Err() != nil {
				return 0
			}
			return 3
		}
		if hooks.afterEvent != nil {
			hooks.afterEvent(ctx, self.policy.Role)
		}
		if terminal {
			return 3
		}
		if errors.Is(checkpointErr, durablehead.ErrUncertain) {
			prior := self.checkpoint
			err := self.storage.resume(ctx, self.policy.Role, func() error { return prior.close() }, func() error {
				owner, err := openMonitorCheckpoint(prior.path, prior.expected, ctx)
				if err != nil {
					return err
				}
				candidate := &monitorProviderWorker{policy: self.policy, checkpoint: owner}
				state, err := candidate.load(ctx)
				if err != nil {
					return errors.Join(err, owner.close())
				}
				owner.syncDirectory = prior.syncDirectory
				self.checkpoint, self.state = owner, state
				return nil
			}, hooks)
			if err != nil {
				if ctx.Err() != nil {
					return 0
				}
				fmt.Fprintln(stderr, "provider checkpoint continuation:", err)
				return 3
			}
			continue
		}
		if !waitMonitorService(ctx, self.policy.Role, interval, hooks) {
			return 0
		}
	}
	return 0
}
