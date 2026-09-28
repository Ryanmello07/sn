// The executable contract phase prepares or imports public custody offline.
// Only explicit online resume can reconcile; only --submit permits one write.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
)

// Commands preserve original custody even when result publication fails.
// Plan output names the first implemented action and never claims installation.
func runBootstrapContractCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "bootstrap-contracts" {
		fmt.Fprintln(stderr, "usage: bootstrap-contracts preview|plan|apply|resume --config FILE")
		return 2
	}
	command := args[1]
	if command != "preview" && command != "plan" && command != "apply" && command != "resume" {
		fmt.Fprintln(stderr, "unknown contract phase command")
		return 2
	}
	flags := flag.NewFlagSet("bootstrap-contracts "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "phase configuration; only preview accepts an unsigned draft")
	accepted := flags.String("accept-plan-hash", "", "exact reviewed phase hash")
	runDirectory := flags.String("run-dir", "", "approved private journal directory")
	signaturePath := flags.String("signed-transaction", "", "private regular file containing original public transaction bytes")
	signatureHash := flags.String("signed-transaction-hash", "", "sha256 pin of original public signed-byte file")
	online := flags.Bool("online", false, "read the independently approved owned RPC route")
	submit := flags.Bool("submit", false, "permit one originally approved, durably counted HTTP submission")
	review := command == "plan" || command == "preview"
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || *configPath == "" || review && (*accepted != "" || *runDirectory != "" || *signaturePath != "" || *signatureHash != "" || *online || *submit) || !review && (*accepted == "" || *runDirectory == "") || (*signaturePath == "") != (*signatureHash == "") || *signaturePath != "" && (command != "resume" || !planSha256(*signatureHash)) || (*online || *submit) && command != "resume" || *submit && !*online {
		fmt.Fprintln(stderr, "contract phase requires exact config/plan/run directory; only resume accepts pinned signed bytes, --online and --submit")
		return 2
	}
	encoder := json.NewEncoder(stdout)
	if command == "preview" {
		preview, err := loadEvmPhasePreview(ctx, *configPath)
		if err != nil {
			fmt.Fprintln(stderr, "contract phase unsigned preview:", err)
			return 2
		}
		if err := encoder.Encode(preview); err != nil {
			fmt.Fprintln(stderr, "contract phase unsigned preview output:", err)
			return 1
		}
		return 0
	}
	plan, err := loadEvmCreatePlan(ctx, *configPath)
	if err != nil {
		fmt.Fprintln(stderr, "contract phase authority:", err)
		return 2
	}
	if command == "plan" {
		if err := encoder.Encode(struct {
			Plan                 evmPhasePlan `json:"plan"`
			PlanHash             string       `json:"plan_hash"`
			ExecutableAction     string       `json:"executable_action"`
			InstallationComplete bool         `json:"installation_complete"`
		}{Plan: plan.Config.Plan, PlanHash: plan.Config.Plan.hash(), ExecutableAction: "reserve-create", InstallationComplete: false}); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if *accepted != plan.Config.Plan.hash() || *runDirectory != plan.Config.Plan.RunDirectory {
		fmt.Fprintln(stderr, "contract phase acceptance differs; no journal opened")
		return 3
	}
	var signed []byte
	if *signaturePath != "" {
		state := filepath.Join(*runDirectory, evmCreateStateFile)
		if *signaturePath == state || *signaturePath == state+".lock" {
			fmt.Fprintln(stderr, "signed-byte input aliases custody")
			return 2
		}
		raw, digest, err := readBootstrapRootFile(ctx, *signaturePath, 128*1024)
		if err != nil || digest != *signatureHash {
			fmt.Fprintln(stderr, "signed-byte file:", errors.Join(errors.New("exact public signed-byte pin differs"), err))
			return 2
		}
		if _, err := plan.Config.Plan.Actions[0].signed(raw); err != nil {
			fmt.Fprintln(stderr, "signed-byte envelope:", err)
			return 2
		}
		signed = raw
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	store, err := openEvmActionStore(plan.Config, command == "apply", nil)
	if err != nil {
		fmt.Fprintln(stderr, "contract phase retained ownership:", err)
		return 3
	}
	defer store.close()
	var chain evmActionChain
	if *online {
		adapter, err := newEvmOwnedChain(plan.Config)
		if err != nil {
			fmt.Fprintln(stderr, "contract phase owned route:", err)
			return 3
		}
		chain = adapter
		defer adapter.client.httpClient.CloseIdleConnections()
	}
	owner, err := newEvmCreateOwner(plan, store, chain)
	if err != nil {
		fmt.Fprintln(stderr, "contract phase retained state:", err)
		return 3
	}
	result, err := owner.advance(ctx, signed, *online, *submit)
	if err != nil {
		fmt.Fprintln(stderr, "contract phase stopped; retain original journal for resume:", err)
		return 1
	}
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(stderr, "contract phase output failed; resume authoritative journal:", err)
		return 1
	}
	return 0
}
