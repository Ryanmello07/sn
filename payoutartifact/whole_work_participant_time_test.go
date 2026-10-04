// Late asynchronous cuts may contain real work beyond the window. Only the
// independent original admission/outcome clocks may resolve that ambiguity.
package payoutartifact

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/urfoundation/sn/protocol"
)

// Change the independently signed original event clocks before publishing a
// window assignment. The SDK boundary cuts and all report bytes stay exact.
func participantSetOriginalTimes(t *testing.T, fixture *wholeWorkTestFixture, id [16]byte, createdAt, closedAt time.Time) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{121}, ed25519.SeedSize))
	var reservationHash [32]byte
	for index, raw := range fixture.inventory.AttributionOriginals {
		original, err := protocol.DecodeProviderWorkReceipt(t.Context(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if original.Reservation == nil || original.Reservation.ContractId != participantTestId(id) {
			continue
		}
		original.Reservation.CreatedAtUnixMicro = createdAt.UnixMicro()
		original, err = protocol.SignProviderWorkReceipt(t.Context(), original, key)
		if err != nil {
			t.Fatal(err)
		}
		updated, err := original.Bytes(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		reservationHash = sha256.Sum256(updated)
		fixture.inventory.AttributionOriginals[index] = updated
	}
	if reservationHash == ([32]byte{}) {
		t.Fatal("original reservation is absent")
	}
	for index, raw := range fixture.inventory.AttributionOriginals {
		original, err := protocol.DecodeProviderWorkReceipt(t.Context(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if original.Outcome == nil || original.Outcome.ContractId != participantTestId(id) {
			continue
		}
		original.Outcome.ReservationHash = reservationHash
		original.Outcome.ClosedAtUnixMicro = closedAt.UnixMicro()
		original, err = protocol.SignProviderWorkReceipt(t.Context(), original, key)
		if err != nil {
			t.Fatal(err)
		}
		fixture.inventory.AttributionOriginals[index], err = original.Bytes(t.Context())
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestWholeWorkLateCutExcludesOnlyOriginalFutureReservation(t *testing.T) {
	fixture := participantEvidenceFixture(t)
	id := fixture.artifact.ClosedWork.Records[1].ContractId
	end := fixture.inventory.Clock.EndTime
	participantSetOriginalTimes(t, fixture, id, end, end.Add(time.Second))
	fixture.artifact.ClosedWork.Records = fixture.artifact.ClosedWork.Records[:1]
	fixture.artifact.ClosedWork.Count = 1
	fixture.inventory.Window.Records = fixture.inventory.Window.Records[:1]
	creationRebuildArtifact(t, fixture)
	value, err := VerifyWholeWorkInventoryWithWitness(t.Context(), fixture.artifact, fixture.inventory, fixture.expected)
	if err != nil || value == nil || !value.Complete || !value.AttributionComplete || value.Contracts != 1 || value.Credited != 1 || value.ExpectedProviders[0].UsageBytes != 0 || value.ExpectedProviders[1].UsageBytes != 100 {
		t.Fatalf("original future reservation poisoned a late cut: %+v, %v", value, err)
	}
	for index, raw := range fixture.inventory.AttributionOriginals {
		original, err := protocol.DecodeProviderWorkReceipt(t.Context(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if original.Reservation != nil && original.Reservation.ContractId == participantTestId(id) {
			fixture.inventory.AttributionOriginals = append(fixture.inventory.AttributionOriginals[:index], fixture.inventory.AttributionOriginals[index+1:]...)
			break
		}
	}
	if value, err := VerifyWholeWorkInventoryWithWitness(t.Context(), fixture.artifact, fixture.inventory, fixture.expected); value != nil || !errors.Is(err, ErrClosedWorkUnavailable) {
		t.Fatalf("unsigned future assertion excluded an original contract: %+v, %v", value, err)
	}
}

func TestWholeWorkLateCutFutureOnlyWindowRemainsKnownZero(t *testing.T) {
	fixture := participantEvidenceFixture(t)
	end := fixture.inventory.Clock.EndTime
	for _, row := range fixture.artifact.ClosedWork.Records {
		participantSetOriginalTimes(t, fixture, row.ContractId, end, end.Add(time.Second))
	}
	fixture.artifact.ClosedWork.Records = []ClosedWorkRecord{}
	fixture.artifact.ClosedWork.Count = 0
	fixture.inventory.Window.Records = []ClosedWorkWindowRecord{}
	creationRebuildArtifact(t, fixture)
	value, err := VerifyWholeWorkInventoryWithWitness(t.Context(), fixture.artifact, fixture.inventory, fixture.expected)
	if err != nil || value == nil || !value.Complete || !value.AttributionComplete || value.Contracts != 0 || len(value.ExpectedProviders) != 3 {
		t.Fatalf("zero-credit late cut discarded authenticated future timing: %+v, %v", value, err)
	}
	for _, provider := range value.ExpectedProviders {
		if provider.UsageBytes != 0 {
			t.Fatalf("future contract credited current provider: %+v", provider)
		}
	}
}

func TestWholeWorkLateTerminalCutUsesOriginalOutcomeToProveOpenAtEnd(t *testing.T) {
	fixture := participantEvidenceFixture(t)
	id := fixture.artifact.ClosedWork.Records[1].ContractId
	participantSetOriginalTimes(t, fixture, id, fixture.inventory.Clock.StartTime.Add(10*time.Second), fixture.inventory.Clock.EndTime)
	fixture.artifact.ClosedWork.Records = fixture.artifact.ClosedWork.Records[:1]
	fixture.artifact.ClosedWork.Count = 1
	row := &fixture.inventory.Window.Records[1]
	row.Disposition, row.ClosedAt, row.Original = "open", nil, nil
	creationRebuildArtifact(t, fixture)
	value, err := VerifyWholeWorkInventoryWithWitness(t.Context(), fixture.artifact, fixture.inventory, fixture.expected)
	if err != nil || value == nil || !value.Complete || !value.AttributionComplete || value.Open != 1 || value.Credited != 1 || value.Contracts != 2 {
		t.Fatalf("late original terminal could not prove earlier open state: %+v, %v", value, err)
	}
}

func TestWholeWorkPublisherCannotRelabelEarlierTerminalAsOpen(t *testing.T) {
	fixture := participantEvidenceFixture(t)
	fixture.artifact.ClosedWork.Records = fixture.artifact.ClosedWork.Records[:1]
	fixture.artifact.ClosedWork.Count = 1
	row := &fixture.inventory.Window.Records[1]
	row.Disposition, row.ClosedAt, row.Original = "open", nil, nil
	creationRebuildArtifact(t, fixture)
	if value, err := VerifyWholeWorkInventoryWithWitness(t.Context(), fixture.artifact, fixture.inventory, fixture.expected); value != nil || !errors.Is(err, ErrClosedWorkUnavailable) {
		t.Fatalf("publisher open label concealed an original earlier close: %+v, %v", value, err)
	}
}
