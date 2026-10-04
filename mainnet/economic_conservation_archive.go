// One combined archive retains exact original checkpoints. Only facts with
// an observed continuation move out of the active head; unmatched source lots,
// carry, unpaid credits and unresolved receipts remain hot. Admission decodes
// each segment once, builds a bounded index and retains every shared owner.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
)

type economicConservationCounts struct {
	Mappings     uint64 `json:"mappings"`
	Lots         uint64 `json:"lots"`
	Captures     uint64 `json:"captures"`
	Entitlements uint64 `json:"entitlements"`
	Claims       uint64 `json:"claims"`
	Payments     uint64 `json:"payments"`
	Receipts     uint64 `json:"receipts"`
}

type economicConservationArchivedAmounts struct {
	Direct     string `json:"direct_gross_alpha"`
	Tail       string `json:"tail_gross_alpha"`
	Unrouted   string `json:"unrouted_gross_alpha"`
	Collateral string `json:"reward_collateral_alpha"`
}

type economicConservationArchive struct {
	Segments           []monitorHistoryReference           `json:"segments"`
	Resources          economicConservationResources       `json:"resources"`
	LastRenewalHash    string                              `json:"last_renewal_hash"`
	LastRenewalOrdinal uint64                              `json:"last_renewal_ordinal"`
	Native             economicEmissionBoundary            `json:"native_cursor"`
	Vault              economicEmissionBoundary            `json:"vault_cursor"`
	PoolBoundaries     map[string]economicEmissionBoundary `json:"original_pool_source_boundaries"`
	Counts             economicConservationCounts          `json:"counts"`
	Amounts            economicConservationArchivedAmounts `json:"amounts"`
}

func (self *economicConservationArchive) validate(policy economicConservationPolicy, state *economicConservationState) error {
	if self == nil {
		return nil
	}
	resources, err := state.resources(policy)
	if err != nil {
		return err
	}
	if len(self.Segments) == 0 || uint64(len(self.Segments)) > resources.ArchiveSegments || self.Native.Number < policy.Native.Observation.From.Number || self.Native.Number > state.Native.Cursor.Number || self.Vault.Number < policy.Vault.From.Number || self.Vault.Number > state.Vault.Cursor.Number || !rootCanonicalHash(self.Native.Hash) || !rootCanonicalHash(self.Vault.Hash) {
		return errors.New("economic archive lost original progress or bounded catalog")
	}
	seen := map[string]monitorHistoryReference{}
	for _, reference := range self.Segments {
		if err := reference.validate(); err != nil {
			return err
		}
		if _, ok := seen[reference.Path]; ok {
			return errors.New("economic archive repeats an original snapshot owner")
		}
		if _, ok := seen[reference.Path+".lock"]; ok {
			return errors.New("economic archive aliases a retained snapshot owner")
		}
		seen[reference.Path], seen[reference.Path+".lock"] = reference, reference
	}
	var componentReferences []monitorHistoryReference
	if state.Native.Archive != nil {
		componentReferences = append(componentReferences, state.Native.Archive.Segments...)
	}
	if state.Vault.Archive != nil {
		componentReferences = append(componentReferences, state.Vault.Archive.Segments...)
	}
	for _, reference := range componentReferences {
		if seen[reference.Path] != reference {
			return errors.New("economic component archive borrowed another owner's history")
		}
	}
	for _, amount := range []string{self.Amounts.Direct, self.Amounts.Tail, self.Amounts.Unrouted, self.Amounts.Collateral} {
		if _, err := monitorEconomicInteger(amount); err != nil {
			return err
		}
	}
	for pool, boundary := range self.PoolBoundaries {
		if !slices.Contains(policy.Vault.PoolIds, pool) || boundary.Number < policy.Native.Observation.From.Number || boundary.Number > self.Native.Number || !rootCanonicalHash(boundary.Hash) {
			return errors.New("economic archive changed an original capture boundary")
		}
	}
	return nil
}

// The index contains facts already admitted from retained original bytes.
// It never supplies evidence to an external caller, and survives neither owner
// replacement nor restart without authenticating the complete bounded chain.
type economicConservationArchiveView struct {
	feeEvidence  map[string]string
	owners       []*monitorHistorySnapshot
	resources    economicConservationResources
	entries      uint64
	bytes        uint64
	mappings     map[string]economicConservationMapping
	lotIds       map[string]bool
	captureKeys  map[string]string
	claimKeys    map[string]string
	claims       map[string]economicConservationClaim
	entitlements map[string]economicConservationEntitlement
	receipts     map[string]economicConservationReceipt
	reviews      map[string]bool
}

func newEconomicConservationArchiveView(resources economicConservationResources) *economicConservationArchiveView {
	return &economicConservationArchiveView{feeEvidence: map[string]string{}, resources: resources, mappings: map[string]economicConservationMapping{}, lotIds: map[string]bool{}, captureKeys: map[string]string{}, claimKeys: map[string]string{}, claims: map[string]economicConservationClaim{}, entitlements: map[string]economicConservationEntitlement{}, receipts: map[string]economicConservationReceipt{}, reviews: map[string]bool{}}
}

// The encoded facts and fixed per-entry bookkeeping have separate bounds.
// The two-times margin is explicit; it is not a claim about measured host RSS.
func (self *economicConservationArchiveView) charge(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	bytes := uint64(len(raw)) + 256
	if self.entries >= self.resources.IndexEntries/2 || bytes > self.resources.IndexBytes/2 || self.bytes > self.resources.IndexBytes/2-bytes {
		return errors.New("economic archive index needs reviewed entry/byte capacity before admission")
	}
	self.entries++
	self.bytes += bytes
	return nil
}

func (self *economicConservationArchiveView) check() error {
	if self == nil {
		return nil
	}
	for _, owner := range self.owners {
		if err := owner.check(); err != nil {
			return err
		}
	}
	return nil
}

func (self *economicConservationArchiveView) close() error {
	if self == nil {
		return nil
	}
	var result error
	for _, owner := range self.owners {
		result = errors.Join(result, owner.close())
	}
	self.owners = nil
	return result
}

func economicConservationReceiptKey(value economicConservationReceipt) string {
	return value.Role + "/" + fmt.Sprint(value.Epoch)
}

// Rechecking the same original transaction may refresh observation time. It
// cannot rewrite acceptance, payer, payment or original block evidence. Keep
// the first archived bytes instead of charging another slot for bookkeeping.
func economicConservationSameReceipt(prior, current economicConservationReceipt) bool {
	prior.Observation.ObservedAt, current.Observation.ObservedAt = "", ""
	return reflect.DeepEqual(prior, current)
}

func (self *economicConservationArchiveView) retainedReceipt(receipt economicConservationReceipt) (bool, error) {
	if self == nil {
		return false, nil
	}
	prior, exists := self.receipts[economicConservationReceiptKey(receipt)]
	if exists && !economicConservationSameReceipt(prior, receipt) {
		return false, errors.New("economic archived original Claim receipt contradicts retained evidence")
	}
	return exists, nil
}

// Unknown payment details remain an active obligation. A later observation
// of the same receipt may fill them; later separate payments cannot revise a
// known receipt's deferred/paid fields.
func economicConservationReceiptArchivable(receipt economicConservationReceipt) bool {
	return receipt.ClaimId != "" && monitorClaimPaymentKnown(&receipt.Observation)
}

// Pure compaction retains all active liabilities and exact source references.
// Component summaries refer to this combined snapshot; their earlier prefix
// is retained by the outer complete chain rather than a second owner catalog.
func compactEconomicConservation(policy economicConservationPolicy, original *economicConservationState, reference monitorHistoryReference, renewal *economicConservationRenewal) (*economicConservationState, error) {
	if err := errors.Join(original.validate(policy), reference.validate()); err != nil {
		return nil, err
	}
	next, err := cloneEconomicConservation(original)
	if err != nil {
		return nil, err
	}
	resources, err := original.resources(policy)
	if err != nil {
		return nil, err
	}
	archive := &economicConservationArchive{Resources: resources, LastRenewalHash: policy.identityHash(), Native: original.Native.Cursor, Vault: original.Vault.Cursor, PoolBoundaries: map[string]economicEmissionBoundary{}, Amounts: economicConservationArchivedAmounts{Direct: "0", Tail: "0", Unrouted: "0", Collateral: "0"}}
	if next.Archive != nil {
		archive.Segments = slices.Clone(next.Archive.Segments)
		archive.Counts, archive.Amounts = next.Archive.Counts, next.Archive.Amounts
		archive.LastRenewalHash, archive.LastRenewalOrdinal = next.Archive.LastRenewalHash, next.Archive.LastRenewalOrdinal
		for pool, boundary := range next.Archive.PoolBoundaries {
			archive.PoolBoundaries[pool] = boundary
		}
	}
	if original.Renewal != nil {
		archive.LastRenewalHash, archive.LastRenewalOrdinal = rootObjectHash(original.Renewal), original.Renewal.Ordinal
	}
	for _, prior := range archive.Segments {
		if monitorHistoryPathsAlias(prior.Path, reference.Path) {
			return nil, errors.New("economic archive aliases an original retained segment")
		}
	}
	archive.Segments = append(archive.Segments, reference)
	next.Archive, next.Renewal = archive, renewal
	if len(next.Native.History) != 0 {
		checkpoint := monitorEconomicNativeCheckpoint{Schema: monitorEconomicNativeCheckpointSchema, PolicyHash: policy.Native.identityHash(), State: next.Native}
		checkpoint.ContentHash = checkpoint.hash()
		record, err := compactMonitorEconomicNative(checkpoint, reference, policy.Native)
		if err != nil {
			return nil, err
		}
		next.Native = record.State
		next.Native.Archive.Segments = []monitorHistoryReference{reference}
	}
	if len(next.Vault.History)+len(next.Vault.Fees) != 0 {
		checkpoint := monitorEconomicEvmCheckpoint{Schema: monitorEconomicEvmCheckpointSchema, PolicyHash: policy.Vault.identityHash(), State: next.Vault}
		checkpoint.ContentHash = checkpoint.hash()
		record, err := compactMonitorEconomicEvm(checkpoint, reference, policy.Vault)
		if err != nil {
			return nil, err
		}
		next.Vault = record.State
		next.Vault.Archive.Segments = []monitorHistoryReference{reference}
	}
	usedLots, blockedPools := map[string]bool{}, map[string]bool{}
	next.Captures = nil
	for _, capture := range original.Captures {
		pool := capture.Event.Values["noId"]
		// A mapped difference is still an unmatched obligation. Keep the
		// complete unresolved suffix and its lots until evidence resolves it.
		if blockedPools[pool] || capture.Native == nil || capture.AmountDifferenceAlpha == nil || *capture.AmountDifferenceAlpha != "0" {
			blockedPools[pool] = true
			next.Captures = append(next.Captures, capture)
			continue
		}
		archive.Counts.Captures++
		archive.PoolBoundaries[pool] = *capture.Native
		for _, id := range capture.Lots {
			usedLots[id] = true
		}
	}
	next.Lots = nil
	for _, lot := range original.Lots {
		if lot.Effect.Provider && (lot.Route == nil || lot.Route.Kind == "tail-pool" && !usedLots[lot.Id]) {
			next.Lots = append(next.Lots, lot)
			continue
		}
		archive.Counts.Lots++
		if !lot.Effect.Provider {
			continue
		}
		target := &archive.Amounts.Direct
		if lot.Route == nil {
			target = &archive.Amounts.Unrouted
		} else if lot.Route.Kind == "tail-pool" {
			target = &archive.Amounts.Tail
		}
		*target, err = economicConservationSum(*target, lot.Effect.Gross)
		if err != nil {
			return nil, err
		}
		archive.Amounts.Collateral, err = economicConservationSum(archive.Amounts.Collateral, lot.Effect.Collateral)
		if err != nil {
			return nil, err
		}
	}
	archive.Counts.Mappings += uint64(len(original.Mappings))
	next.Mappings = nil
	pendingClaims, paidClaims, pendingEntitlements := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, credit := range original.Credits {
		for _, id := range credit.Claims {
			pendingClaims[id] = true
		}
	}
	for _, carry := range original.Carry {
		for _, source := range carry {
			pendingEntitlements[source.Id] = true
		}
	}
	next.Payments = nil
	for _, payment := range original.Payments {
		if payment.Status != "aggregate-credit-observed" {
			next.Payments = append(next.Payments, payment)
			continue
		}
		archive.Counts.Payments++
		for _, id := range payment.Credit.Claims {
			paidClaims[id] = true
		}
	}
	next.Claims = nil
	for _, claim := range original.Claims {
		if pendingClaims[claim.Id] || !paidClaims[claim.Id] {
			next.Claims = append(next.Claims, claim)
			pendingEntitlements[claim.Entitlement] = true
			continue
		}
		archive.Counts.Claims++
	}
	next.Entitlements = nil
	for _, entitlement := range original.Entitlements {
		terminal := entitlement.Status == "root-missed" || entitlement.Status == "carried" || entitlement.Status == "finalized" && entitlement.Total != nil && entitlement.Claimed == *entitlement.Total
		if !terminal || pendingEntitlements[entitlement.Id] {
			next.Entitlements = append(next.Entitlements, entitlement)
			continue
		}
		archive.Counts.Entitlements++
	}
	next.Receipts = nil
	for _, receipt := range original.Receipts {
		if !economicConservationReceiptArchivable(receipt) {
			next.Receipts = append(next.Receipts, receipt)
			continue
		}
		known, err := original.archiveView.retainedReceipt(receipt)
		if err != nil {
			return nil, err
		}
		if !known {
			archive.Counts.Receipts++
		}
	}
	next.entitlementIds, next.claimIds, next.captureKeys, next.claimKeys = nil, nil, nil, nil
	next.ContentHash = next.hash()
	return next, next.validate(policy)
}

func economicConservationRetainedIds[T any](values []T, id func(T) string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[id(value)] = true
	}
	return result
}

// Admission indexes only the facts removed by the deterministic compaction.
// An old unresolved record appearing in multiple snapshots is not new income.
func (self *economicConservationArchiveView) admit(original, compacted *economicConservationState) error {
	if err := self.retainFeeEvidence(original); err != nil {
		return err
	}
	if original.Renewal != nil {
		if self.reviews[original.Renewal.ReviewSha256] {
			return errors.New("economic archive reused an original operational review")
		}
		if err := self.charge(original.Renewal); err != nil {
			return err
		}
		self.reviews[original.Renewal.ReviewSha256] = true
	}
	for _, mapping := range original.Mappings {
		if prior, ok := self.mappings[mapping.EvmHash]; ok {
			if prior != mapping {
				return errors.New("economic archived native/EVM identity conflicts")
			}
			continue
		}
		if err := self.charge(mapping); err != nil {
			return err
		}
		self.mappings[mapping.EvmHash] = mapping
	}
	retainedLots := economicConservationRetainedIds(compacted.Lots, func(value economicConservationLot) string { return value.Id })
	for _, lot := range original.Lots {
		if retainedLots[lot.Id] {
			continue
		}
		if self.lotIds[lot.Id] {
			return errors.New("economic archived earning occurrence is duplicated")
		}
		if err := self.charge(lot.Id); err != nil {
			return err
		}
		self.lotIds[lot.Id] = true
	}
	retainedCaptures := economicConservationRetainedIds(compacted.Captures, func(value economicConservationCapture) string { return value.Id })
	for _, capture := range original.Captures {
		if retainedCaptures[capture.Id] {
			continue
		}
		key := economicConservationEntitlementId(capture.Event.Values["epoch"], capture.Event.Values["noId"])
		if _, exists := self.captureKeys[key]; exists {
			return errors.New("economic archived capture interval is duplicated")
		}
		if err := self.charge([]string{key, capture.Id}); err != nil {
			return err
		}
		self.captureKeys[key] = capture.Id
	}
	retainedClaims := economicConservationRetainedIds(compacted.Claims, func(value economicConservationClaim) string { return value.Id })
	for _, claim := range original.Claims {
		if retainedClaims[claim.Id] {
			continue
		}
		key := claim.Entitlement + "/" + claim.Event.Values["coldkey"]
		if _, exists := self.claimKeys[key]; exists {
			return errors.New("economic archived accepted leaf is duplicated")
		}
		if err := self.charge(claim); err != nil {
			return err
		}
		self.claimKeys[key], self.claims[claim.Id] = claim.Id, claim
	}
	retainedEntitlements := economicConservationRetainedIds(compacted.Entitlements, func(value economicConservationEntitlement) string { return value.Id })
	for _, entitlement := range original.Entitlements {
		if retainedEntitlements[entitlement.Id] {
			continue
		}
		if _, exists := self.entitlements[entitlement.Id]; exists {
			return errors.New("economic archived terminal entitlement was rewritten")
		}
		if err := self.charge(entitlement); err != nil {
			return err
		}
		self.entitlements[entitlement.Id] = entitlement
	}
	for _, receipt := range original.Receipts {
		if !economicConservationReceiptArchivable(receipt) {
			continue
		}
		key := economicConservationReceiptKey(receipt)
		known, err := self.retainedReceipt(receipt)
		if err != nil {
			return err
		}
		if known {
			continue
		}
		if err := self.charge(receipt); err != nil {
			return err
		}
		self.receipts[key] = receipt
	}
	return nil
}

func openEconomicConservationArchive(ctx context.Context, policy economicConservationPolicy, state *economicConservationState, hooks monitorServiceHooks) (_ *economicConservationArchiveView, resultErr error) {
	resources, err := state.resources(policy)
	if err != nil {
		return nil, err
	}
	view := newEconomicConservationArchiveView(resources)
	if policy.Continuation != nil {
		view.reviews[policy.Continuation.ReviewSha256] = true
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, view.close())
		}
	}()
	if state.Archive == nil {
		return view, nil
	}
	if err := state.Archive.validate(policy, state); err != nil {
		return nil, err
	}
	var prior *economicConservationArchive
	for _, reference := range state.Archive.Segments {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hooks.beforeHistoryRead(economicConservationRole, "conservation-archive-admission")
		owner, raw, err := openMonitorHistoryReader(ctx, reference)
		if err != nil {
			return nil, err
		}
		view.owners = append(view.owners, owner)
		var original economicConservationState
		if err := decodeMonitorHistoryInput(raw, &original); err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(original.Archive, prior) {
			return nil, errors.New("economic archive omitted or changed its complete predecessor")
		}
		original.archiveView = view
		compacted, err := compactEconomicConservation(policy, &original, reference, nil)
		if err != nil {
			return nil, err
		}
		if err := view.admit(&original, compacted); err != nil {
			return nil, err
		}
		prior = compacted.Archive
	}
	if !reflect.DeepEqual(prior, state.Archive) {
		return nil, errors.New("economic archive summary differs from exact original checkpoints")
	}
	if state.Renewal != nil && view.reviews[state.Renewal.ReviewSha256] {
		return nil, errors.New("economic continuation reused an archived operational review")
	}
	// Check the active head against the admitted original receipt index before
	// the observer can perform a new source read or publish another snapshot.
	checked := *state
	checked.archiveView = view
	if err := checked.validate(policy); err != nil {
		return nil, err
	}
	return view, errors.Join(ctx.Err(), view.check())
}
