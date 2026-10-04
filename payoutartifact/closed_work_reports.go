// Client originals are checked independently of the artifact publisher. They
// establish signed increments, not complete physical work or earning eligibility.
package payoutartifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/urfoundation/sn/protocol"
	coreprotocol "github.com/urnetwork/connect/protocol"
)

const ClosedWorkReportsSchema = "urnetwork-original-close-report-census-v1"
const ClosedWorkInventoryReportsSchema = "urnetwork-original-close-report-census-v2"
const MaxClosedWorkReportsPerContract = 1024

// The exact SQL envelope contains every stable report retained for this row.
// The count cannot prove that legacy increments or entire contracts were absent.
type ClosedWorkReports struct {
	Window  *ClosedWorkWindow  `json:"window,omitempty"`
	Schema  string             `json:"schema"`
	Count   uint64             `json:"count"`
	Reports []ClosedWorkReport `json:"reports"`
}

// Outer fields describe the immutable accounted tuple; signed bytes and original
// registration are separate, optional facts. Null remains different from zero.
type ClosedWorkReport struct {
	ClientId        string  `json:"client_id"`
	ReportId        string  `json:"report_id"`
	Party           string  `json:"party"`
	AckedBytes      *uint64 `json:"acked_bytes"`
	UnackedBytes    *uint64 `json:"unacked_bytes"`
	Checkpoint      *bool   `json:"checkpoint"`
	Original        []byte  `json:"original"`
	KeyRegistration []byte  `json:"key_registration"`
	KeyIssue        string  `json:"key_issue,omitempty"`
	Inventory       []byte  `json:"inventory,omitempty"`
}

// These counters intentionally omit any whole-window or provider-authenticated
// boolean. The original root signer must come from independently admitted state.
type VerifiedClosedWorkReports struct {
	ClosedWork                VerifiedClosedWork
	Contracts                 uint64
	SignedReports             uint64
	RegisteredReports         uint64
	AmountJoins               uint64
	CompleteReportInventories uint64
	InventoryReports          uint64
	Window                    *ClosedWorkWindow
	VerifiedWindow            *VerifiedClosedWorkWindow
}

// Derive the identical versioned client-key namespace from the containing
// payout's explicit fields; no transport hostname or mutable current policy joins.
func ClosedWorkReportDomain(artifact *Artifact) (protocol.ClientKeyHistoryDomain, error) {
	var domain protocol.ClientKeyHistoryDomain
	if artifact == nil {
		return domain, ErrClosedWorkUnavailable
	}
	genesis, e1 := hex.DecodeString(strings.TrimPrefix(artifact.GenesisHash, "0x"))
	policy, e2 := hex.DecodeString(strings.TrimPrefix(artifact.PolicyHash, "0x"))
	if e1 != nil || e2 != nil || len(genesis) != 32 || len(policy) != 32 {
		return domain, ErrClosedWorkIntegrity
	}
	domain = protocol.ClientKeyHistoryDomain{ChainID: artifact.ChainID, GenesisHash: [32]byte(genesis), Netuid: artifact.Netuid, Coordinator: artifact.Coordinator, SettlementVault: artifact.SettlementVault, DeploymentIDHash: sha256.Sum256([]byte(artifact.DeploymentID)), PolicyHash: [32]byte(policy), NoID: artifact.NoID}
	return domain, domain.Validate()
}

// The ordinary artifact and original database math are admitted first. A zero
// expected root signer checks client signatures but grants no registered-key credit.
func VerifyClosedWorkReports(ctx context.Context, artifact *Artifact, rootSigner common.Address) (*VerifiedClosedWorkReports, error) {
	closedWork, err := VerifyClosedWork(ctx, artifact)
	if err != nil {
		return nil, err
	}
	domain, err := ClosedWorkReportDomain(artifact)
	if err != nil {
		return nil, err
	}
	domainHash, _ := domain.Digest()
	result := &VerifiedClosedWorkReports{ClosedWork: *closedWork}
	reportIds := map[[32]byte]bool{}
	providerNetworks := make(map[[16]byte][16]byte, len(artifact.Providers))
	for _, provider := range artifact.Providers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		providerNetworks[provider.ClientID] = provider.NetworkID
	}
	for _, row := range artifact.ClosedWork.Records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(row.OriginalReports) == 0 {
			continue
		}
		if len(row.OriginalReports) > MaxClosedWorkRecordBytes {
			return nil, ErrClosedWorkCapacity
		}
		if err := protocol.ValidateUniqueJsonKeys(row.OriginalReports); err != nil {
			return nil, errors.Join(ErrClosedWorkIntegrity, err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var header struct {
			Schema string `json:"schema"`
		}
		if err := json.Unmarshal(row.OriginalReports, &header); err != nil {
			return nil, errors.Join(ErrClosedWorkIntegrity, err)
		}
		if header.Schema != ClosedWorkReportsSchema && header.Schema != ClosedWorkInventoryReportsSchema {
			continue // Future optional evidence cannot change this component's authority.
		}
		var census ClosedWorkReports
		decoder := json.NewDecoder(bytes.NewReader(row.OriginalReports))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&census); err != nil {
			return nil, errors.Join(ErrClosedWorkIntegrity, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, ErrClosedWorkIntegrity
		}
		if census.Reports == nil || census.Count != uint64(len(census.Reports)) {
			continue // A different component or incomplete original is unknown here.
		}
		if len(census.Reports) > MaxClosedWorkReportsPerContract {
			return nil, ErrClosedWorkCapacity
		}
		if census.Window != nil {
			if result.Window != nil {
				return nil, ErrClosedWorkIntegrity
			}
			window, err := VerifyClosedWorkWindow(ctx, artifact, census.Window, nil)
			if err != nil && !errors.Is(err, ErrClosedWorkUnavailable) {
				return nil, err
			}
			if window != nil {
				result.Window, result.VerifiedWindow = census.Window, window
			}
		}
		completeInventory, inventoryCount, err := verifyClosedReportInventory(ctx, census)
		if err != nil {
			return nil, err
		}
		result.InventoryReports += inventoryCount
		var totals [2]uint64
		var terminals [2]uint64
		var clients [2][16]byte
		known := len(census.Reports) > 0
		previous := ""
		for _, report := range census.Reports {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			clientId, e1 := closedWorkId(report.ClientId)
			reportId, e2 := closedWorkId(report.ReportId)
			key := report.ClientId + "/" + report.ReportId
			if e1 != nil || e2 != nil || key <= previous || report.AckedBytes == nil || report.UnackedBytes == nil || report.Checkpoint == nil || *report.AckedBytes > math.MaxInt64 || report.Party != "source" && report.Party != "destination" {
				return nil, ErrClosedWorkIntegrity
			}
			if len(report.KeyIssue) > 64 || report.KeyIssue != "" && (len(report.Original) == 0 || len(report.KeyRegistration) != 0) {
				return nil, ErrClosedWorkIntegrity
			}
			// Diagnostic text is publisher-supplied and can never supply authority.
			previous = key
			var reportKey [32]byte
			copy(reportKey[:16], clientId[:])
			copy(reportKey[16:], reportId[:])
			if reportIds[reportKey] {
				return nil, errors.Join(ErrClosedWorkIntegrity, errors.New("original close report identity is reused across contracts"))
			}
			reportIds[reportKey] = true
			party := 0
			if report.Party == "destination" {
				party = 1
			}
			if clients[party] != ([16]byte{}) && clients[party] != clientId || *report.AckedBytes > math.MaxInt64-totals[party] {
				return nil, ErrClosedWorkIntegrity
			}
			clients[party] = clientId
			totals[party] += *report.AckedBytes
			if !*report.Checkpoint {
				terminals[party]++
			}
			if len(report.Original) == 0 {
				if len(report.KeyRegistration) != 0 {
					return nil, ErrClosedWorkIntegrity
				}
				known = false
				continue
			}
			original, err := coreprotocol.DecodeOriginalCloseReport(report.Original)
			outer := &coreprotocol.CloseContract{ContractId: row.ContractId[:], ReportId: reportId[:], AckedByteCount: *report.AckedBytes, UnackedByteCount: *report.UnackedBytes, Checkpoint: *report.Checkpoint}
			if err != nil || !original.Matches(clientId, outer) {
				return nil, errors.Join(ErrClosedWorkIntegrity, err)
			}
			if original.DomainHash != domainHash {
				known = false
				continue // An old policy signature never migrates to the new domain.
			}
			result.SignedReports++
			if len(report.KeyRegistration) == 0 || rootSigner == (common.Address{}) {
				known = false
				continue
			}
			registration, err := protocol.DecodeClientKeyRegistration(report.KeyRegistration)
			if err != nil {
				return nil, errors.Join(ErrClosedWorkIntegrity, err)
			}
			if registration.Domain != domain || registration.Signer != rootSigner || registration.EffectiveBoundary.Block > artifact.End.Number || registration.EffectiveBoundary.Epoch > artifact.Epoch {
				known = false // A valid foreign or later authority is not this original.
				continue
			}
			if registration.ClientID != clientId || registration.PublicKey != original.PublicKey || !registration.Present {
				return nil, ErrClosedWorkIntegrity
			}
			if network, provider := providerNetworks[clientId]; provider && registration.NetworkID != network {
				return nil, errors.Join(ErrClosedWorkIntegrity, errors.New("original client registration differs from retained provider identity"))
			}
			result.RegisteredReports++
		}
		if terminals[0] > 1 || terminals[1] > 1 || clients[0] != ([16]byte{}) && clients[0] == clients[1] {
			return nil, ErrClosedWorkIntegrity
		}
		result.Contracts++
		if known && terminals[0] == 1 && terminals[1] == 1 {
			snapshot, err := decodeClosedWorkSnapshot(row, artifact.Epoch)
			if err != nil {
				return nil, err
			}
			lower, upper := min(totals[0], totals[1]), max(totals[0], totals[1])
			if snapshot.Expiry == nil && snapshot.Legacy == nil && lower+(upper-lower)/2 == uint64(*snapshot.ByteCount) {
				result.AmountJoins++
				if completeInventory {
					result.CompleteReportInventories++
				}
			}
		}
	}
	return result, ctx.Err()
}
