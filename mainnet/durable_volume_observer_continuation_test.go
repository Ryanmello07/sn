// A signed finite passive service spends one sample on an observation outage,
// not all of its remaining samples. The physical owners stay joined to the run.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/urfoundation/sn/internal/durablepath"
	"golang.org/x/sys/unix"
)

func TestDurableCompositionPassiveUnavailableSampleContinuesBeforeRpc(t *testing.T) {
	compositionPassiveUnavailableSample(t, false, "recover")
}

func TestDurableCompositionPassiveUnavailableSampleContinuesAfterRpc(t *testing.T) {
	compositionPassiveUnavailableSample(t, true, "recover")
}

func TestDurableCompositionPassiveUnavailableSampleCancellationJoins(t *testing.T) {
	compositionPassiveUnavailableSample(t, false, "cancel")
}

func TestDurableCompositionPassiveUnavailableSampleIdentityLossStops(t *testing.T) {
	compositionPassiveUnavailableSample(t, false, "identity")
}

// Each variant crosses the actual public two-sample command. Only the kernel
// fact observation and owned wait are injected; signing uses synthetic keys
// before preparation, and all root, lock, head and checkpoint bytes are real.
func compositionPassiveUnavailableSample(t *testing.T, afterRpc bool, transition string) {
	t.Helper()
	fixture := newBootstrapRootPassiveFixture(t)
	service := *fixture.root.plan.PassiveService
	service.MaximumSamples = 2
	fixture.root.config.RootService = bootstrapRootTestWrite(t, fixture.root.config.RootService.Path, service)
	fixture.config.Root = bootstrapRootTestWrite(t, fixture.root.configPath, fixture.root.config)
	var err error
	fixture.root.plan, err = loadBootstrapRootPlan(t.Context(), fixture.root.configPath)
	if err != nil {
		t.Fatal(err)
	}
	fixture.rootRole.approval.RootPlanHash = fixture.root.plan.ContentHash
	fixture.rootRole.approval.ServiceConfigHash = fixture.root.plan.serviceHash()
	fixture.rootRole.sign(t)
	bootstrapRootTestWrite(t, fixture.path, fixture.config)
	fixture.preparation, err = loadBootstrapChainPreparation(t.Context(), fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	args := compositionPassiveArguments(t, fixture)
	beforeReads := compositionRpcReads(fixture.census)
	path := filepath.Join(fixture.root.plan.RunDirectory, bootstrapRootProgressFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	var preparationFile, checkpointFile *os.File
	var preparationIdentity, checkpointIdentity os.FileInfo
	failed, recovered := false, false
	host := &compositionObservationHost{Host: fixture.root.storage.Host}
	host.observe = func(file *os.File) error {
		if preparationFile == nil {
			preparationFile = file
			preparationIdentity, err = file.Stat()
			if err != nil {
				t.Fatal(err)
			}
		} else if file != preparationFile && checkpointFile == nil {
			checkpointFile = file
			checkpointIdentity, err = file.Stat()
			if err != nil {
				t.Fatal(err)
			}
		}
		boundary := checkpointFile != nil
		if afterRpc {
			boundary = compositionRpcReads(fixture.census) > beforeReads
		}
		if file == preparationFile && boundary && !recovered {
			failed = true
			return unix.EIO
		}
		return nil
	}
	checkRetainedOwners := func() {
		t.Helper()
		if preparationFile == nil || checkpointFile == nil {
			t.Fatal("sample did not retain both physical owners")
		}
		for _, owner := range []struct {
			file     *os.File
			identity os.FileInfo
		}{{file: preparationFile, identity: preparationIdentity}, {file: checkpointFile, identity: checkpointIdentity}} {
			current, err := owner.file.Stat()
			if err != nil || !os.SameFile(current, owner.identity) {
				t.Fatal("sample replaced or closed its retained owner", err)
			}
		}
	}
	waits, events := 0, 0
	hooks := monitorServiceHooks{
		wait: func(waitCtx context.Context, role string, delay time.Duration) bool {
			waits++
			if !failed || waitCtx.Err() != nil || role != "root" || delay <= 0 || delay > 30*time.Second {
				t.Fatal("invalid bounded sample wait", failed, waitCtx.Err(), role, delay)
			}
			checkRetainedOwners()
			if events == 0 {
				if waits > 64 || recovered {
					t.Fatal("sample exceeded its retry budget", waits, recovered)
				}
			} else if events == 1 {
				if waits != 65 || delay != time.Second {
					t.Fatal("soft failure skipped the signed inter-sample interval", waits, delay)
				}
				if transition == "cancel" {
					cancel()
					return false
				}
				recovered = true
				if transition == "identity" {
					if err := os.Rename(path, path+".retained"); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				t.Fatal("command exceeded its signed two-sample allowance", events)
			}
			return true
		},
		afterEvent: func(context.Context, string) {
			events++
			checkRetainedOwners()
			if events == 1 {
				if waits != 64 {
					t.Fatal("failed sample was not bounded", waits)
				}
				if _, err := os.Lstat(service.CheckpointPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("unavailable preparation changed finalized checkpoint custody", err)
				}
				wantReads := beforeReads
				if afterRpc {
					wantReads++
				}
				if reads := compositionRpcReads(fixture.census); reads != wantReads {
					t.Fatal("unavailable sample invented an extra network observation", reads, wantReads)
				}
			}
		},
	}
	ctx := durablepath.WithHost(fixture.storageContext(parent), host)
	var output, diagnostics bytes.Buffer
	code := runMainWithMonitorHooks(ctx, args, &output, &diagnostics, time.Now, hooks)
	wantCode, wantEvents := 0, 2
	if transition == "cancel" {
		wantEvents = 1
	} else if transition == "identity" {
		wantCode, wantEvents = 3, 1
	}
	if code != wantCode || waits != 65 || events != wantEvents || !failed {
		t.Fatal("soft preparation outage abandoned unused signed samples or custody", code, waits, events, failed, diagnostics.String())
	}
	for _, file := range []*os.File{preparationFile, checkpointFile} {
		if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatal("terminal public run did not join its physical owners", err)
		}
	}
	decoder := json.NewDecoder(&output)
	var first rootMonitorEvent
	if err := decoder.Decode(&first); err != nil || first.Sample != 1 || first.Status != "storage-unavailable" || first.Observation != nil || first.Snapshot != nil || first.ReadPhase != "preparation" || first.ReadCause != "unavailable" {
		t.Fatal("failed sample invented freshness or omitted its bounded cause", first, err)
	}
	if transition == "recover" {
		var second rootMonitorEvent
		if err := decoder.Decode(&second); err != nil || second.Sample != 2 || second.Status != "ready" || second.Observation == nil || !second.Observation.ReadOnlyReady {
			t.Fatal("next signed sample did not recover on retained custody", second, err)
		}
		if raw, err := os.ReadFile(service.CheckpointPath); err != nil || len(raw) == 0 {
			t.Fatal("recovered sample did not retain its finalized checkpoint", err)
		}
	} else if _, err := os.Lstat(service.CheckpointPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("terminal cancellation or loss published a checkpoint", err)
	}
	if err := decoder.Decode(&rootMonitorEvent{}); !errors.Is(err, io.EOF) {
		t.Fatal("command exceeded its signed sample allowance", err)
	}
	if transition == "identity" {
		path += ".retained"
	}
	if raw, err := os.ReadFile(path); err != nil || !bytes.Equal(before, raw) {
		t.Fatal("sample continuation changed original preparation bytes", err)
	}
}
