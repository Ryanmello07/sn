//go:build linux || darwin

// The runtime and proof publishers consume one approved origin census.
// Bind addresses remain local; artifact and client APIs use this public route.
package main

import (
	"errors"
	"slices"
)

// Recheck resolved input before either rendering entry point can write.
// Normalization belongs to config loading, not a later unreviewed rewrite.
func validateRuntimeOperatorApiOrigins(cfg *ResolvedConfig) error {
	if cfg == nil || cfg.Config == nil || cfg.Public == nil || cfg.Config.Topology.Operators < 1 {
		return errors.New("runtime operator API origin context is incomplete")
	}
	origins, err := validateOperatorAPIOrigins(cfg.OperatorAPIOrigins, cfg.Config.Topology.Operators, cfg.ChainID, cfg.Public.Chain.GenesisHash, cfg.Config.Deployment.Network)
	if err != nil || !slices.Equal(origins, cfg.OperatorAPIOrigins) {
		return errors.Join(errors.New("runtime operator API origins differ from the approved canonical census"), err)
	}
	return nil
}
