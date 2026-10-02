// The composed public path joins bootstrap's prepared head to the actual
// monitor writer, without signing or invoking a service manager.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Bootstrap leaves a fresh, independently prepared monitor head. The public
// passive command must consume that exact head and retain its completed bytes
// across restart; missing custody cannot become a fresh observation campaign.
func TestDurableCompositionPassiveBootstrapStartsRetainedMonitor(t *testing.T) {
	f := newBootstrapRootPassiveFixture(t)
	prepared := f.result(t, "apply")
	if prepared.NetworkEffects || prepared.ActivationReady || prepared.Root.Broadcasts != 0 {
		t.Fatal("synthetic bootstrap widened authority", prepared)
	}
	config := rootPassiveRuntimeConfig{Schema: rootPassiveRuntimeSchema, Root: f.config.Root, Role: *f.config.RootValidator}
	reference := bootstrapRootTestWrite(t, filepath.Join(filepath.Dir(f.path), "composed-passive-runtime.json"), config)
	args := []string{"root-passive-service", "run", "--config", reference.Path, "--accept-runtime-sha256", reference.Sha256}
	ctx := f.storageContext(t.Context())
	var output, diagnostic bytes.Buffer
	if code := runMain(ctx, args, &output, &diagnostic); code != 0 {
		t.Fatal("prepared bootstrap and monitor did not compose", code, diagnostic.String())
	}
	var event rootMonitorEvent
	if err := json.Unmarshal(output.Bytes(), &event); err != nil || event.Status != "ready" || event.Observation == nil {
		t.Fatal("composed observer omitted its real sample", err, output.String())
	}
	path := f.root.plan.PassiveService.CheckpointPath
	original, err := os.ReadFile(path)
	if err != nil || len(original) == 0 {
		t.Fatal("composed monitor did not retain its checkpoint", err)
	}
	output.Reset()
	diagnostic.Reset()
	if code := runMain(ctx, args, &output, &diagnostic); code != 0 {
		t.Fatal("retained monitor did not reopen after joined public run", code, diagnostic.String())
	}
	retained := path + ".original"
	if err := os.Rename(path, retained); err != nil {
		t.Fatal(err)
	}
	completed, err := os.ReadFile(retained)
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	diagnostic.Reset()
	if code := runMain(ctx, args, &output, &diagnostic); code == 0 {
		t.Fatal("completed monitor loss was silently initialized", output.String())
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("refused composed restart recreated custody", err)
	}
	if raw, err := os.ReadFile(retained); err != nil || !bytes.Equal(raw, completed) {
		t.Fatal("refused composed restart changed retained checkpoint", err)
	}
}
