// This command composes bounded local preparation. No option enables RPC,
// transaction broadcast, signature import or a validator service loop.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
)

// Exact accepted preparation and run-directory identities precede any mutation.
// Every resume reloads its independently pinned inputs before retained ownership.
func runBootstrapChainCommand(ctx context.Context, args []string, stdout, stderr io.Writer) (result int) {
	if len(args) < 2 || args[1] != "plan" && args[1] != "apply" && args[1] != "resume" {
		fmt.Fprintln(stderr, "bootstrap-chain requires plan, apply or resume for offline launch preparation")
		return 2
	}
	command := args[1]
	flags := flag.NewFlagSet("bootstrap-chain "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "canonical private JSON chain preparation config")
	runDirectory := flags.String("run-dir", "", "exact precreated private custody directory")
	accepted := flags.String("accept-plan-hash", "", "accepted offline preparation plan hash")
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || *configPath == "" ||
		command == "plan" && (*runDirectory != "" || *accepted != "") || command != "plan" && (*runDirectory == "" || !planSha256(*accepted)) {
		fmt.Fprintln(stderr, "bootstrap-chain plan needs --config; apply/resume also need --run-dir and --accept-plan-hash")
		return 2
	}
	preparation, err := loadBootstrapChainPreparation(ctx, *configPath)
	if err != nil {
		fmt.Fprintln(stderr, "bootstrap chain preparation inputs:", err)
		return 2
	}
	encoder := json.NewEncoder(stdout)
	if command == "plan" {
		if err := encoder.Encode(preparation.Plan); err != nil {
			fmt.Fprintln(stderr, "bootstrap chain plan output:", err)
			return 1
		}
		return 0
	}
	if *accepted != preparation.Plan.ContentHash || *runDirectory != preparation.Plan.Config.RunDirectory {
		fmt.Fprintln(stderr, "bootstrap chain accepted plan or run directory differs; no journal opened")
		return 3
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintln(stderr, "bootstrap chain canceled:", err)
		return 1
	}
	store, err := openBootstrapChainStore(preparation, command == "apply", nil)
	if err != nil {
		fmt.Fprintln(stderr, "bootstrap chain retained ownership:", err)
		return 3
	}
	defer func() {
		if err := store.close(); err != nil {
			fmt.Fprintln(stderr, "bootstrap chain close:", err)
			result = 1
		}
	}()
	prepared, err := advanceBootstrapChain(ctx, store, nil)
	if err != nil {
		fmt.Fprintln(stderr, "bootstrap chain local preparation stopped; retain all journals for resume:", err)
		return 1
	}
	if err := encoder.Encode(prepared); err != nil {
		fmt.Fprintln(stderr, "bootstrap chain output failed; resume the retained preparation:", err)
		return 1
	}
	return 0
}
