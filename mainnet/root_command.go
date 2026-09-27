// A bounded root observer reuses finalized continuity and durable checkpoints.
// It has no signer, transaction constructor or submission capability.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"
)

const rootEventSchema = "urnetwork-mainnet-root-monitor-event-v1"

// Failed samples never retain a prior ready observation as their current state.
type rootMonitorEvent struct {
	Schema     string               `json:"schema"`
	Sample     int                  `json:"sample"`
	ObservedAt string               `json:"observed_at"`
	Status     string               `json:"status"`
	Detail     string               `json:"detail,omitempty"`
	Snapshot   *rootPreviewEnvelope `json:"snapshot,omitempty"`
}

// Read-only readiness is a narrow observation result, never activation approval.
func runRootCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	rpcUrl := flags.String("rpc", "", "explicit owned HTTP(S) RPC URL")
	policyPath := flags.String("policy", "", "independently approved root observer policy JSON")
	retryWindow := flags.Duration("retry-window", 300*time.Second, "one bounded complete sample window, at least 60 seconds")
	interval := flags.Duration("interval", 30*time.Second, "delay after each completed root monitor sample")
	samples := flags.Int("samples", 1, "finite monitor sample count, 1 through 10000")
	stallAfter := flags.Duration("stall-after", 5*time.Minute, "finalized progress alert threshold")
	checkpointPath := flags.String("checkpoint", "", "absolute durable finalized checkpoint path for root-monitor")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *rpcUrl == "" || *policyPath == "" || *retryWindow < 60*time.Second || *retryWindow > 15*time.Minute || *samples < 1 || *samples > 10000 || *interval <= 0 || *stallAfter <= 0 {
		fmt.Fprintln(stderr, "root command requires --rpc URL --policy FILE; retry-window must be 60s..15m, samples 1..10000 and intervals positive")
		return 2
	}
	if command == "root-preview" && (*samples != 1 || *checkpointPath != "") {
		fmt.Fprintln(stderr, "root-preview takes one sample and no checkpoint")
		return 2
	}
	var policy rootValidatorPolicy
	policyHash, err := readEconomicInput(*policyPath, &policy)
	if err == nil {
		err = policy.validate()
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	client, err := newRpcClient(*rpcUrl, *retryWindow)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	defer client.httpClient.CloseIdleConnections()
	state := &monitorState{}
	var checkpoint *monitorCheckpointStore
	if *checkpointPath != "" {
		checkpoint, err = openMonitorCheckpoint(*checkpointPath, identityExpectation{NativeChain: policy.NativeChain, GenesisHash: policy.GenesisHash, EvmChainId: policy.EvmChainId})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
		defer checkpoint.close()
		state, err = checkpoint.load()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
	}
	encoder := json.NewEncoder(stdout)
	lastExit := 0
	for sample := 1; sample <= *samples; sample++ {
		if ctx.Err() != nil {
			return 0
		}
		event := rootMonitorEvent{Schema: rootEventSchema, Sample: sample}
		preview, readErr := client.readRootPreview(ctx, policy, policyHash)
		now := time.Now().UTC()
		event.ObservedAt = now.Format(time.RFC3339Nano)
		lastExit = 0
		if readErr != nil {
			if ctx.Err() != nil {
				return 0
			}
			event.Status, event.Detail = "rpc-error", readErr.Error()
			lastExit = 1
			if errors.Is(readErr, errRpcIntegrity) {
				event.Status = "rpc-integrity"
				lastExit = 3
			}
		} else {
			// Continuity has the same bounded budget as existing mainnet monitor
			// reads and rechecks the retained height, not the previous tip.
			continuous, continuityErr := client.priorFinalizedMatches(ctx, state, preview.Identity)
			if continuityErr != nil {
				event.Status, event.Detail = "rpc-error", continuityErr.Error()
				lastExit = 1
				if errors.Is(continuityErr, errRpcIntegrity) {
					event.Status = "rpc-integrity"
					lastExit = 3
				}
			} else if !continuous {
				event.Status, event.Detail = "finality-conflict", "previous finalized block changed at its retained height"
				lastExit = 3
			} else {
				previousHash, previousNumber := state.lastHash, state.lastNumber
				status, observeErr := state.observe(now, preview.Identity, *stallAfter)
				if observeErr != nil {
					event.Status, event.Detail = status, observeErr.Error()
					lastExit = 3
				} else {
					event.Status = preview.Status
					if !preview.ReadOnlyReady {
						lastExit = 3
					}
					if status == "finality-stalled" {
						event.Status = status
						preview.ReadOnlyReady, preview.Status = false, "blocked"
						preview.Blockers = append(preview.Blockers, "ROOT_FINALITY_STALLED")
						lastExit = 3
					}
					if checkpoint != nil && (state.lastHash != previousHash || state.lastNumber != previousNumber) {
						if saveErr := checkpoint.save(state); saveErr != nil {
							event.Status, event.Detail = "checkpoint-error", saveErr.Error()
							preview.ReadOnlyReady, preview.Status = false, "blocked"
							preview.Blockers = append(preview.Blockers, "ROOT_FINALITY_CHECKPOINT_FAILED")
							lastExit = 1
						}
					}
					sealed, sealErr := sealRootPreview(preview)
					if sealErr != nil {
						fmt.Fprintln(stderr, sealErr)
						return 1
					}
					event.Snapshot = &sealed
				}
			}
		}
		if err := encoder.Encode(event); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if event.Status == "rpc-integrity" || event.Status == "finality-conflict" || event.Status == "checkpoint-error" {
			return lastExit
		}
		if sample == *samples {
			return lastExit
		}
		select {
		case <-ctx.Done():
			return 0
		case <-time.After(*interval):
		}
	}
	return lastExit
}
