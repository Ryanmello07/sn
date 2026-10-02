package main

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/urfoundation/sn/protocol"
)

// Prior exact assertions survive a bounded source census omitting old entries.
// Their receipt time is historical; only current observation can refresh reads.
type monitorClaimEpochState struct {
	Epoch       int64                      `json:"epoch"`
	Observation *protocol.ClaimObservation `json:"observation,omitempty"`
	FirstSeenAt time.Time                  `json:"first_seen_at"`
	ProgressAt  time.Time                  `json:"progress_at"`
}

type monitorClaimState struct {
	SampleAt           time.Time                `json:"sample_at"`
	HighWaterAt        time.Time                `json:"high_water_at"`
	LastAcceptedAt     time.Time                `json:"last_accepted_at"`
	SemanticProgressAt time.Time                `json:"semantic_progress_at"`
	OutageSince        time.Time                `json:"outage_since"`
	LastIssueAt        time.Time                `json:"last_issue_at"`
	LastIssue          string                   `json:"last_issue"`
	Incidents          uint64                   `json:"incidents"`
	Restarts           uint64                   `json:"restarts"`
	Status             string                   `json:"status"`
	Record             *protocol.ClaimProgress  `json:"record,omitempty"`
	Epochs             []monitorClaimEpochState `json:"epochs"`
	current            bool
}

func newMonitorClaimState(policy monitorClaimPolicy) *monitorClaimState {
	value := &monitorClaimState{Status: "starting"}
	for _, expected := range policy.Epochs {
		value.Epochs = append(value.Epochs, monitorClaimEpochState{Epoch: expected.Epoch})
	}
	return value
}

// Observing a later finalized block or acknowledging unchanged queue bytes is
// not settlement progress. Compare only actual proof/outcome fields.
func monitorClaimSemantic(observation *protocol.ClaimObservation) string {
	if observation == nil {
		return ""
	}
	value := *observation
	value.ObservedAt, value.BlockHash, value.BlockNumber = "", "", 0
	raw, _ := json.Marshal(value)
	return string(raw)
}

func monitorClaimAccepted(observation *protocol.ClaimObservation) bool {
	return observation != nil && (observation.EvidenceKind == "signed-receipt" || observation.EvidenceKind == "finalized-leaf" && observation.LeafClaimed != nil && *observation.LeafClaimed)
}

func cloneMonitorClaimObservation(value *protocol.ClaimObservation) *protocol.ClaimObservation {
	if value == nil {
		return nil
	}
	copyValue := *value
	if value.LeafClaimed != nil {
		claimed := *value.LeafClaimed
		copyValue.LeafClaimed = &claimed
	}
	return &copyValue
}

func cloneMonitorClaimProgress(value *protocol.ClaimProgress) *protocol.ClaimProgress {
	copyValue := *value
	if value.DeclaredPool != nil {
		pool := *value.DeclaredPool
		copyValue.DeclaredPool = &pool
	}
	if value.OldestUnresolvedEpoch != nil {
		epoch := *value.OldestUnresolvedEpoch
		copyValue.OldestUnresolvedEpoch = &epoch
	}
	copyValue.Entries = append([]protocol.ClaimProgressEntry(nil), value.Entries...)
	for index := range copyValue.Entries {
		copyValue.Entries[index].Observation = cloneMonitorClaimObservation(value.Entries[index].Observation)
	}
	return &copyValue
}

// A contradiction to retained acceptance is visible and stops only this role.
// Missing optional detail cannot erase earlier exact asserted evidence.
func (self *monitorClaimState) observationCode(policy monitorClaimPolicy, value *protocol.ClaimProgress, now time.Time) string {
	if value == nil || value.Validate() != nil {
		return "invalid"
	}
	if code := monitorClaimIdentity(policy, value); code != "ok" {
		return code
	}
	if value.Status != "active" || value.Sequence == 0 {
		return "unavailable"
	}
	published, _ := time.Parse(time.RFC3339Nano, value.PublishedAt)
	started, _ := time.Parse(time.RFC3339Nano, value.StartedAt)
	if published.After(now) || now.Sub(published) > time.Duration(policy.FreshnessSeconds)*time.Second {
		return "stale"
	}
	if prior := self.Record; prior != nil {
		priorPublished, _ := time.Parse(time.RFC3339Nano, prior.PublishedAt)
		priorStarted, _ := time.Parse(time.RFC3339Nano, prior.StartedAt)
		if value.InstanceId == prior.InstanceId {
			if value.StartedAt != prior.StartedAt {
				return "identity"
			}
			if value.Sequence <= prior.Sequence || published.Before(priorPublished) {
				return "stale"
			}
		} else if !started.After(priorStarted) || published.Before(priorPublished) {
			return "stale"
		}
	}
	for _, prior := range self.Epochs {
		for _, entry := range value.Entries {
			if entry.Epoch != prior.Epoch || prior.Observation == nil || entry.Observation == nil {
				continue
			}
			before, after := prior.Observation, entry.Observation
			if monitorClaimAccepted(before) && !monitorClaimAccepted(after) || before.EvidenceKind == "signed-receipt" && after.EvidenceKind == "signed-receipt" && (before.TransactionHash != after.TransactionHash || before.AcceptedAmountRao != after.AcceptedAmountRao || before.BlockHash != after.BlockHash) {
				return "contradiction"
			}
		}
	}
	return "ok"
}

func (self *monitorClaimState) observe(policy monitorClaimPolicy, value *protocol.ClaimProgress, code string, now time.Time) {
	self.current = false
	if !self.HighWaterAt.IsZero() && now.Before(self.HighWaterAt) && !monitorClaimTerminal(code) {
		code = "clock"
	}
	self.SampleAt = now
	if now.After(self.HighWaterAt) {
		self.HighWaterAt = now
	}
	if code == "ok" {
		code = self.observationCode(policy, value, now)
	}
	if code == "ok" {
		if self.Record != nil && self.Record.InstanceId != value.InstanceId && self.Restarts < math.MaxUint64 {
			self.Restarts++
		}
		for index := range self.Epochs {
			retained := &self.Epochs[index]
			for _, entry := range value.Entries {
				if entry.Epoch != retained.Epoch || entry.Observation == nil {
					continue
				}
				// Do not replace a receipt with weaker leaf/API evidence.
				if retained.Observation != nil && retained.Observation.EvidenceKind == "signed-receipt" && entry.Observation.EvidenceKind != "signed-receipt" {
					continue
				}
				if retained.FirstSeenAt.IsZero() {
					retained.FirstSeenAt = now
				}
				if monitorClaimSemantic(retained.Observation) != monitorClaimSemantic(entry.Observation) {
					retained.ProgressAt, self.SemanticProgressAt = now, now
				}
				retained.Observation = cloneMonitorClaimObservation(entry.Observation)
			}
		}
		self.Record = cloneMonitorClaimProgress(value)
		self.LastAcceptedAt, self.current = now, true
		if self.summary(policy, now).Overdue > 0 {
			code = "overdue"
		}
	}
	if code == "ok" {
		self.OutageSince = time.Time{}
	} else {
		if self.OutageSince.IsZero() {
			self.OutageSince = now
		}
		if code != self.Status && self.Incidents < math.MaxUint64 {
			self.Incidents++
		}
		self.LastIssue, self.LastIssueAt = code, now
	}
	self.Status = code
}

// This fixed-size summary fits the shared diagnostic sink. Exact decimal
// amounts remain in retained typed evidence, never lossy floats or labels.
type monitorClaimSummary struct {
	Expected         int       `json:"expected"`
	Unknown          int       `json:"unknown"`
	AcceptedReceipts int       `json:"accepted_receipts"`
	ClaimedLeaves    int       `json:"claimed_leaves"`
	MerkleProofs     int       `json:"merkle_proofs"`
	ApiNoClaim       int       `json:"api_no_claim"`
	Deferred         int       `json:"deferred"`
	AggregatePaid    int       `json:"aggregate_paid"`
	InvalidPayment   int       `json:"invalid_payment"`
	Overdue          int       `json:"overdue"`
	ProgressAt       time.Time `json:"semantic_progress_at"`
}

func (self *monitorClaimState) summary(policy monitorClaimPolicy, now time.Time) monitorClaimSummary {
	result := monitorClaimSummary{Expected: len(policy.Epochs), ProgressAt: self.SemanticProgressAt}
	for index, expected := range policy.Epochs {
		observation := self.Epochs[index].Observation
		deadline, _ := time.Parse(time.RFC3339Nano, expected.AcceptBy)
		if !now.Before(deadline) && !monitorClaimAccepted(observation) {
			result.Overdue++
		}
		if observation == nil {
			result.Unknown++
			continue
		}
		switch observation.EvidenceKind {
		case "api-no-claim":
			result.ApiNoClaim++
		case "finalized-leaf":
			if observation.LeafClaimed != nil && *observation.LeafClaimed {
				result.ClaimedLeaves++
			}
			if observation.ProofStatus == "merkle-verified" {
				result.MerkleProofs++
			}
		case "signed-receipt":
			result.AcceptedReceipts++
		}
		switch observation.PaymentStatus {
		case "deferred":
			result.Deferred++
		case "aggregate-paid":
			result.AggregatePaid++
		case "invalid":
			result.InvalidPayment++
		}
	}
	return result
}

var monitorClaimCodes = map[string]int{"starting": 0, "ok": 1, "unknown": 2, "overdue": 3, "unavailable": 4, "invalid": 5, "stale": 6, "clock": 7, "identity": 8, "authentication": 9, "contradiction": 10}

func monitorClaimTerminal(code string) bool {
	return code == "identity" || code == "authentication" || code == "contradiction"
}

func validateMonitorClaimState(policy monitorClaimPolicy, state monitorClaimState) error {
	if state.SampleAt.IsZero() || state.HighWaterAt.IsZero() || state.HighWaterAt.Before(state.SampleAt) || len(state.Epochs) != len(policy.Epochs) {
		return errors.New("claim checkpoint lacks a bounded observation and exact epoch census")
	}
	if _, ok := monitorClaimCodes[state.Status]; !ok || state.Status == "starting" || (state.Record == nil) != state.LastAcceptedAt.IsZero() || state.Status != "ok" && state.OutageSince.IsZero() || state.Status == "ok" && !state.OutageSince.IsZero() {
		return errors.New("claim checkpoint status or observation provenance differs")
	}
	if state.Record != nil && (state.Record.Validate() != nil || state.Record.Status != "active" || monitorClaimIdentity(policy, state.Record) != "ok") {
		return errors.New("claim checkpoint retained a foreign or incomplete publication")
	}
	for index, epoch := range state.Epochs {
		if epoch.Epoch != policy.Epochs[index].Epoch || epoch.FirstSeenAt.After(state.HighWaterAt) || epoch.ProgressAt.After(state.HighWaterAt) {
			return errors.New("claim checkpoint expected epoch or times differ")
		}
		if epoch.Observation == nil {
			if !epoch.FirstSeenAt.IsZero() || !epoch.ProgressAt.IsZero() {
				return errors.New("claim checkpoint invented absent evidence history")
			}
			continue
		}
		observation := epoch.Observation
		if observation.Validate() != nil || observation.Epoch != epoch.Epoch || epoch.FirstSeenAt.IsZero() || epoch.ProgressAt.Before(epoch.FirstSeenAt) || state.Record == nil {
			return errors.New("claim checkpoint lost retained evidence identity")
		}
		if observation.EvidenceKind != "api-no-claim" && (observation.Pool != policy.ExpectedPool || observation.ShareBps != policy.Epochs[index].ShareBps) {
			return errors.New("claim checkpoint evidence differs from independent pool")
		}
	}
	return nil
}
