// Actual admission originals complete the earning-party join only when their
// independent source, endpoint histories, stream cohort and outcome all agree.
package payoutartifact

import (
	"context"
	"crypto/sha256"
	"errors"
	"sort"
	"time"

	"github.com/urfoundation/sn/protocol"
	coreprotocol "github.com/urnetwork/connect/protocol"
)

type wholeWorkSourceIdentity struct {
	sourceId   string
	generation string
}

type wholeWorkEndpointIdentity struct {
	source    wholeWorkSourceIdentity
	clientId  string
	networkId string
}

type wholeWorkEndpointProof struct {
	source wholeWorkSourceIdentity
	state  protocol.ProviderWorkEndpointState
}

// Receipt hashes identify originals shared by many contracts. Endpoint replay
// retains one state per original prefix, never a quadratic copy per contract.
type wholeWorkParticipantPool struct {
	authorityKVs   map[wholeWorkSourceIdentity]protocol.ProviderWorkSourceAuthority
	originalKVs    map[[32]byte]protocol.ProviderWorkReceipt
	reservationKVs map[[16]byte][32]byte
	outcomeKVs     map[[16]byte][32]byte
	streamKVs      map[[16]byte][32]byte
	endpointKVs    map[[32]byte]wholeWorkEndpointProof
}

// Only independently replayed outcomes may select a dispute amount policy.
// Complete additionally requires the entire original earning-party set.
type wholeWorkParticipantVerification struct {
	Complete bool
	Outcomes map[[16]byte]protocol.ProviderWorkOutcome
}

// Convert component failures without disguising cancellation as malformed data.
func wholeWorkParticipantError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, protocol.ErrProviderWorkCapacity):
		return errors.Join(ErrClosedWorkCapacity, err)
	case errors.Is(err, protocol.ErrProviderWorkUnavailable):
		return errors.Join(ErrClosedWorkUnavailable, err)
	default:
		return errors.Join(ErrClosedWorkIntegrity, err)
	}
}

// The roster explicitly approves one global persistent admission owner at a
// time. Overlapping independent generations need a stronger multi-owner fence.
func (self *wholeWorkParticipantPool) uniqueSourceAt(original protocol.ProviderWorkReceipt) bool {
	count := 0
	for identity, authority := range self.authorityKVs {
		if at := original.ObservedUnixMicro(); authority.FromUnixMicro <= at && at < authority.ThroughUnixMicro {
			if identity != (wholeWorkSourceIdentity{sourceId: original.SourceId, generation: original.Generation}) {
				return false
			}
			count++
		}
	}
	return count == 1
}

// Read every original before deciding which missing components are unknown.
// Neither a self-described signer nor an artifact publisher supplies authority.
func readWholeWorkParticipantPool(ctx context.Context, domainHash [32]byte, authorities []protocol.ProviderWorkSourceAuthority, originals [][]byte) (*wholeWorkParticipantPool, error) {
	pool := &wholeWorkParticipantPool{
		authorityKVs: map[wholeWorkSourceIdentity]protocol.ProviderWorkSourceAuthority{}, originalKVs: map[[32]byte]protocol.ProviderWorkReceipt{},
		reservationKVs: map[[16]byte][32]byte{}, outcomeKVs: map[[16]byte][32]byte{}, streamKVs: map[[16]byte][32]byte{}, endpointKVs: map[[32]byte]wholeWorkEndpointProof{},
	}
	for _, authority := range authorities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := authority.Validate(); err != nil || authority.DomainHash != domainHash {
			return nil, errors.Join(ErrClosedWorkIntegrity, err)
		}
		identity := wholeWorkSourceIdentity{sourceId: authority.SourceId, generation: authority.Generation}
		if _, exists := pool.authorityKVs[identity]; exists {
			return nil, ErrClosedWorkIntegrity
		}
		pool.authorityKVs[identity] = authority
	}
	endpointKVs := map[wholeWorkEndpointIdentity][]protocol.ProviderWorkReceipt{}
	for _, raw := range originals {
		original, err := protocol.DecodeProviderWorkReceipt(ctx, raw)
		if err != nil {
			return nil, wholeWorkParticipantError(err)
		}
		if original.DomainHash != domainHash {
			return nil, ErrClosedWorkIntegrity
		}
		source := wholeWorkSourceIdentity{sourceId: original.SourceId, generation: original.Generation}
		authority, exists := pool.authorityKVs[source]
		if !exists {
			return nil, ErrClosedWorkUnavailable
		}
		if err := protocol.VerifyProviderWorkReceiptAuthority(ctx, original, authority); err != nil {
			return nil, wholeWorkParticipantError(err)
		}
		hash := sha256.Sum256(raw)
		if _, exists := pool.originalKVs[hash]; exists {
			return nil, errors.Join(ErrClosedWorkIntegrity, errors.New("duplicate participant original"))
		}
		pool.originalKVs[hash] = original
		var idText string
		var target map[[16]byte][32]byte
		switch {
		case original.Session != nil:
			event := original.Session
			endpoint := wholeWorkEndpointIdentity{source: source, clientId: event.ClientId, networkId: event.NetworkId}
			endpointKVs[endpoint] = append(endpointKVs[endpoint], original)
		case original.Reservation != nil:
			idText, target = original.Reservation.ContractId, pool.reservationKVs
		case original.Outcome != nil:
			idText, target = original.Outcome.ContractId, pool.outcomeKVs
		case original.Stream != nil:
			idText, target = original.Stream.StreamId, pool.streamKVs
		}
		if target != nil {
			id, _ := protocol.ParseProviderWorkId(idText)
			if _, exists := target[id]; exists {
				return nil, errors.Join(ErrClosedWorkIntegrity, errors.New("conflicting original admission identity"))
			}
			target[id] = hash
		}
	}
	for endpoint, events := range endpointKVs {
		sort.Slice(events, func(i, j int) bool { return events[i].Session.Sequence < events[j].Session.Sequence })
		for index := 1; index < len(events); index++ {
			if events[index-1].Session.Sequence == events[index].Session.Sequence {
				return nil, ErrClosedWorkIntegrity
			}
		}
		states, err := protocol.ReplayProviderWorkEndpoint(ctx, pool.authorityKVs[endpoint.source], events)
		if err != nil && !errors.Is(err, protocol.ErrProviderWorkUnavailable) {
			return nil, wholeWorkParticipantError(err)
		}
		for _, state := range states {
			pool.endpointKVs[state.Head.HeadHash] = wholeWorkEndpointProof{source: endpoint.source, state: state}
		}
	}
	return pool, ctx.Err()
}

// A complete zero-extender state is currently supported. Directory key records
// alone do not bind provider ownership or prove activation at this reservation.
func (self *wholeWorkParticipantPool) endpointComplete(original protocol.ProviderWorkReceipt, head protocol.ProviderWorkEndpointHead) (bool, error) {
	proof, exists := self.endpointKVs[head.HeadHash]
	if !exists {
		return false, nil
	}
	if proof.source != (wholeWorkSourceIdentity{sourceId: original.SourceId, generation: original.Generation}) || proof.state.Head != head {
		return false, ErrClosedWorkIntegrity
	}
	if proof.state.ObservedAtUnixMicro > original.ObservedUnixMicro() || proof.state.ActiveExtenders != 0 {
		return false, nil
	}
	return true, nil
}

// Stream members come from the first actual creation, including when this
// contract reused that stream or the companion request reverses its endpoints.
func (self *wholeWorkParticipantPool) streamParties(ctx context.Context, facts coreprotocol.OriginalContractCreationFacts, outcome protocol.ProviderWorkOutcome, contracts map[[16]byte]*wholeWorkContract, creations map[[16]byte]coreprotocol.OriginalContractCreationFacts, parties map[[16]byte][16]byte) (bool, error) {
	if facts.StreamId == ([16]byte{}) {
		if outcome.StreamHash != ([32]byte{}) {
			return false, ErrClosedWorkIntegrity
		}
		return true, nil
	}
	hash, exists := self.streamKVs[facts.StreamId]
	if !exists || outcome.StreamHash == ([32]byte{}) {
		return false, nil
	}
	if hash != outcome.StreamHash {
		return false, ErrClosedWorkIntegrity
	}
	original := self.originalKVs[hash]
	cohort := original.Stream
	if !self.uniqueSourceAt(original) {
		return false, nil
	}
	source, _ := protocol.ParseProviderWorkId(cohort.SourceId)
	destination, _ := protocol.ParseProviderWorkId(cohort.DestinationId)
	if !((source == facts.SourceId && destination == facts.DestinationId) || (source == facts.DestinationId && destination == facts.SourceId)) {
		return false, ErrClosedWorkIntegrity
	}
	originId, _ := protocol.ParseProviderWorkId(cohort.OriginContractId)
	origin, exists := creations[originId]
	contract := contracts[originId]
	originReservationHash, originReserved := self.reservationKVs[originId]
	if !exists || contract == nil || !originReserved {
		return false, nil
	}
	originReservation := self.originalKVs[originReservationHash].Reservation
	if cohort.CreatedAtUnixMicro < originReservation.CreatedAtUnixMicro || cohort.CreatedAtUnixMicro > outcome.ClosedAtUnixMicro {
		return false, nil
	}
	if origin.StreamId != facts.StreamId || origin.SourceId != source || origin.DestinationId != destination || len(origin.IntermediaryIds) != len(cohort.Intermediaries) {
		return false, ErrClosedWorkIntegrity
	}
	admission, err := coreprotocol.DecodeOriginalContractAdmission(ctx, contract.ends[contract.source].OriginalCreation)
	if err != nil {
		return false, errors.Join(ErrClosedWorkIntegrity, err)
	}
	request, err := coreprotocol.DecodeOriginalContractRequest(ctx, admission.Request)
	if err != nil || sha256.Sum256(request.RequestFrame) != cohort.RequestFrameHash {
		return false, errors.Join(ErrClosedWorkIntegrity, err)
	}
	usageOrigin := facts.SourceId
	if !facts.UsageOriginIsSource {
		usageOrigin = facts.DestinationId
	}
	for index, member := range cohort.Intermediaries {
		client, _ := protocol.ParseProviderWorkId(member.ClientId)
		network, _ := protocol.ParseProviderWorkId(member.NetworkId)
		if client != origin.IntermediaryIds[index] {
			return false, ErrClosedWorkIntegrity
		}
		if client == usageOrigin {
			continue
		}
		if previous, exists := parties[client]; exists && previous != network {
			return false, ErrClosedWorkIntegrity
		}
		parties[client] = network
	}
	return true, nil
}

// This is the only positive earning-party path: exact original requests,
// fenced endpoint absence, original stream membership and original settlement.
func verifyWholeWorkParticipants(ctx context.Context, artifact *Artifact, authority WholeWorkAuthority, inventory *WholeWorkInventory, contracts map[[16]byte]*wholeWorkContract, creations map[[16]byte]coreprotocol.OriginalContractCreationFacts) (*wholeWorkParticipantVerification, error) {
	if ctx == nil || artifact == nil || artifact.ClosedWork == nil || inventory == nil || inventory.Clock == nil {
		return nil, ErrClosedWorkUnavailable
	}
	result := &wholeWorkParticipantVerification{Outcomes: map[[16]byte]protocol.ProviderWorkOutcome{}}
	domainHash, err := authority.Domain.Digest()
	if err != nil {
		return nil, err
	}
	pool, err := readWholeWorkParticipantPool(ctx, domainHash, authority.WorkSources, inventory.AttributionOriginals)
	if err != nil {
		if errors.Is(err, ErrClosedWorkUnavailable) {
			return result, nil
		}
		return nil, err
	}
	ownerNetworkKVs := map[[16]byte][16]byte{}
	for _, owner := range authority.Owners {
		if previous, exists := ownerNetworkKVs[owner.ClientId]; exists && previous != owner.NetworkId {
			return result, nil
		}
		ownerNetworkKVs[owner.ClientId] = owner.NetworkId
	}
	complete := true
	for _, row := range artifact.ClosedWork.Records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		facts, created := creations[row.ContractId]
		reservationHash, reserved := pool.reservationKVs[row.ContractId]
		outcomeHash, closed := pool.outcomeKVs[row.ContractId]
		contract := contracts[row.ContractId]
		if !created || !reserved || !closed || contract == nil {
			complete = false
			continue
		}
		reservationOriginal := pool.originalKVs[reservationHash]
		outcomeOriginal := pool.originalKVs[outcomeHash]
		reservation, outcome := reservationOriginal.Reservation, outcomeOriginal.Outcome
		source, _ := protocol.ParseProviderWorkId(reservation.SourceId)
		destination, _ := protocol.ParseProviderWorkId(reservation.DestinationId)
		sourceNetwork, _ := protocol.ParseProviderWorkId(reservation.SourceNetworkId)
		destinationNetwork, _ := protocol.ParseProviderWorkId(reservation.DestinationNetworkId)
		if source != facts.SourceId || destination != facts.DestinationId || reservation.Capacity != facts.ReservedBytes || outcome.Capacity != facts.ReservedBytes || outcome.ReservationHash != reservationHash || ownerNetworkKVs[source] != sourceNetwork || ownerNetworkKVs[destination] != destinationNetwork {
			return nil, errors.Join(ErrClosedWorkIntegrity, errors.New("original reservation or settlement differs from SDK admission"))
		}
		if reservation.RequestFrameHash == nil || reservation.UsageOriginIsSource == nil {
			complete = false
			continue
		}
		admission, err := coreprotocol.DecodeOriginalContractAdmission(ctx, contract.ends[source].OriginalCreation)
		if err != nil {
			return nil, errors.Join(ErrClosedWorkIntegrity, err)
		}
		request, err := coreprotocol.DecodeOriginalContractRequest(ctx, admission.Request)
		if err != nil || sha256.Sum256(request.RequestFrame) != *reservation.RequestFrameHash || facts.UsageOriginIsSource != *reservation.UsageOriginIsSource {
			return nil, errors.Join(ErrClosedWorkIntegrity, errors.New("SDK creation reinterprets its original received request or service direction"), err)
		}
		closedAt, err := time.Parse(time.RFC3339Nano, row.ClosedAt)
		originalTime := time.UnixMicro(outcome.ClosedAtUnixMicro)
		if err != nil || !closedAt.Equal(originalTime) || originalTime.Before(inventory.Clock.StartTime) || !originalTime.Before(inventory.Clock.EndTime) {
			return nil, errors.Join(ErrClosedWorkIntegrity, errors.New("payout changed the original settlement window"))
		}
		if reservation.CreatedAtUnixMicro > outcome.ClosedAtUnixMicro {
			complete = false
			continue
		}
		totalsComplete := true
		for _, party := range []struct {
			id       [16]byte
			amount   uint64
			terminal bool
		}{{id: source, amount: outcome.SourceBytes, terminal: outcome.SourceComplete}, {id: destination, amount: outcome.DestinationBytes, terminal: outcome.DestinationComplete}} {
			head, err := coreprotocol.DecodeOriginalCloseInventory(contract.ends[party.id].LatestInventory)
			if err != nil || !party.terminal || !head.Terminal {
				totalsComplete = false
				continue
			}
			if head.CumulativeAckedBytes != party.amount {
				return nil, errors.Join(ErrClosedWorkIntegrity, errors.New("original settlement differs from the terminal SDK report"))
			}
		}
		supported := totalsComplete && reservation.Complete && pool.uniqueSourceAt(reservationOriginal) && pool.uniqueSourceAt(outcomeOriginal)
		for _, head := range []protocol.ProviderWorkEndpointHead{reservation.SourceHead, reservation.DestinationHead} {
			present, err := pool.endpointComplete(reservationOriginal, head)
			if err != nil {
				return nil, err
			}
			supported = supported && present
		}
		if supported {
			result.Outcomes[row.ContractId] = *outcome
		}
		parties := map[[16]byte][16]byte{destination: destinationNetwork}
		if !facts.UsageOriginIsSource {
			parties = map[[16]byte][16]byte{source: sourceNetwork}
		}
		streamComplete, err := pool.streamParties(ctx, facts, *outcome, contracts, creations, parties)
		if err != nil {
			return nil, err
		}
		supported = supported && streamComplete
		if !supported {
			complete = false
			continue
		}
		snapshot, err := decodeClosedWorkSnapshot(row, artifact.Epoch)
		if err != nil {
			return nil, err
		}
		if snapshot.ExcludedReason != "" {
			complete = false
			continue
		}
		if len(parties) != len(*snapshot.Providers) {
			return nil, errors.Join(ErrClosedWorkIntegrity, errors.New("payout changed the original service-party set"))
		}
		for _, provider := range *snapshot.Providers {
			client, _ := closedWorkId(provider.ClientId)
			network, _ := closedWorkId(provider.NetworkId)
			if expected, exists := parties[client]; !exists || expected != network {
				return nil, errors.Join(ErrClosedWorkIntegrity, errors.New("payout changed an original service party"))
			}
		}
	}
	result.Complete = complete
	return result, ctx.Err()
}
