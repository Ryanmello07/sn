// Full fee proofs remain in exact archived checkpoints under held custody.
// The bounded index retains original transaction identity and knowledge; an
// unresolved refund remains unresolved after its proof bytes leave the head.
package main

import (
	"errors"
	"maps"
	"reflect"
)

func (self *economicConservationFeeSummary) validate() error {
	if self == nil || self.Census != "original-selected-signed-receipts" || self.OriginalRequests == 0 || !planSha256(self.EvidenceChain) || self.WholeProviderCensus || self.AuthenticatedFees > self.SelectedTransactions || self.MissingFees != self.SelectedTransactions-self.AuthenticatedFees || self.SelectedCensusComplete != (self.SelectedTransactions != 0 && self.MissingFees == 0) {
		return errors.New("economic archived native fee census changed original knowledge or scope")
	}
	if !self.SelectedCensusComplete {
		if self.WithdrawalRao != nil || self.RefundRao != nil || self.DebitRao != nil {
			return errors.New("economic archived unknown fee acquired an aggregate amount")
		}
		return nil
	}
	if self.WithdrawalRao == nil || self.RefundRao == nil || self.DebitRao == nil {
		return errors.New("economic archived complete fee census lost original amounts")
	}
	withdrawal, e1 := monitorEconomicInteger(*self.WithdrawalRao)
	refund, e2 := monitorEconomicInteger(*self.RefundRao)
	debit, e3 := monitorEconomicInteger(*self.DebitRao)
	if e1 != nil || e2 != nil || e3 != nil || withdrawal.Cmp(refund) < 0 || withdrawal.Sub(withdrawal, refund).Cmp(debit) != 0 {
		return errors.New("economic archived original fee debit does not conserve withdrawal and refund")
	}
	return nil
}

func (self *economicConservationArchive) retiresFees(reference monitorHistoryReference) bool {
	if self != nil {
		for _, retired := range self.FeeRetirements {
			if retired == reference {
				return true
			}
		}
	}
	return false
}

// A retirement is an explicit operation over an existing exact snapshot, not
// a free-standing sum or reference to an unrelated archive owner.
func (self *economicConservationArchive) validateFeeRetirements(segments map[string]monitorHistoryReference) error {
	if len(self.FeeRetirements) == 0 {
		if self.NativeFees != nil {
			return errors.New("economic archived fee census lost its original proof segments")
		}
		return nil
	}
	if err := self.NativeFees.validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, reference := range self.FeeRetirements {
		if seen[reference.Path] || segments[reference.Path] != reference {
			return errors.New("economic native fee retirement repeats or substitutes an original segment")
		}
		seen[reference.Path] = true
	}
	return nil
}

// Previously verified requests are reusable from either the active head or
// its authenticated archive. The worker copies these hashes before dispatch.
func (self *economicConservationState) retainedNativeFee(request string) string {
	for _, value := range self.NativeFees {
		if value.Evidence.RequestHash == request {
			return value.Evidence.ContentHash
		}
	}
	if self.archiveView != nil {
		return self.archiveView.feeRetired[request]
	}
	return ""
}

// This derives the index from original typed evidence already validated by
// compaction. It preserves the first known transaction and refuses every
// contradictory original, including an overlap from another archive request.
func (self *economicConservationArchiveView) indexRetiredNativeFees(original *economicConservationState, summary *economicConservationFeeSummary) error {
	if err := summary.validate(); err != nil {
		return err
	}
	for _, retained := range original.NativeFees {
		request, digest := retained.Evidence.RequestHash, retained.Evidence.ContentHash
		if prior := self.feeRetired[request]; prior != "" {
			if prior != digest {
				return errors.New("economic retired native fee request replaced original proof")
			}
			continue
		}
		if err := self.charge([]string{request, digest}); err != nil {
			return err
		}
		self.feeRetired[request] = digest
		for _, transaction := range retained.Evidence.Context.Transactions {
			if transaction.NativeBlock == nil {
				continue
			}
			prior, known := self.feeTransactions[transaction.TransactionHash]
			if known && !economicConservationSameFee(prior, transaction) {
				return errors.New("economic archived native fee contradicts original transaction")
			}
			if !known || !prior.FeeAuthenticated && transaction.FeeAuthenticated {
				if err := self.charge(transaction); err != nil {
					return err
				}
				self.feeTransactions[transaction.TransactionHash] = transaction
			}
		}
	}
	if summary.OriginalRequests != uint64(len(self.feeRetired)) || summary.SelectedTransactions != uint64(len(self.feeTransactions)) {
		return errors.New("economic retired fee census differs from original request and transaction index")
	}
	value := *summary
	self.feeSummary = &value
	return nil
}

// Only the proof payload leaves the active checkpoint. The exact archive is
// retained first by the public operation's publish/readback sequence. A copied
// index permits deterministic planning without mutating its admitted ancestor.
func (self *economicConservationState) retireNativeFees(policy economicConservationPolicy, original *economicConservationState, reference monitorHistoryReference) error {
	if original.Archive != nil && original.Archive.NativeFees != nil && original.archiveView == nil {
		return errors.New("economic fee retirement requires the held original archive")
	}
	summary, err := original.feeSummary(policy)
	if err != nil {
		return err
	}
	if err := summary.validate(); err != nil {
		return err
	}
	resources, err := self.resources(policy)
	if err != nil {
		return err
	}
	view := newEconomicConservationArchiveView(resources)
	if original.archiveView != nil {
		*view = *original.archiveView
		view.feeEvidence = maps.Clone(original.archiveView.feeEvidence)
		view.feeRetired = maps.Clone(original.archiveView.feeRetired)
		view.feeTransactions = maps.Clone(original.archiveView.feeTransactions)
	}
	view.resources = resources
	if err := view.retainFeeEvidence(original); err != nil {
		return err
	}
	if err := view.indexRetiredNativeFees(original, summary); err != nil {
		return err
	}
	self.NativeFees = nil
	self.Archive.FeeRetirements = append(self.Archive.FeeRetirements, reference)
	self.Archive.NativeFees = view.feeSummary
	self.archiveView = view
	return nil
}

func (self *economicConservationState) validateAdmittedFeeArchive() error {
	if self.Archive == nil || self.Archive.NativeFees == nil {
		if self.archiveView != nil && self.archiveView.feeSummary != nil {
			return errors.New("economic active head dropped its original archived fee census")
		}
		return nil
	}
	if err := self.Archive.NativeFees.validate(); err != nil {
		return err
	}
	if self.archiveView != nil && !reflect.DeepEqual(self.Archive.NativeFees, self.archiveView.feeSummary) {
		return errors.New("economic archived fee census differs from held original proofs")
	}
	return nil
}
