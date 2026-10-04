// A signed original database census is one component of provider evidence.
// Its derived counters never authenticate absent client-close signatures,
// reliability trials, eligibility history, or independent finality.
package main

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/urfoundation/sn/payoutartifact"
)

// Comparable counters can be retained in the exact original archive reference.
// A zero value is omitted from legacy references, preserving their hash grammar.
type economicConservationClosedWork struct {
	Hash                      string `json:"census_hash"`
	Contracts                 uint64 `json:"contracts"`
	Providers                 uint64 `json:"providers"`
	UsageBytes                uint64 `json:"usage_bytes"`
	OrdinarySnapshots         uint64 `json:"ordinary_snapshots"`
	ExpiredSnapshots          uint64 `json:"expired_snapshots"`
	UncreditedLegacySnapshots uint64 `json:"uncredited_legacy_snapshots"`
	SignedCloseReports        uint64 `json:"signed_close_reports,omitempty"`
	RegisteredCloseReports    uint64 `json:"registered_close_reports,omitempty"`
	CloseAmountJoins          uint64 `json:"close_amount_joins,omitempty"`
}

// Missing, partial or foreign components are unknown without erasing an
// otherwise valid original entitlement. Actual contradictions remain typed.
func readEconomicClosedWork(ctx context.Context, artifact *payoutartifact.Artifact, rootSigner common.Address) (*economicConservationClosedWork, error) {
	reports, err := payoutartifact.VerifyClosedWorkReports(ctx, artifact, rootSigner)
	if errors.Is(err, payoutartifact.ErrClosedWorkUnavailable) {
		return nil, nil
	}
	if errors.Is(err, payoutartifact.ErrClosedWorkCapacity) {
		return nil, errors.Join(errMonitorEconomicCapacity, err)
	}
	if err != nil {
		return nil, err
	}
	value := reports.ClosedWork
	return &economicConservationClosedWork{Hash: value.CensusHash, Contracts: value.Contracts, Providers: value.Providers, UsageBytes: value.UsageBytes, OrdinarySnapshots: value.OrdinarySnapshots, ExpiredSnapshots: value.ExpiredSnapshots, UncreditedLegacySnapshots: value.UncreditedLegacySnapshots, SignedCloseReports: reports.SignedReports, RegisteredCloseReports: reports.RegisteredReports, CloseAmountJoins: reports.AmountJoins}, nil
}

// Only recomputed original rows can populate retained counters. A caller's
// self-sealed counters, or dropping known evidence, cannot manufacture truth.
func (self *economicConservationEntitlementCensus) validateClosedWork(ctx context.Context) error {
	known, err := readEconomicClosedWork(ctx, &self.Artifact, common.HexToAddress(self.RootSigner))
	if err != nil {
		return err
	}
	if known == nil {
		if self.ClosedWork != nil {
			return errors.New("economic original closed-work counters have no complete source")
		}
		return nil
	}
	if self.ClosedWork == nil || *known != *self.ClosedWork {
		return errors.New("economic original closed-work counters differ from retained signed rows")
	}
	return nil
}

// A cold value is used only after the enclosing exact reference is checked
// against the original snapshot held by this owner.
func (self economicConservationEntitlement) closedWork() economicConservationClosedWork {
	if self.Census != nil && self.Census.ClosedWork != nil {
		return *self.Census.ClosedWork
	}
	if self.CensusReference != nil {
		return self.CensusReference.ClosedWork
	}
	return economicConservationClosedWork{}
}

// This sum describes original database rows, not complete physical traffic.
// Big integers preserve totals across many independent archived epochs.
func (self *economicConservationEntitlementSummary) addClosedWork(value economicConservationClosedWork) {
	if value.Hash == "" {
		return
	}
	self.ClosedWorkRoots++
	self.ClosedWorkContracts += value.Contracts
	self.SignedCloseReports += value.SignedCloseReports
	self.RegisteredCloseReports += value.RegisteredCloseReports
	self.CloseAmountJoins += value.CloseAmountJoins
	amount := new(big.Int)
	if self.ClosedWorkUsageBytes != "" {
		amount.SetString(self.ClosedWorkUsageBytes, 10)
	}
	self.ClosedWorkUsageBytes = amount.Add(amount, new(big.Int).SetUint64(value.UsageBytes)).String()
}

// Both operands are derived from admitted original records, never a supplied
// summary. Full provider authentication remains a separate evidence decision.
func (self *economicConservationEntitlementSummary) mergeClosedWork(cold *economicConservationEntitlementSummary) {
	self.ClosedWorkRoots += cold.ClosedWorkRoots
	self.ClosedWorkContracts += cold.ClosedWorkContracts
	self.SignedCloseReports += cold.SignedCloseReports
	self.RegisteredCloseReports += cold.RegisteredCloseReports
	self.CloseAmountJoins += cold.CloseAmountJoins
	if cold.ClosedWorkUsageBytes == "" {
		return
	}
	amount, _ := new(big.Int).SetString(cold.ClosedWorkUsageBytes, 10)
	active := new(big.Int)
	if self.ClosedWorkUsageBytes != "" {
		active.SetString(self.ClosedWorkUsageBytes, 10)
	}
	self.ClosedWorkUsageBytes = active.Add(active, amount).String()
}
