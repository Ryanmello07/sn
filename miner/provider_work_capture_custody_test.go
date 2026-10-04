//go:build linux || darwin

// Real provider construction must reject incomplete prepared custody before
// allocating its SDK worker tree or publishing any replacement outbox state.
package miner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/urnetwork/connect"
	"github.com/urnetwork/sdk"
	"golang.org/x/sys/unix"
)

// A valid reviewed profile and retained key cannot authorize an empty rebirth.
// The nil downstream owners prove the constructor refuses at custody admission.
func TestProviderWholeWorkActualConstructorRefusesUnpreparedCustody(t *testing.T) {
	profile, seed := providerWorkCaptureFixture(t)
	path, digest := writeProviderWorkCaptureFixture(t, profile)
	loaded, err := ReadProviderWorkCaptureProfile(t.Context(), path, digest, true)
	if err != nil {
		t.Fatal(err)
	}
	directory := profile.Providers[0].OutboxDirectory
	for _, indexed := range []bool{false, true} {
		if indexed {
			file, err := os.OpenFile(filepath.Join(directory, connect.OriginalWorkOutboxIndexName), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		}
		settings := ProviderDeviceSettings([32]byte{})
		settings.KeyMaterial = sdk.NewDeviceLocalKeyMaterial(seed, nil, nil)
		device, err := newProviderDeviceLocal(t.Context(), nil, nil, "", "synthetic incomplete outbox", settings, loaded, "direct", connect.Id(profile.Providers[0].ClientId))
		if device != nil || !errors.Is(err, connect.ErrOriginalWorkOutboxIdentity) {
			t.Fatal("actual constructor bypassed prepared original custody", indexed, device, err)
		}
		entries, err := os.ReadDir(directory)
		if err != nil || !indexed && len(entries) != 0 || indexed && (len(entries) != 1 || entries[0].Name() != connect.OriginalWorkOutboxIndexName) {
			t.Fatal("failed launch created or replaced original custody", indexed, entries, err)
		}
		if _, err := unix.Getxattr(directory, connect.OriginalWorkOutboxAttribute, make([]byte, 4096)); err == nil {
			t.Fatal("failed launch minted an empty birth checkpoint", indexed)
		}
	}
}

// Cancellation belongs to the actual constructor and precedes any outbox read,
// transport or SDK construction. The original directory remains untouched.
func TestProviderWholeWorkActualConstructorPreservesCanceledCustody(t *testing.T) {
	profile, seed := providerWorkCaptureFixture(t)
	settings := ProviderDeviceSettings([32]byte{})
	settings.KeyMaterial = sdk.NewDeviceLocalKeyMaterial(seed, nil, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	device, err := newProviderDeviceLocal(ctx, nil, nil, "", "synthetic canceled outbox", settings, &profile, "direct", connect.Id(profile.Providers[0].ClientId))
	if device != nil || !errors.Is(err, context.Canceled) || settings.ContractManagerSettings.OriginalWorkCapture != nil {
		t.Fatal("canceled actual constructor changed capture ownership", device, err)
	}
	entries, err := os.ReadDir(profile.Providers[0].OutboxDirectory)
	if err != nil || len(entries) != 0 {
		t.Fatal("canceled constructor created original custody", entries, err)
	}
}
