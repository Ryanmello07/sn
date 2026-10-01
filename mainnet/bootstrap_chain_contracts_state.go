// Installation inspection borrows original action journals under shared locks.
// Missing descendants stay unclaimed; partial claims require their original owner.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// This destination set contains only already implemented original owners.
// The unimplemented Safe reservation has no invented journal or lock marker.
func bootstrapContractStateFile(index int) string {
	return []string{evmCreateStateFile, evmVaultCreateStateFile, evmCoordinatorCreateStateFile, evmEscrowRegisterStateFile, evmProxyCreateStateFile, evmReserveLinkStateFile, evmVaultLinkStateFile, evmEvidenceCreateStateFile}[index]
}

// The descriptor, its named inode and the private physical parent must remain
// the same while a read-only owner borrows original action custody.
type bootstrapContractReadinessMarker struct {
	file     *os.File
	path     string
	root     bootstrapSuccessorRootIdentity
	expected string
}

// A replaced marker with identical contents is still a different local lock.
func (self *bootstrapContractReadinessMarker) checkpoint() error {
	if self == nil || self.file == nil {
		return errors.New("contract readiness marker is closed")
	}
	root, err := bootstrapSuccessorPhysicalRoot(filepath.Dir(self.path))
	if err != nil || root != self.root {
		return errors.Join(errors.New("contract readiness original physical directory changed"), err)
	}
	if err := bootstrapSuccessorPrivateRegular(self.file); err != nil {
		return err
	}
	opened, openErr := self.file.Stat()
	named, nameErr := os.Lstat(self.path)
	if openErr != nil || nameErr != nil || !os.SameFile(opened, named) || opened.Size() != int64(len(self.expected)) {
		return errors.Join(errors.New("contract readiness original marker custody changed"), openErr, nameErr)
	}
	raw, err := io.ReadAll(io.NewSectionReader(self.file, 0, int64(len(self.expected))+1))
	if err != nil || string(raw) != self.expected {
		return errors.Join(errors.New("contract readiness marker is incomplete or belongs to another original approval or predecessor"), err)
	}
	return nil
}

// Shared ownership releases only its descriptor, never a retained marker.
func (self *bootstrapContractReadinessMarker) close() error {
	if self == nil || self.file == nil {
		return nil
	}
	err := self.file.Close()
	self.file = nil
	return err
}

// A marker must already be complete, private and regular. Shared ownership
// cannot recover an interrupted claim or advance its original action.
func openBootstrapContractReadinessMarker(path, expected string) (_ *bootstrapContractReadinessMarker, resultErr error) {
	root, err := bootstrapSuccessorPhysicalRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path+".lock", syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	lock := &bootstrapContractReadinessMarker{file: os.NewFile(uintptr(fd), path+".lock"), path: path + ".lock", root: root, expected: expected}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, lock.close())
		}
	}()
	if err := syscall.Flock(fd, syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		return nil, errors.Join(errors.New("contract readiness conflicts with an active custody owner"), err)
	}
	if err := lock.checkpoint(); err != nil {
		return nil, err
	}
	return lock, nil
}

// All retained ancestors stay locked until inspection ends. A later incomplete
// journal leaves earlier validated receipts visible, with accounting unresolved.
func inspectBootstrapContractCustody(ctx context.Context, plans []evmCreatePlan, result *bootstrapChainContractReadiness) (resultErr error) {
	if ctx == nil || result == nil || len(plans) == 0 || len(plans) > 8 || len(result.Actions) != 9 {
		return errors.New("contract custody inspection lacks its approved projections")
	}
	locks := []*bootstrapContractReadinessMarker{}
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			resultErr = errors.Join(resultErr, locks[i].close())
		}
		if resultErr != nil {
			result.Status, result.CustodyInspectionComplete, result.RemainingOriginalAttempts = "unresolved", false, nil
		}
	}()
	config := plans[0].Config
	configHash := rootObjectHash(config)
	records := []evmActionRecord{}
	for i, plan := range plans {
		if err := ctx.Err(); err != nil {
			return err
		}
		action := &result.Actions[i]
		action.CustodyStatus = "unresolved"
		path := filepath.Join(config.Plan.RunDirectory, bootstrapContractStateFile(i))
		_, markerErr := os.Lstat(path + ".lock")
		_, journalErr := os.Lstat(path)
		if i > 0 && errors.Is(markerErr, os.ErrNotExist) && errors.Is(journalErr, os.ErrNotExist) {
			action.CustodyStatus = "not-claimed"
			continue
		}
		if markerErr != nil || journalErr != nil {
			return errors.Join(fmt.Errorf("contract readiness %s has missing or incomplete retained custody", action.Id), markerErr, journalErr)
		}
		marker := configHash + "\n"
		if i > 0 {
			if len(records) != i || records[i-1].Receipt == nil || records[i-1].Receipt.Status != 1 {
				return fmt.Errorf("contract readiness %s exists without its complete original predecessor", action.Id)
			}
			marker = rootObjectHash(struct{ ConfigHash, ActionId, PredecessorHash string }{ConfigHash: configHash, ActionId: action.Id, PredecessorHash: rootObjectHash(records[i-1])}) + "\n"
		}
		lock, err := openBootstrapContractReadinessMarker(path, marker+bootstrapRootClaimComplete)
		if err != nil {
			return fmt.Errorf("contract readiness %s: %w", action.Id, err)
		}
		locks = append(locks, lock)
		raw, _, err := readBootstrapRootFile(ctx, path, 512*1024)
		if err != nil {
			return err
		}
		var record evmActionRecord
		if err := decodePlanJson(raw, &record); err != nil {
			return err
		}
		plan.Prerequisites = records
		if err := errors.Join(record.validateForAction(config, i), validateEvmCreatePrerequisite(plan, record)); err != nil {
			return fmt.Errorf("contract readiness %s: %w", action.Id, err)
		}
		if record.Receipt != nil && record.Receipt.Status == 1 {
			if err := validateEvmCreateCompletion(plan, record); err != nil {
				return fmt.Errorf("contract readiness %s completion: %w", action.Id, err)
			}
			if i > 0 && (record.Receipt.NativeNumber < records[i-1].Receipt.NativeNumber || record.Receipt.BlockNumber < records[i-1].Receipt.BlockNumber) {
				return fmt.Errorf("contract readiness %s inclusion precedes its original predecessor", action.Id)
			}
		}
		result.RetainedAttempts += uint16(record.Attempts)
		if result.RetainedAttempts > uint16(config.Plan.MaximumAttempts) {
			return errors.New("contract readiness cumulative attempts exceed the original graph allowance")
		}
		action.JournalHash, action.CustodyHash, action.TransactionHash = record.ContentHash, rootObjectHash(record), record.TransactionHash
		action.Attempts, action.Receipt = record.Attempts, record.Receipt
		action.CustodyStatus = "retained-awaiting-signature"
		if record.Signed != "" {
			action.CustodyStatus = "retained-signature-reconciliation-pending"
		}
		if record.Receipt != nil {
			action.ReceiptObservation, action.CustodyStatus = "retained", "retained-reverted"
			if record.Receipt.Status == 1 {
				action.CustodyStatus = "retained-complete"
				result.RetainedCompletedActions++
				result.SuccessorRequirements.UnfinishedActions = result.SuccessorRequirements.UnfinishedActions[1:]
			} else {
				result.Blockers = append(result.Blockers, "REVERTED_ORIGINAL_ACTION_REQUIRES_SEPARATE_RECOVERY_AUTHORITY")
			}
		}
		records = append(records, record)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for i, lock := range locks {
		if err := lock.checkpoint(); err != nil {
			return err
		}
		raw, _, err := readBootstrapRootFile(ctx, filepath.Join(config.Plan.RunDirectory, bootstrapContractStateFile(i)), 512*1024)
		var current evmActionRecord
		if err == nil {
			err = decodePlanJson(raw, &current)
		}
		if err != nil || rootObjectHash(current) != rootObjectHash(records[i]) {
			return errors.Join(errors.New("contract readiness original journal changed during inspection"), err)
		}
	}
	remaining := uint16(config.Plan.MaximumAttempts) - result.RetainedAttempts
	result.Status, result.CustodyInspectionComplete, result.RemainingOriginalAttempts = "blocked", true, &remaining
	if int(result.RetainedCompletedActions) != len(plans) {
		result.Blockers = append(result.Blockers, "APPROVED_EXECUTABLE_PREFIX_INCOMPLETE")
	}
	return nil
}
