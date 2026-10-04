// Removed funding facts remain bound to the original archived checkpoint.
// The private index is rebuilt once on cold admission and never persisted as
// self-authorizing evidence. Growing history is not rescanned by each sample.
package main

import (
	"context"
	"errors"
)

// Only the private admitted owner can construct or mutate these compact facts.
type economicConservationFundingIndex struct {
	captures     map[string]economicFundingCapture
	entitlements map[string]economicFundingEntitlement
	captured     economicFundingRange
	accepted     economicFundingRange
	paid         economicFundingRange
	captureCount uint64
	claimCount   uint64
	paymentCount uint64
}

// The surrounding admission has already checked custody. All work below is
// on admitted original facts and stops at the same owner's cancellation.
func (self *economicConservationArchiveView) retainFundingComposition(ctx context.Context, original, compacted *economicConservationState) error {
	if ctx == nil || self == nil || original == nil || compacted == nil {
		return errors.New("economic funding archive lacks its original owner")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if self.funding == nil {
		self.funding = &economicConservationFundingIndex{captures: map[string]economicFundingCapture{}, entitlements: map[string]economicFundingEntitlement{}, captured: zeroEconomicFunding(), accepted: zeroEconomicFunding(), paid: zeroEconomicFunding()}
	}
	// Do not borrow a fee-retirement candidate's private clone. The preceding
	// snapshots already admitted by this same view supply cold dependencies.
	checked := *original
	checked.archiveView = self
	resolver, err := newEconomicFundingResolver(ctx, &checked)
	if err != nil {
		return err
	}
	retainedCaptures := economicConservationRetainedIds(compacted.Captures, func(value economicConservationCapture) string { return value.Id })
	for _, capture := range original.Captures {
		if err := ctx.Err(); err != nil {
			return err
		}
		if retainedCaptures[capture.Id] {
			continue
		}
		if _, exists := self.funding.captures[capture.Id]; exists {
			return errors.New("economic funding archive repeated an original capture")
		}
		value, err := economicFundingForCapture(capture)
		if err != nil {
			return err
		}
		next, err := self.funding.captured.add(value.Funding)
		if err != nil {
			return err
		}
		if err := self.charge(value); err != nil {
			return err
		}
		self.funding.captures[value.Id] = value
		self.funding.captured = next
		self.funding.captureCount++
	}
	retainedEntitlements := economicConservationRetainedIds(compacted.Entitlements, func(value economicConservationEntitlement) string { return value.Id })
	for _, record := range original.Entitlements {
		if err := ctx.Err(); err != nil {
			return err
		}
		if retainedEntitlements[record.Id] {
			continue
		}
		if _, exists := self.funding.entitlements[record.Id]; exists {
			return errors.New("economic funding archive repeated an original entitlement")
		}
		value, err := resolver.entitlement(record.Id)
		if err != nil {
			return err
		}
		if err := self.charge(value); err != nil {
			return err
		}
		self.funding.entitlements[record.Id] = value
	}
	retainedClaims := economicConservationRetainedIds(compacted.Claims, func(value economicConservationClaim) string { return value.Id })
	for _, claim := range original.Claims {
		if err := ctx.Err(); err != nil {
			return err
		}
		if retainedClaims[claim.Id] {
			continue
		}
		value, err := resolver.claim(claim)
		if err != nil {
			return err
		}
		self.funding.accepted, err = self.funding.accepted.add(value)
		if err != nil {
			return err
		}
		self.funding.claimCount++
	}
	retainedPayments := economicConservationRetainedIds(compacted.Payments, func(value economicConservationPayment) string { return value.Id })
	for _, payment := range original.Payments {
		if err := ctx.Err(); err != nil {
			return err
		}
		if retainedPayments[payment.Id] {
			continue
		}
		value, err := resolver.payment(payment)
		if err != nil {
			return err
		}
		self.funding.paid, err = self.funding.paid.add(value)
		if err != nil {
			return err
		}
		self.funding.paymentCount++
	}
	return ctx.Err()
}
