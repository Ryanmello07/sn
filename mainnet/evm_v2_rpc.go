// Settlement v2 readback compares exact runtime code, every getter and every
// reviewed storage word with the approved projections at one block selector.
// Missing archive values remain read errors, never a completed installation.
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
)

// One contract's exact runtime, getters and storage words at one selector.
func (self *evmOwnedChain) readback(ctx context.Context, contract evmCreatePlan, block any) error {
	name := contract.actionName()
	var code string
	if err := self.read(ctx, "eth_getCode", []any{contract.Address.Hex(), block}, &code); err != nil {
		return err
	}
	if code != "0x"+hex.EncodeToString(contract.Runtime) {
		return fmt.Errorf("%s readback runtime differs", name)
	}
	for _, getter := range contract.Getters {
		var output string
		if err := self.read(ctx, "eth_call", []any{map[string]any{"to": getter.target(contract.Address), "data": getter.Data}, block}, &output); err != nil {
			return err
		}
		if output != getter.Expected {
			return fmt.Errorf("%s readback getter %s differs", name, getter.Data)
		}
	}
	for _, word := range contract.Storage {
		var output string
		if err := self.read(ctx, "eth_getStorageAt", []any{contract.Address.Hex(), word.Slot, block}, &output); err != nil {
			return err
		}
		if output != word.Expected {
			return fmt.Errorf("%s readback storage slot %s differs", name, word.Slot)
		}
	}
	return nil
}

// Completion reads back the initialized proxy and its implementation at the
// binding's selector, beside vault2's own exact bound state.
func (self *evmOwnedChain) readbackSettlement2(ctx context.Context, plan evmCreatePlan, block any) error {
	if plan.Proxy == nil || plan.Coordinator == nil {
		return errors.New("settlement2 readback lacks its approved proxy and implementation")
	}
	if err := self.readback(ctx, *plan.Proxy, block); err != nil {
		return err
	}
	return self.readback(ctx, *plan.Coordinator, block)
}

// The one-shot binding needs a registered, unbound vault2 and the exact
// initialized proxy and implementation in both canonical and pending state.
// A coordinator that is already fixed, even to this proxy, prevents a send.
func (self *evmOwnedChain) admitVault2LinkTarget(ctx context.Context, plan evmCreatePlan, block map[string]any) error {
	if plan.Escrow == nil {
		return errors.New("vault2 binding lacks its registered vault2 projection")
	}
	for _, selector := range []any{block, "pending"} {
		if err := self.readback(ctx, *plan.Escrow, selector); err != nil {
			return err
		}
		if err := self.readbackSettlement2(ctx, plan, selector); err != nil {
			return err
		}
	}
	return nil
}
