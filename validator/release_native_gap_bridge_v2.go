//go:build linux || darwin

package validator

// A native gap bridge lets a production validator resume after it missed
// native epochs without changing its signed config. The operator pins the
// exact bridge bytes by SHA-256 on the command line. It admits one forward
// edge only: from the retained applied intent at last_native_epoch whose
// measurement artifact is last_artifact_hash, to the first intent at native
// epoch first_native_epoch or later. Missed epochs have no intent, measurement
// or EMA observation; the head EMA folds the next actual input once. Every
// signature, cut, terminal and head-lineage check still applies to that edge.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const ReleaseNativeGapBridgeSchema = "urnetwork-validator-native-gap-bridge-v1"
const ReleaseNativeGapBridgeMaximumBytes = 4096

// The canonical wire form is json.MarshalIndent with a two-space indent, in
// this field order, followed by one newline. Pins cover those exact bytes.
type ReleaseNativeGapBridge struct {
	Schema           string `json:"schema"`
	ChainID          uint64 `json:"chain_id"`
	DeploymentID     string `json:"deployment_id"`
	ValidatorID      uint64 `json:"validator_id"`
	LastNativeEpoch  uint64 `json:"last_native_epoch"`
	LastArtifactHash string `json:"last_artifact_hash"`
	FirstNativeEpoch uint64 `json:"first_native_epoch"`
}

// Only startup replay creates this owner, after it finds the bridge's applied
// predecessor in the retained intents. Its edge checks are stateless: once
// history moves past the predecessor, no later edge can satisfy them.
type releaseNativeGapBridgeV2 struct {
	bridge  ReleaseNativeGapBridge
	crossed bool
}

// The path is read once from a bounded regular file that is not a symlink.
func ReadReleaseNativeGapBridge(path, expectedSHA256 string) (result *ReleaseNativeGapBridge, resultErr error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("native gap bridge path must be canonical and absolute")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("native gap bridge: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
		if resultErr != nil {
			result = nil
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("native gap bridge: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > ReleaseNativeGapBridgeMaximumBytes {
		return nil, errors.New("native gap bridge is not a bounded regular file")
	}
	encoded, err := io.ReadAll(io.LimitReader(file, ReleaseNativeGapBridgeMaximumBytes+1))
	if err != nil {
		return nil, fmt.Errorf("native gap bridge: %w", err)
	}
	return DecodeReleaseNativeGapBridge(encoded, expectedSHA256)
}

// Decoding checks the pin and the canonical wire. It grants nothing until the
// config owner and then the retained intent history match it.
func DecodeReleaseNativeGapBridge(encoded []byte, expectedSHA256 string) (*ReleaseNativeGapBridge, error) {
	if _, err := parseReleaseContentHash(expectedSHA256); err != nil {
		return nil, fmt.Errorf("native gap bridge sha256 pin: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > ReleaseNativeGapBridgeMaximumBytes || ReleaseMeasurementContentHash(encoded) != expectedSHA256 {
		return nil, errors.New("native gap bridge bytes differ from the pinned sha256")
	}
	var bridge ReleaseNativeGapBridge
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bridge); err != nil {
		return nil, fmt.Errorf("native gap bridge: %w", err)
	}
	canonical, err := json.MarshalIndent(&bridge, "", "  ")
	if err != nil || !bytes.Equal(encoded, append(canonical, '\n')) {
		return nil, errors.Join(errors.New("native gap bridge is not canonical JSON: two-space indent, schema field order, one trailing newline"), err)
	}
	if bridge.Schema != ReleaseNativeGapBridgeSchema || bridge.ChainID == 0 || strings.TrimSpace(bridge.DeploymentID) == "" || bridge.ValidatorID == 0 || bridge.LastNativeEpoch == 0 {
		return nil, errors.New("native gap bridge schema, chain, deployment, validator or last native epoch is absent")
	}
	if _, err := parseReleaseContentHash(bridge.LastArtifactHash); err != nil {
		return nil, fmt.Errorf("native gap bridge last_artifact_hash: %w", err)
	}
	if bridge.LastNativeEpoch >= ^uint64(0)-1 || bridge.FirstNativeEpoch <= bridge.LastNativeEpoch+1 || bridge.FirstNativeEpoch == ^uint64(0) {
		return nil, errors.New("native gap bridge first_native_epoch must be at least last_native_epoch + 2")
	}
	return &bridge, nil
}

// The bridge names its owner; the signed config, its hash and its approval
// are not changed. Other gap authorities cannot be combined with it.
func (self *ReleaseNativeGapBridge) configure(cfg *ReleaseConfig) error {
	if self == nil || cfg == nil {
		return errors.New("native gap bridge or its config is absent")
	}
	if self.ChainID != cfg.ChainID || self.DeploymentID != cfg.DeploymentID || self.ValidatorID != cfg.ValidatorID {
		return fmt.Errorf("native gap bridge names chain %d, deployment %q, validator %d; the config has chain %d, deployment %q, validator %d",
			self.ChainID, self.DeploymentID, self.ValidatorID, cfg.ChainID, cfg.DeploymentID, cfg.ValidatorID)
	}
	if cfg.ProvisionalDeferClosedNativeInput || cfg.ProvisionalRuntimeCompatibility != "" || cfg.historyAdoptionV2 != nil || cfg.nativeGapBridge != nil {
		return errors.New("native gap bridge cannot combine with provisional or adopted history gaps")
	}
	owned := *self
	cfg.nativeGapBridge = &owned
	return nil
}

// Startup binds the bridge to the retained intents before any worker starts.
// The applied predecessor is either the current intent, so the edge is still
// pending, or is followed directly by the bridged intent at first_native_epoch
// or later. Any other history refuses startup while the bridge is supplied.
func (self *ReleaseNativeGapBridge) matchHistory(ctx context.Context, all []*SteeringIntent) (*releaseNativeGapBridgeV2, error) {
	if self == nil {
		return nil, nil
	}
	if ctx == nil {
		return nil, errors.New("native gap bridge history owner is incomplete")
	}
	found := -1
	for index, intent := range all {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if intent != nil && intent.SubnetEpoch == self.LastNativeEpoch && intent.MeasurementArtifactHash == self.LastArtifactHash {
			if found >= 0 {
				return nil, errors.New("native gap bridge predecessor repeats in the retained intent history")
			}
			found = index
		}
	}
	if found < 0 {
		return nil, fmt.Errorf("native gap bridge predecessor (native epoch %d, %s) is not in the retained intent history", self.LastNativeEpoch, self.LastArtifactHash)
	}
	if all[found].Status != "applied" {
		return nil, fmt.Errorf("native gap bridge predecessor at native epoch %d is %q, not applied", self.LastNativeEpoch, all[found].Status)
	}
	owner := &releaseNativeGapBridgeV2{bridge: *self}
	if found+1 < len(all) {
		if next := all[found+1]; next == nil || next.SubnetEpoch < self.FirstNativeEpoch {
			return nil, fmt.Errorf("native gap bridge predecessor is not followed by an intent at native epoch %d or later", self.FirstNativeEpoch)
		}
		owner.crossed = true
	}
	return owner, ctx.Err()
}

// The one admitted intent edge. Retries, consecutive successors and any later
// gap use the ordinary successor rules.
func (self *releaseNativeGapBridgeV2) allowsIntentEdge(previous, current *SteeringIntent) bool {
	return self != nil && previous != nil && current != nil && previous.Status == "applied" &&
		previous.SubnetEpoch == self.bridge.LastNativeEpoch && previous.MeasurementArtifactHash == self.bridge.LastArtifactHash &&
		current.SubnetEpoch >= self.bridge.FirstNativeEpoch
}

// The same edge between the signed artifacts. The caller still authenticates
// both endpoints, their cuts, terminals and head EMA lineage.
func (self *releaseNativeGapBridgeV2) allowsArtifactEdge(previousHash string, previous, current *ReleaseMeasurementArtifact) bool {
	return self != nil && previous != nil && current != nil && previousHash == self.bridge.LastArtifactHash &&
		current.PreviousArtifactHash == previousHash && previous.SubnetEpoch == self.bridge.LastNativeEpoch &&
		current.SubnetEpoch >= self.bridge.FirstNativeEpoch
}

// Startup admits this owner only while the store's state at last_native_epoch
// belongs to the matched predecessor; committing the bridged epoch consumes it.
func (self *releaseNativeGapBridgeV2) allowsHeadEMAEdge(last, epoch uint64) bool {
	return self != nil && last == self.bridge.LastNativeEpoch && epoch >= self.bridge.FirstNativeEpoch
}

// Before first_native_epoch the bridged predecessor stays the final decision:
// release idles with the ordinary already-final result instead of failing.
func (self *releaseNativeGapBridgeV2) requireFirstEpoch(current *SteeringIntent, epoch uint64) error {
	if self == nil || current == nil || current.SubnetEpoch != self.bridge.LastNativeEpoch || current.MeasurementArtifactHash != self.bridge.LastArtifactHash {
		return nil
	}
	if epoch < self.bridge.FirstNativeEpoch {
		return ErrSteeringAlreadyFinal
	}
	return nil
}
