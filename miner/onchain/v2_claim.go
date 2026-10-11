package onchain

// v2_claim.go — `snclaim v2 status` and `snclaim v2 claim` (VERSION2-FINAL.md
// §10.2): a coldkey's accounts as the ledger endpoint lists them, with the
// claimed watermark read from vault2, and one claim per call. A claim pays
// cumulative − claimed to the account's coldkey whoever sends it, so a
// third party having claimed first is success (SPEC A6).

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"

	docopt "github.com/docopt/docopt-go"

	"github.com/urfoundation/sn/merkle"
)

// cmdV2Status implements `snclaim v2 status`: per pool, the account index,
// the finalized cumulative, claimable, claimed (read from vault2) and the
// pending cumulative. An endpoint that cannot answer shows "unavailable",
// never zero.
func cmdV2Status(ctx context.Context, opts docopt.Opts) error {
	coldkey, err := parseColdkey(strOpt(opts, "--coldkey"))
	if err != nil {
		return err
	}
	vault, err := parseVault2(strOpt(opts, "--vault"))
	if err != nil {
		return err
	}
	api, err := parseAPIURL(strOpt(opts, "--api_url"))
	if err != nil {
		return err
	}
	wantChainID, _, err := parseChainAndGas(opts)
	if err != nil {
		return err
	}
	client, chainID, rpcURL, err := v2Connect(ctx, opts, wantChainID)
	if err != nil {
		return err
	}
	defer client.Close()

	fmt.Printf("settlement v2 account status — vault %s (chain id %s, rpc %s)\n", vault.Hex(), chainID, rpcURL)
	fmt.Printf("  coldkey:      %s\n", renderColdkey(coldkey))
	accounts, err := fetchV2Accounts(ctx, api, coldkey)
	if err != nil {
		fmt.Printf("  accounts:     unavailable (%v)\n", err)
		return nil
	}
	if len(accounts) == 0 {
		fmt.Printf("  accounts:     none\n")
		return nil
	}
	reader := &v2Reader{client: client, vault: vault}
	for i := range accounts {
		account := &accounts[i]
		if err := account.check(vault); err != nil {
			return err
		}
		claimed, err := v2View(ctx, reader, vault2.PackClaimed(new(big.Int).SetUint64(account.NoID), new(big.Int).SetUint64(account.Index)), vault2.UnpackClaimed)
		if err != nil {
			return fmt.Errorf("claimed(%d,%d): %w", account.NoID, account.Index, err)
		}
		finalized, claimable := "none (the account is only in the pending root)", formatAlpha(new(big.Int))
		if account.RootIndex != nil {
			cumulative, err := v2Amount("cumulative", account.Cumulative)
			if err != nil {
				return err
			}
			finalized = fmt.Sprintf("%s at root %d", v2Alpha(cumulative), *account.RootIndex)
			if remaining := new(big.Int).Sub(cumulative.ToBig(), claimed); remaining.Sign() > 0 {
				claimable = formatAlpha(remaining)
			}
		}
		pending := "none"
		if account.PendingCumulative != nil {
			value, err := v2Amount("pending_cumulative", *account.PendingCumulative)
			if err != nil {
				return err
			}
			pending = v2Alpha(value)
		}
		fmt.Printf("  pool %d, account %d:\n", account.NoID, account.Index)
		fmt.Printf("    finalized:  %s\n", finalized)
		fmt.Printf("    claimed:    %s\n", formatAlpha(claimed))
		fmt.Printf("    claimable:  %s\n", claimable)
		fmt.Printf("    pending:    %s\n", pending)
	}
	return nil
}

// cmdV2Claim implements `snclaim v2 claim`: it takes the coldkey's one
// finalized account, or the one --no_id and --index pick, verifies the
// endpoint's proof against the root vault2 holds, refuses a mismatch, and
// sends claim. An account whose claimed watermark already reaches its
// cumulative, before the send or after a refused one, is reported as done.
func cmdV2Claim(ctx context.Context, opts docopt.Opts) error {
	coldkey, err := parseColdkey(strOpt(opts, "--coldkey"))
	if err != nil {
		return err
	}
	vault, err := parseVault2(strOpt(opts, "--vault"))
	if err != nil {
		return err
	}
	api, err := parseAPIURL(strOpt(opts, "--api_url"))
	if err != nil {
		return err
	}
	noID, pickNoID, err := parseUint64Opt(opts, "--no_id")
	if err != nil {
		return err
	}
	index, pickIndex, err := parseUint64Opt(opts, "--index")
	if err != nil {
		return err
	}
	wantChainID, gasLimit, err := parseChainAndGas(opts)
	if err != nil {
		return err
	}
	key, err := loadKeyFile(strOpt(opts, "--key_file"))
	if err != nil {
		return err
	}
	client, chainID, rpcURL, err := v2Connect(ctx, opts, wantChainID)
	if err != nil {
		return err
	}
	defer client.Close()

	accounts, err := fetchV2Accounts(ctx, api, coldkey)
	if err != nil {
		return fmt.Errorf("ledger endpoint: %w", err)
	}
	var picked []*v2Account
	for i := range accounts {
		account := &accounts[i]
		if account.RootIndex != nil && (!pickNoID || account.NoID == noID) && (!pickIndex || account.Index == index) {
			picked = append(picked, account)
		}
	}
	if len(picked) != 1 {
		names := make([]string, len(picked))
		for i, account := range picked {
			names[i] = fmt.Sprintf("--no_id=%d --index=%d", account.NoID, account.Index)
		}
		return fmt.Errorf("coldkey %s has %d finalized accounts matching the selection [%s]; claim exactly one", renderColdkey(coldkey), len(picked), strings.Join(names, ", "))
	}
	account := picked[0]
	if err := account.check(vault); err != nil {
		return err
	}
	cumulative, err := v2Amount("cumulative", account.Cumulative)
	if err != nil {
		return err
	}
	proof, err := account.proof()
	if err != nil {
		return err
	}

	reader := &v2Reader{client: client, vault: vault}
	noIDBig, indexBig, rootIndex := new(big.Int).SetUint64(account.NoID), new(big.Int).SetUint64(account.Index), new(big.Int).SetUint64(*account.RootIndex)
	root, err := v2View(ctx, reader, vault2.PackRoots(noIDBig, rootIndex), vault2.UnpackRoots)
	if err != nil {
		return fmt.Errorf("roots(%d,%d): %w", account.NoID, *account.RootIndex, err)
	}
	if !merkle.VerifyLedgerProof(root.Root, uint256.MustFromBig(root.Total), root.LeafCount, account.Index, coldkey, cumulative, proof) {
		return fmt.Errorf("the ledger endpoint's proof for pool %d account %d does not verify against root %d (0x%x) in vault2; nothing sent", account.NoID, account.Index, *account.RootIndex, root.Root)
	}
	readClaimed := func() (*big.Int, error) {
		claimed, err := v2View(ctx, reader, vault2.PackClaimed(noIDBig, indexBig), vault2.UnpackClaimed)
		if err != nil {
			return nil, fmt.Errorf("claimed(%d,%d): %w", account.NoID, account.Index, err)
		}
		return claimed, nil
	}
	claimed, err := readClaimed()
	if err != nil {
		return err
	}
	if claimed.Cmp(cumulative.ToBig()) >= 0 {
		fmt.Printf("pool %d account %d: claimed %s already reaches the cumulative %s; nothing to claim\n", account.NoID, account.Index, formatAlpha(claimed), v2Alpha(cumulative))
		return nil
	}

	sender := &v2Sender{client: client, chainID: chainID, rpcURL: rpcURL, vault: vault, from: crypto.PubkeyToAddress(key.PublicKey), key: key, gasLimit: gasLimit, dryRun: boolOpt(opts, "--dry-run")}
	calldata := vault2.PackClaim(noIDBig, rootIndex, v2LedgerAccount(account.Index, &merkle.LedgerAccount{Coldkey: coldkey, Cumulative: *cumulative}), v2ProofNodes(proof))
	receipt, err := sender.send(ctx, "claim", calldata, nil, func() {
		fmt.Printf("  account:     pool %d, account %d, root %d\n", account.NoID, account.Index, *account.RootIndex)
		fmt.Printf("  coldkey:     %s\n", renderColdkey(coldkey))
		fmt.Printf("  cumulative:  %s\n", v2Alpha(cumulative))
		fmt.Printf("  claimed:     %s\n", formatAlpha(claimed))
		fmt.Printf("  claimable:   %s\n", formatAlpha(new(big.Int).Sub(cumulative.ToBig(), claimed)))
		fmt.Printf("  proof:       %d siblings, verified against root 0x%x\n", len(proof), root.Root)
	})
	if err != nil {
		if !v2Reverted(err) {
			return err
		}
		now, rerr := readClaimed()
		if rerr != nil {
			return errors.Join(err, rerr)
		}
		if now.Cmp(cumulative.ToBig()) < 0 {
			return err
		}
		fmt.Printf("pool %d account %d: another sender claimed it first (claimed %s); done\n", account.NoID, account.Index, formatAlpha(now))
		return nil
	}
	if receipt == nil {
		return nil // dry run
	}
	for _, log := range receipt.Logs {
		if log.Address != vault {
			continue
		}
		event, err := vault2.UnpackClaimedEvent(log)
		if err != nil || event.NoId.Cmp(noIDBig) != 0 || event.Index.Cmp(indexBig) != 0 {
			continue
		}
		fmt.Printf("Claimed: pool %s, account %s, root %s\n", event.NoId, event.Index, event.RootIndex)
		fmt.Printf("  coldkey:     %s\n", renderColdkey(event.Coldkey))
		fmt.Printf("  cumulative:  %s\n", formatAlpha(event.Cumulative))
		fmt.Printf("  paid:        %s\n", formatAlpha(event.Amount))
		fmt.Printf("  caller:      %s\n", event.Caller.Hex())
		return nil
	}
	return fmt.Errorf("tx %s succeeded but %s emitted no Claimed for pool %d account %d", receipt.TxHash, vault.Hex(), account.NoID, account.Index)
}
