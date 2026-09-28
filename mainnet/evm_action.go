// The CREATE owner imports public signed bytes and never owns a private key.
// One caller may advance at a time; canceled waiters cannot mutate custody.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
)

// A complete canonical observation is retained independently of CLI output.
type evmCreateReceipt struct {
	TransactionHash   string `json:"transaction_hash"`
	BlockHash         string `json:"evm_block_hash"`
	BlockNumber       uint64 `json:"evm_block_number"`
	NativeHash        string `json:"native_block_hash"`
	NativeNumber      uint64 `json:"native_block_number"`
	Status            uint64 `json:"status"`
	GasUsed           uint64 `json:"gas_used"`
	EffectiveGasPrice string `json:"effective_gas_price_wei"`
	ContractAddress   string `json:"contract_address"`
	RuntimeHash       string `json:"runtime_hash,omitempty"`
	GetterHash        string `json:"getter_hash,omitempty"`
}

// Read results never grant another nonce or a replacement signature.
type evmActionObservation struct {
	Status     string
	SendReady  bool
	ScanNumber uint64
	ScanHash   string
	Receipt    *evmCreateReceipt
}

// The same owned adapter performs reconciliation, admission and exactly one
// HTTP write. Deterministic tests also exercise its real local HTTP route.
type evmActionChain interface {
	reconcile(context.Context, evmCreatePlan, evmActionRecord) (evmActionObservation, error)
	submit(context.Context, evmCreatePlan, evmActionRecord) error
}

// The result explicitly separates one contract from the full installation.
type evmCreateResult struct {
	PlanHash             string            `json:"plan_hash"`
	Status               string            `json:"status"`
	Address              string            `json:"reserve_address"`
	SigningDigest        string            `json:"signing_digest"`
	UnsignedTransaction  string            `json:"unsigned_transaction"`
	TransactionHash      string            `json:"transaction_hash,omitempty"`
	Attempts             uint8             `json:"attempts"`
	Receipt              *evmCreateReceipt `json:"receipt,omitempty"`
	InstallationComplete bool              `json:"installation_complete"`
	ActivationReady      bool              `json:"activation_ready"`
	RemainingActions     []string          `json:"remaining_actions"`
}

// All mutable projections are copied on construction. Storage failure poisons
// this instance; a new owner must reload actual durable state before proceeding.
type evmCreateOwner struct {
	plan   evmCreatePlan
	store  evmActionStorage
	chain  evmActionChain
	gate   chan struct{}
	failed error
}

// JSON copying preserves the exact public approval while severing caller slices.
func copyEvmPhaseConfig(config evmPhaseConfig) evmPhaseConfig {
	raw, _ := json.Marshal(config)
	var copied evmPhaseConfig
	_ = json.Unmarshal(raw, &copied)
	return copied
}

// No network operation or signature request happens during construction.
func newEvmCreateOwner(plan evmCreatePlan, store evmActionStorage, chain evmActionChain) (*evmCreateOwner, error) {
	if store == nil {
		return nil, errors.New("EVM custody storage is unavailable")
	}
	if err := plan.Config.validate(); err != nil {
		return nil, err
	}
	plan.Config = copyEvmPhaseConfig(plan.Config)
	plan.Runtime = append([]byte(nil), plan.Runtime...)
	plan.Getters = append([]contractGetter(nil), plan.Getters...)
	if _, err := store.load(); err != nil {
		return nil, err
	}
	return &evmCreateOwner{plan: plan, store: store, chain: chain, gate: make(chan struct{}, 1)}, nil
}

// A failed publication must never be followed by a network effect in this owner.
func (self *evmCreateOwner) retain(record *evmActionRecord) error {
	record.ContentHash = ""
	record.ContentHash = rootObjectHash(*record)
	if err := self.store.save(*record); err != nil {
		self.failed = err
		return err
	}
	return nil
}

// Offline mode only retains public bytes. Online mode reconciles first; submit
// additionally requires exact current admission and an unused finite attempt.
func (self *evmCreateOwner) advance(ctx context.Context, signed []byte, online, submit bool) (evmCreateResult, error) {
	var result evmCreateResult
	if ctx == nil || submit && !online {
		return result, errors.New("EVM operation context or online mode is invalid")
	}
	select {
	case self.gate <- struct{}{}:
	case <-ctx.Done():
		return result, ctx.Err()
	}
	defer func() { <-self.gate }()
	if err := errors.Join(ctx.Err(), self.failed); err != nil {
		return result, err
	}
	record, err := self.store.load()
	if err != nil {
		return result, err
	}
	p := self.plan.Config.Plan
	action := p.Actions[0]
	if len(signed) != 0 {
		tx, err := action.signed(signed)
		if err != nil {
			return result, err
		}
		encoded := "0x" + hex.EncodeToString(signed)
		if record.Signed != "" && record.Signed != encoded {
			return result, errors.New("EVM custody already owns different original signed bytes")
		}
		if record.Signed == "" {
			record.Signed, record.TransactionHash = encoded, tx.Hash().Hex()
			if err := self.retain(&record); err != nil {
				return result, err
			}
		}
	}
	status := "signature-awaiting-import"
	if record.Signed != "" {
		status = "signed-custody-complete"
	}
	if online {
		if self.chain == nil || record.Signed == "" {
			return result, errors.New("EVM online reconciliation requires retained signed bytes and an owned adapter")
		}
		observation, err := self.chain.reconcile(ctx, self.plan, record)
		if err != nil {
			return result, err
		}
		if observation.ScanNumber < record.ScanNumber || observation.ScanNumber == record.ScanNumber && observation.ScanHash != record.ScanHash {
			return result, errors.New("EVM reconciliation regressed retained ancestry")
		}
		if record.Receipt != nil && (observation.Receipt == nil || *record.Receipt != *observation.Receipt) {
			return result, errors.New("EVM canonical receipt changed after retention")
		}
		record.ScanNumber, record.ScanHash = observation.ScanNumber, observation.ScanHash
		record.Receipt = observation.Receipt
		if err := self.retain(&record); err != nil {
			return result, err
		}
		status = observation.Status
		if submit && observation.SendReady {
			if record.Receipt != nil || record.Attempts >= p.MaximumAttempts {
				status = "attempt-allowance-exhausted"
			} else {
				if err := ctx.Err(); err != nil {
					return result, err
				}
				record.Attempts++
				if err := self.retain(&record); err != nil {
					return result, err
				}
				// The attempt is uncertain before entering the transport, even if
				// cancellation or a malformed reply prevents acknowledgement.
				if err := self.chain.submit(ctx, self.plan, record); err != nil {
					return self.result(record, "submission-uncertain"), err
				}
				status = "submitted-awaiting-canonical-receipt"
			}
		}
	}
	return self.result(record, status), ctx.Err()
}

// Public signing material contains the exact envelope; it cannot sign itself.
func (self *evmCreateOwner) result(record evmActionRecord, status string) evmCreateResult {
	tx, _ := self.plan.Config.Plan.Actions[0].unsigned()
	raw, _ := tx.MarshalBinary()
	signer := types.LatestSignerForChainID(big.NewInt(mainnetEvmChainId))
	return evmCreateResult{PlanHash: self.plan.Config.Plan.hash(), Status: status, Address: self.plan.Address.Hex(), SigningDigest: signer.Hash(tx).Hex(), UnsignedTransaction: "0x" + hex.EncodeToString(raw), TransactionHash: record.TransactionHash, Attempts: record.Attempts, Receipt: record.Receipt, RemainingActions: []string{"vault-create", "coordinator-create", "escrow-register", "proxy-create", "reserve-link", "vault-link", "evidence-create", "evidence-anchor"}}
}
