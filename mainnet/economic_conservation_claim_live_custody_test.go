// Public Claim sampling keeps one fresh custody fence at dispatch and one at
// publication. Intervening owner loss cannot grant effects from a cached index.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Retire one full original window through the real proposal/signature/apply path.
func economicConservationClaimLiveCustodyFixture(t *testing.T) *economicConservationArchiveFixture {
	t.Helper()
	f := newEconomicConservationClaimWindowFixture(t, true, false)
	_, _, args := economicConservationClaimWindowTestPlan(t, f, economicConservationClaimWindowTestNext(t, f), monitorReadDigest([]byte("synthetic original live Claim custody review")))
	if code, issue := f.apply(t, args, &bytes.Buffer{}, monitorServiceHooks{}); code != 0 {
		t.Fatal("original Claim custody fixture did not apply", code, issue)
	}
	return f
}

// Matching bytes in a replacement inode never replace the original held owner.
func economicConservationClaimReplaceHeldArchive(t *testing.T, f *economicConservationArchiveFixture) {
	t.Helper()
	path := f.request.ArchivePath
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(filepath.Dir(path), "retained-"+filepath.Base(path))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

// A completed first sample does not authorize another read after owner loss.
func TestEconomicConservationClaimNextSampleChecksFreshOriginalCustody(t *testing.T) {
	f := economicConservationClaimLiveCustodyFixture(t)
	var appends, events int
	var retained []byte
	var output, diagnostic bytes.Buffer
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	hooks := monitorServiceHooks{
		beforeEconomicNativeAppend: func(context.Context, context.CancelFunc) { appends++ },
		afterEvent: func(context.Context, string) {
			events++
			if events == 1 {
				var err error
				retained, err = os.ReadFile(f.source.checkpoint)
				if err != nil || appends != 1 {
					t.Fatal("first real sample did not establish the dependent operation", appends, err)
				}
				economicConservationClaimReplaceHeldArchive(t, f)
			} else {
				cancel()
			}
		},
		wait: func(ctx context.Context, _ string, _ time.Duration) bool {
			f.source.now = f.source.now.Add(time.Second)
			return ctx.Err() == nil
		},
	}
	code := runMainWithMonitorHooks(ctx, append(f.source.args(t), "--follow"), &output, &diagnostic, func() time.Time { return f.source.now }, hooks)
	current, err := os.ReadFile(f.source.checkpoint)
	if code == 0 || events != 1 || appends != 1 || err != nil || !bytes.Equal(current, retained) || len(bytes.Split(bytes.TrimSpace(output.Bytes()), []byte{'\n'})) != 1 {
		t.Fatal("lost original custody reached another dependent sample", code, events, appends, err, diagnostic.String())
	}
}

// Loss or cancellation occurs after the successful sample fence and actual
// read join, before native append and the later checkpoint publication fence.
func TestEconomicConservationClaimPublicationKeepsOriginalAfterMidSampleFault(t *testing.T) {
	for _, fault := range []string{"owner-loss", "cancel"} {
		f := economicConservationClaimLiveCustodyFixture(t)
		original, err := os.ReadFile(f.source.checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(f.ctx)
		var appends int
		hooks := monitorServiceHooks{beforeEconomicNativeAppend: func(context.Context, context.CancelFunc) {
			appends++
			if fault == "owner-loss" {
				economicConservationClaimReplaceHeldArchive(t, f)
			} else {
				cancel()
			}
		}}
		var output, diagnostic bytes.Buffer
		code := runMainWithMonitorHooks(ctx, f.source.args(t), &output, &diagnostic, func() time.Time { return f.source.now }, hooks)
		cancel()
		current, readErr := os.ReadFile(f.source.checkpoint)
		if appends != 1 || output.Len() != 0 || readErr != nil || !bytes.Equal(current, original) || fault == "owner-loss" && code == 0 || fault == "cancel" && (code != 0 || strings.Contains(diagnostic.String(), "changed its original")) {
			t.Fatal("mid-sample fault published a dependent Claim checkpoint", fault, code, appends, readErr, diagnostic.String())
		}
	}
}
