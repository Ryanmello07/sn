//go:build linux

// A restored terminal publication must reconcile its original transaction
// before the independent physical-adoption receipt can occupy its census slot.
package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The public preview test uses only pre-existing API names, allowing an exact
// test-only overlay on the settled-root implementation to show its refusal.
func localRebindPendingOutcomeControl(t *testing.T, boundary string) {
	t.Helper()
	f := newBootstrapSuccessorCanonicalFixture(t)
	owner, adapter, join := f.openRuntimeRevisions()
	if result, err := advanceBootstrapSuccessorExecution(t.Context(), owner, adapter, true); err != nil || !result.SubmissionAttempted || result.CumulativeAttempts != 9 {
		t.Fatal("synthetic original execution did not retain one actual attempt", result, err)
	}
	lost := errors.New("synthetic interrupted original terminal publication")
	reached := false
	owner.local.hook = func(stage string) error {
		if stage == bootstrapSuccessorExecutionEventName(2)+".intent:"+boundary {
			reached = true
			return lost
		}
		return nil
	}
	if _, err := advanceBootstrapSuccessorExecution(t.Context(), owner, adapter, false); !reached || !errors.Is(err, lost) {
		t.Fatal("terminal interruption missed its actual durable boundary", reached, err)
	}
	join()
	local, registry := f.original.config.RunDirectory, f.approval.Plan.Request.RegistryDirectory
	before := bootstrapSuccessorPreparationTestFiles(t, local)
	nonces := bootstrapSuccessorPreparationTestFiles(t, registry)
	var oldCensus bootstrapSuccessorMemberCensus
	if err := json.Unmarshal([]byte(before[".successor-local-members.json"]), &oldCensus); err != nil || oldCensus.Pending == nil {
		t.Fatal("interruption lost original publication intent", err)
	}
	expected := *oldCensus.Pending
	if expected.StageInode == 0 && boundary != "reserved" || expected.StageInode != 0 && boundary == "reserved" {
		t.Fatal("fixture did not distinguish pre-stage from acknowledged-stage custody", boundary, expected.StageInode)
	}
	path, hash := localRebindTestRestore(t, f)
	var output, diagnostic bytes.Buffer
	args := append(append(append([]string{}, f.paths...), f.approvalArgs...), "--local-restore-plan", path, "--local-restore-plan-sha256", hash)
	if code := f.original.command(t.Context(), "contract-successor-execution-local-rebind-preview", &output, &diagnostic, args...); code != 0 {
		t.Fatal("public restored local review cannot retain its original terminal publication", boundary, code, diagnostic.String())
	}
	var preview struct {
		Plan         json.RawMessage `json:"plan"`
		SigningBytes string          `json:"signing_bytes"`
	}
	if err := json.Unmarshal(output.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	var plan struct {
		PendingOutcomeSha256 string `json:"pending_outcome_sha256"`
	}
	if err := json.Unmarshal(preview.Plan, &plan); err != nil || plan.PendingOutcomeSha256 != expected.Sha256 {
		t.Fatal("independent local review did not bind exact original terminal payload", err)
	}
	message, err := hex.DecodeString(strings.TrimPrefix(preview.SigningBytes, "0x"))
	if err != nil || !bytes.HasPrefix(message, []byte("urnetwork-mainnet-successor-local-rebind-v1\x00")) {
		t.Fatal("pending local approval lost its original separate domain", err)
	}
	approval := bootstrapRootTestWrite(t, filepath.Join(filepath.Dir(path), "synthetic-pending-local-rebind.json"), struct {
		Schema    string          `json:"schema"`
		Plan      json.RawMessage `json:"plan"`
		Signature string          `json:"signature_ed25519"`
	}{Schema: "urnetwork-mainnet-successor-local-rebind-envelope-v1", Plan: preview.Plan, Signature: hex.EncodeToString(ed25519.Sign(f.key, message))})
	extra := append(append([]string{}, f.approvalArgs...), "--local-rebind-approval", approval.Path, "--local-rebind-approval-sha256", approval.Sha256)
	for attempt := 0; attempt < 2; attempt++ {
		output.Reset()
		diagnostic.Reset()
		publicArgs := append(append([]string{}, f.paths...), extra...)
		if code := f.original.command(t.Context(), "contract-successor-execution-resume", &output, &diagnostic, publicArgs...); code != 0 {
			t.Fatal("public offline pending adoption cannot retain original outcome", attempt, code, diagnostic.String())
		}
		var result struct {
			CumulativeAttempts    uint16 `json:"cumulative_attempts"`
			PhysicalRebindPending bool   `json:"physical_rebind_pending"`
			LocalCustodyComplete  bool   `json:"local_custody_complete"`
			SubmissionAttempted   bool   `json:"submission_attempted"`
		}
		if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.CumulativeAttempts != 9 || !result.PhysicalRebindPending || result.LocalCustodyComplete || result.SubmissionAttempted {
			t.Fatal("pending local adoption reported false completion or renewed attempts", result, err)
		}
		if _, err := os.Lstat(filepath.Join(local, bootstrapSuccessorExecutionPrefix+"-local-rebind.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("pending original publication was displaced by a new receipt", err)
		}
		current := bootstrapSuccessorPreparationTestFiles(t, local)
		var retained bootstrapSuccessorMemberCensus
		if err := json.Unmarshal([]byte(current[".successor-local-members.json"]), &retained); err != nil || retained.Pending == nil || retained.Pending.Payload != expected.Payload || retained.Pending.Sha256 != expected.Sha256 || retained.Pending.Name != expected.Name || retained.Pending.Stage != expected.Stage {
			t.Fatal("pending restore lost original immutable outcome", err)
		}
		if !maps.Equal(nonces, bootstrapSuccessorPreparationTestFiles(t, registry)) {
			t.Fatal("pending restore changed original nonce claims")
		}
	}
	chain := f.original.contracts
	chain.stateLock.Lock()
	baseOverride, writes := chain.override, len(chain.writes)
	chain.override = func(method string, params []any, result any) any {
		result = baseOverride(method, params, result)
		if method == "eth_getTransactionByHash" || method == "eth_getTransactionReceipt" {
			return nil
		}
		return result
	}
	chain.stateLock.Unlock()
	online := append(append([]string{}, extra...), "--online", "--canonical-approval", f.canonicalRef.Path, "--canonical-approval-sha256", f.canonicalRef.Sha256)
	output.Reset()
	if code, diagnostic := f.invoke("contract-successor-execution-resume", &output, append(append([]string{}, online...), "--submit")...); code == 0 || !strings.Contains(diagnostic, "outcome is not canonically reconciled") {
		t.Fatal("unresolved restored outcome became another send", code, diagnostic)
	}
	chain.stateLock.Lock()
	actualWrites := len(chain.writes)
	chain.override = baseOverride
	chain.stateLock.Unlock()
	if actualWrites != writes {
		t.Fatal("absent canonical read renewed the original signed send")
	}
	output.Reset()
	if code, diagnostic := f.invoke("contract-successor-execution-resume", &output, online...); code != 0 {
		t.Fatal("exact original canonical outcome cannot finish local adoption", code, diagnostic)
	}
	var result struct {
		CumulativeAttempts    uint16 `json:"cumulative_attempts"`
		PhysicalRebindPending bool   `json:"physical_rebind_pending"`
		LocalCustodyComplete  bool   `json:"local_custody_complete"`
		InstallationComplete  bool   `json:"installation_complete"`
		SubmissionAttempted   bool   `json:"submission_attempted"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.CumulativeAttempts != 9 || result.PhysicalRebindPending || !result.LocalCustodyComplete || !result.InstallationComplete || result.SubmissionAttempted {
		t.Fatal("canonical pending outcome did not preserve original count and receipt", result, err)
	}
	completed := bootstrapSuccessorPreparationTestFiles(t, local)
	if completed[bootstrapSuccessorExecutionPrefix+"-local-rebind.json"] == "" {
		t.Fatal("canonical reconciliation omitted exact physical adoption receipt")
	}
	output.Reset()
	diagnostic.Reset()
	if code := f.original.command(t.Context(), "contract-successor-execution-resume", &output, &diagnostic, append(append([]string{}, f.paths...), extra...)...); code != 0 {
		t.Fatal("completed pending restore cannot reopen publicly", code, diagnostic.String())
	}
	if !maps.Equal(completed, bootstrapSuccessorPreparationTestFiles(t, local)) || !maps.Equal(nonces, bootstrapSuccessorPreparationTestFiles(t, registry)) {
		t.Fatal("completed pending restore reopened with new custody bytes")
	}
	chain.stateLock.Lock()
	actualWrites = len(chain.writes)
	chain.stateLock.Unlock()
	if actualWrites != writes {
		t.Fatal("canonical completion rebroadcast the original transaction")
	}
}

func TestBootstrapSuccessorLocalRebindRetainsReservedOriginalOutcome(t *testing.T) {
	localRebindPendingOutcomeControl(t, "reserved")
}

func TestBootstrapSuccessorLocalRebindRetainsAcknowledgedOriginalOutcome(t *testing.T) {
	localRebindPendingOutcomeControl(t, "name-synced")
}
