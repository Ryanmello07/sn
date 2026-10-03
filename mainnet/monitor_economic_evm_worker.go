// Evm economic observations use an independently provisioned checkpoint per
// role. A durable publication precedes process-local cursor acknowledgement.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfoundation/sn/internal/durablehead"
	"github.com/urfoundation/sn/internal/durablepath"
	"github.com/urfoundation/sn/protocol"
	"github.com/urnetwork/connect/durablevolume"
)

const monitorEconomicEvmCheckpointSchema = "urnetwork-mainnet-evm-economic-checkpoint-v1"

var monitorEconomicEvmStatusCodes = map[string]int{
	"starting": 0, "observed-economic-outcome-unresolved": 1, "caught-up": 2,
	"unavailable": 3, "runtime-unavailable": 4, "capacity-held": 5,
	"identity-conflict": 6, "clock-unavailable": 7,
}

type monitorEconomicEvmCheckpoint struct {
	Schema               string                  `json:"schema"`
	PolicyHash           string                  `json:"policy_hash"`
	State                monitorEconomicEvmState `json:"state"`
	ContentHash          string                  `json:"content_hash"`
	Resources            *monitorEvmResources    `json:"resources,omitempty"`
	ResourceReviewSha256 string                  `json:"resource_review_sha256,omitempty"`
}

func (self monitorEconomicEvmCheckpoint) hash() string {
	self.ContentHash = ""
	return rootObjectHash(self)
}

type monitorEconomicEvmWorker struct {
	policy       monitorEconomicEvmPolicy
	checkpoint   *monitorCheckpointStore
	metrics      *monitorMetricsStore
	state        *monitorEconomicEvmState
	client       *rpcClient
	storage      monitorStorageRecovery
	acknowledged *monitorEvmResources
}

func openMonitorEconomicEvmWorker(ctx context.Context, client *rpcClient, policy monitorEconomicEvmPolicy, expected identityExpectation, checkpoint, metrics string, hooks monitorServiceHooks) (*monitorEconomicEvmWorker, error) {
	if err := durablepath.Require(ctx); err != nil {
		return nil, err
	}
	if err := policy.validate(expected); err != nil {
		return nil, err
	}
	policy.PoolIds = append([]string(nil), policy.PoolIds...)
	policy.Coldkeys = append([]string(nil), policy.Coldkeys...)
	policy.FeePayers = append([]string(nil), policy.FeePayers...)
	if policy.ResourceRevision != nil {
		revision := *policy.ResourceRevision
		policy.ResourceRevision = &revision
	}
	checkpointPath, metricsPath := monitorEconomicEvmPaths(checkpoint, metrics, policy.Role)
	owner, err := openMonitorCheckpoint(checkpointPath, expected, ctx)
	if err != nil {
		return nil, err
	}
	worker := &monitorEconomicEvmWorker{policy: policy, checkpoint: owner}
	worker.metrics, err = openMonitorMetrics(metricsPath, ctx)
	if err != nil {
		return nil, errors.Join(err, owner.close())
	}
	worker.state, err = worker.load(ctx)
	if err != nil {
		return nil, errors.Join(err, worker.close(hooks))
	}
	if hooks.syncDirectory != nil {
		owner.syncDirectory = func(file *os.File) error { return hooks.syncDirectory(policy.Role, "checkpoint", file) }
		worker.metrics.syncDirectory = func(file *os.File) error { return hooks.syncDirectory(policy.Role, "metrics", file) }
	}
	seconds := policy.ReadBudgetSeconds
	if seconds == 0 {
		seconds = 300
	}
	worker.client, err = newRpcClient(client.url, time.Duration(seconds)*time.Second)
	if err != nil {
		return nil, errors.Join(err, worker.close(hooks))
	}
	worker.client.retryWait = client.retryWait
	if hooks.rpcWait != nil {
		worker.client.retryWait = func(ctx context.Context, wait time.Duration) error { return hooks.rpcWait(ctx, policy.Role, wait) }
	}
	return worker, nil
}

func (self *monitorEconomicEvmWorker) close(hooks monitorServiceHooks) error {
	if self == nil {
		return nil
	}
	if self.client != nil {
		self.client.httpClient.CloseIdleConnections()
	}
	return closeMonitorServiceOwners(self.policy.Role, self.metrics, self.checkpoint, hooks)
}

func (self *monitorEconomicEvmWorker) load(ctx context.Context) (*monitorEconomicEvmState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := self.checkpoint.requireOwner(); err != nil {
		return nil, err
	}
	raw, err := self.checkpoint.directory.read(filepath.Base(self.checkpoint.path), maxRpcReplyBytes, true)
	if monitorCheckpointAbsent(err) {
		return newMonitorEconomicEvmState(self.policy), nil
	}
	if err != nil {
		return nil, err
	}
	if err := protocol.ValidateUniqueJsonKeys(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var record monitorEconomicEvmCheckpoint
	if err := decoder.Decode(&record); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("EVM economic checkpoint has trailing JSON")
	}
	if record.Schema != monitorEconomicEvmCheckpointSchema || record.PolicyHash != self.policy.identityHash() || record.ContentHash != record.hash() {
		return nil, errors.New("EVM economic checkpoint differs from its retained policy or checksum")
	}
	validationPolicy := self.policy
	if record.Resources != nil {
		if record.Resources.validate() != nil || !self.policy.resources().includes(*record.Resources) || record.ResourceReviewSha256 != "" && !planSha256(record.ResourceReviewSha256) {
			return nil, errors.New("EVM economic renewal shrank acknowledged resources or changed their review")
		}
		validationPolicy.setResources(*record.Resources)
	} else if self.policy.ResourceRevision != nil {
		validationPolicy.setResources(self.policy.ResourceRevision.Original)
	}
	if err := record.State.validate(validationPolicy); err != nil {
		return nil, err
	}
	acknowledged := validationPolicy.resources()
	self.acknowledged = &acknowledged
	record.State.CapacityRemaining = self.policy.HistoryEntries - uint64(len(record.State.History)+len(record.State.Fees))
	return &record.State, nil
}

func (self *monitorEconomicEvmWorker) save(state *monitorEconomicEvmState) error {
	if err := self.checkpoint.requireOwner(); err != nil {
		return err
	}
	if err := state.validate(self.policy); err != nil {
		return err
	}
	resources := self.policy.resources()
	record := monitorEconomicEvmCheckpoint{Schema: monitorEconomicEvmCheckpointSchema, PolicyHash: self.policy.identityHash(), State: *state, Resources: &resources}
	if self.policy.ResourceRevision != nil {
		record.ResourceReviewSha256 = self.policy.ResourceRevision.ReviewSha256
	}
	record.ContentHash = record.hash()
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(raw)+1 > maxRpcReplyBytes {
		return errMonitorEconomicCapacity
	}
	return errors.Join(self.checkpoint.directory.publish(filepath.Base(self.checkpoint.path), append(raw, '\n'), 0600, self.checkpoint.syncDirectory), self.checkpoint.requireOwner())
}

func monitorEconomicEvmReadCode(err error) string {
	switch {
	case errors.Is(err, errRpcIntegrity), errors.Is(err, errRpcIdentityMismatch), errors.Is(err, durablevolume.ErrIdentity):
		return "identity-conflict"
	case errors.Is(err, errMonitorEconomicCapacity):
		return "capacity-held"
	case errors.Is(err, errRootReceiptProfileUnavailable):
		return "runtime-unavailable"
	default:
		return "unavailable"
	}
}

// Only bounded summaries are exported. Absent independent finality, native fee
// debits/refunds and provider entitlement proofs remain explicitly unknown.
type monitorEconomicEvmSummary struct {
	Cursor                           economicEmissionBoundary    `json:"cursor"`
	PendingThrough                   *economicEmissionBoundary   `json:"pending_through,omitempty"`
	Finalized                        *economicEmissionBoundary   `json:"observed_finalized,omitempty"`
	BatchCount                       uint64                      `json:"batch_count"`
	BatchChainHash                   string                      `json:"batch_chain_hash"`
	ContractState                    *monitorEconomicEvmSnapshot `json:"observed_contract_state,omitempty"`
	ObservedFeeCostWei               *string                     `json:"observed_fee_cost_wei"`
	HistoryEntries                   int                         `json:"history_entries"`
	CapacityRemaining                uint64                      `json:"capacity_remaining"`
	SampleAt                         time.Time                   `json:"sample_at"`
	LastReadAt                       time.Time                   `json:"last_read_at"`
	LastProgressAt                   time.Time                   `json:"last_progress_at"`
	UnavailableSince                 time.Time                   `json:"unavailable_since"`
	Incidents                        uint64                      `json:"incidents"`
	Authority                        string                      `json:"authority"`
	FeeAuthority                     string                      `json:"fee_authority"`
	NativeFeeDebitRao                *string                     `json:"native_fee_debit_rao"`
	NativeFeeRefundRao               *string                     `json:"native_fee_refund_rao"`
	NativeMinerAllocationAlpha       *string                     `json:"native_miner_allocation_alpha"`
	CompleteProviderEntitlementAlpha *string                     `json:"complete_provider_entitlement_alpha"`
	IndependentFinalityVerified      bool                        `json:"independent_finality_verified"`
	NativeFeeExecutionVerified       bool                        `json:"native_fee_execution_verified"`
	ConfiguredResources              monitorEvmResources         `json:"configured_resources"`
	AcknowledgedResources            *monitorEvmResources        `json:"acknowledged_resources"`
}

func (self *monitorEconomicEvmState) summary(policy monitorEconomicEvmPolicy, acknowledged *monitorEvmResources) monitorEconomicEvmSummary {
	var feeCost *string
	if len(policy.FeePayers) != 0 && self.BatchCount != 0 {
		total := new(big.Int)
		for _, fee := range self.Fees {
			amount, _ := monitorEconomicInteger(fee.FeeWei)
			total.Add(total, amount)
		}
		value := total.String()
		feeCost = &value
	}
	return monitorEconomicEvmSummary{Cursor: self.Cursor, PendingThrough: self.PendingThrough, Finalized: self.Finalized,
		BatchCount: self.BatchCount, BatchChainHash: self.BatchChainHash, ContractState: self.Snapshot, ObservedFeeCostWei: feeCost,
		HistoryEntries: len(self.History) + len(self.Fees), CapacityRemaining: self.CapacityRemaining, SampleAt: self.SampleAt,
		LastReadAt: self.LastReadAt, LastProgressAt: self.LastProgressAt, UnavailableSince: self.UnavailableSince, Incidents: self.Incidents,
		Authority: "owned-rpc-assertion", FeeAuthority: "receipt-gas-and-owned-rpc-effective-price", ConfiguredResources: policy.resources(), AcknowledgedResources: acknowledged}
}

func renderMonitorEconomicEvmMetrics(policy monitorEconomicEvmPolicy, state *monitorEconomicEvmState, code string, current, checkpointCurrent bool, now time.Time) []byte {
	flag := func(value bool) int {
		if value {
			return 1
		}
		return 0
	}
	stamp := func(value time.Time) int64 {
		if value.IsZero() {
			return 0
		}
		return value.Unix()
	}
	var output strings.Builder
	for _, metric := range []struct {
		name  string
		value any
	}{
		{name: "status", value: monitorEconomicEvmStatusCodes[code]}, {name: "read_current", value: flag(current)},
		{name: "checkpoint_current", value: flag(checkpointCurrent)}, {name: "cursor_block", value: state.Cursor.Number},
		{name: "batches", value: state.BatchCount}, {name: "retained_events", value: len(state.History)},
		{name: "retained_transaction_fees", value: len(state.Fees)}, {name: "sample_timestamp_seconds", value: stamp(state.SampleAt)},
		{name: "last_read_timestamp_seconds", value: stamp(state.LastReadAt)}, {name: "last_progress_timestamp_seconds", value: stamp(state.LastProgressAt)},
		{name: "progress_stalled", value: flag(!state.LastProgressAt.IsZero() && now.Sub(state.LastProgressAt) >= time.Duration(policy.StallSeconds)*time.Second)},
		{name: "incidents", value: state.Incidents}, {name: "outage_started_timestamp_seconds", value: stamp(state.UnavailableSince)},
		{name: "history_capacity", value: policy.HistoryEntries}, {name: "history_remaining", value: state.CapacityRemaining},
		{name: "capacity_warning", value: flag(state.CapacityRemaining <= policy.HistoryEntries/4)},
		{name: "fee_observation_known", value: flag(len(policy.FeePayers) != 0 && state.BatchCount != 0)},
		{name: "independent_finality_verified", value: 0}, {name: "native_fee_execution_verified", value: 0},
		{name: "complete_provider_entitlement_known", value: 0},
	} {
		fmt.Fprintf(&output, "sn_mainnet_evm_economic_%s{role=%q} %v\n", metric.name, policy.Role, metric.value)
	}
	return []byte(output.String())
}

func (self *monitorEconomicEvmWorker) resume(ctx context.Context, hooks monitorServiceHooks) error {
	prior := self.checkpoint
	return self.storage.resume(ctx, self.policy.Role, func() error {
		file := prior.lock
		err := prior.close()
		if hooks.afterClose != nil {
			err = errors.Join(err, hooks.afterClose(self.policy.Role, "checkpoint", file))
		}
		return err
	}, func() error {
		owner, err := openMonitorCheckpoint(prior.path, prior.expected, ctx)
		if err != nil {
			return err
		}
		candidate := &monitorEconomicEvmWorker{policy: self.policy, checkpoint: owner}
		state, err := candidate.load(ctx)
		if err != nil {
			return errors.Join(err, owner.close())
		}
		owner.syncDirectory = prior.syncDirectory
		self.checkpoint, self.state = owner, state
		self.acknowledged = candidate.acknowledged
		return nil
	}, hooks)
}

// Every RPC in a batch shares the role's one finite deadline. Exhaustion records
// a missing sample and keeps the same cursor for the next owned iteration.
func (self *monitorEconomicEvmWorker) run(ctx context.Context, interval time.Duration, stdout, stderr io.Writer, now func() time.Time, hooks monitorServiceHooks) int {
	for ctx.Err() == nil {
		var observation *monitorEvmObservation
		readErr := self.checkpoint.requireOwner()
		if readErr == nil {
			observation, readErr = observeMonitorEconomicEvm(ctx, self.client, self.policy, self.state)
		}
		if ctx.Err() != nil {
			return 0
		}
		observedAt := now().UTC()
		candidate := *self.state
		code := "caught-up"
		if readErr == nil && observation != nil {
			var next *monitorEconomicEvmState
			next, readErr = self.state.append(self.policy, observation, observedAt)
			if readErr == nil {
				candidate, code = *next, next.Status
			}
		}
		if readErr != nil {
			code = monitorEconomicEvmReadCode(readErr)
		}
		if observedAt.IsZero() || observedAt.Before(self.state.SampleAt) {
			// A regressed wall clock cannot publish a future-dated successful sample.
			readErr = errors.Join(readErr, errors.New("EVM economic observation clock regressed"))
			if code != "identity-conflict" {
				code = "clock-unavailable"
			}
			candidate = *self.state
			observedAt = self.state.SampleAt
		}
		candidate.SampleAt, candidate.Status = observedAt, code
		if readErr == nil {
			candidate.LastReadAt, candidate.UnavailableSince = observedAt, time.Time{}
		} else if candidate.UnavailableSince.IsZero() {
			candidate.UnavailableSince = observedAt
			if candidate.Incidents < math.MaxUint64 {
				candidate.Incidents++
			}
		}
		checkpointErr := self.save(&candidate)
		if checkpointErr == nil {
			self.state = &candidate
			acknowledged := self.policy.resources()
			self.acknowledged = &acknowledged
		}
		var checkpointOwnership *monitorOutputOwnershipError
		if errors.Is(checkpointErr, durablevolume.ErrIdentity) || errors.As(checkpointErr, &checkpointOwnership) {
			code = "identity-conflict"
		}
		current := readErr == nil && checkpointErr == nil
		raw := renderMonitorEconomicEvmMetrics(self.policy, self.state, code, current, checkpointErr == nil, observedAt)
		raw = appendMonitorOutputMetrics(raw, "sn_mainnet_evm_economic", self.policy.Role, monitorDiagnosticSnapshot(stdout, stderr))
		metricsErr := self.metrics.saveRaw(raw)
		combined := errors.Join(readErr, checkpointErr, metricsErr)
		var ownership *monitorOutputOwnershipError
		terminal := code == "identity-conflict" || errors.As(combined, &ownership) || errors.Is(combined, durablevolume.ErrIdentity)
		issue := ""
		if combined != nil {
			issue = combined.Error()
			if len(issue) > 1024 {
				issue = issue[:1024]
			}
		}
		event := struct {
			Schema            string                    `json:"schema"`
			Role              string                    `json:"role"`
			Status            string                    `json:"status"`
			Current           bool                      `json:"current"`
			CheckpointCurrent bool                      `json:"checkpoint_current"`
			MetricsCurrent    bool                      `json:"metrics_current"`
			State             monitorEconomicEvmSummary `json:"state"`
			Issue             string                    `json:"issue,omitempty"`
		}{Schema: "urnetwork-mainnet-evm-economic-event-v1", Role: self.policy.Role, Status: code,
			Current: current, CheckpointCurrent: checkpointErr == nil, MetricsCurrent: metricsErr == nil, State: self.state.summary(self.policy, self.acknowledged), Issue: issue}
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
			if err := self.resume(ctx, hooks); err != nil {
				if ctx.Err() != nil {
					return 0
				}
				fmt.Fprintln(stderr, "EVM economic checkpoint continuation:", err)
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
