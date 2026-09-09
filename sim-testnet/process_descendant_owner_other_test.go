//go:build !linux

// Retain the existing non-Linux descendant assertion without Linux prctl.
package main

import "testing"

func runSupervisorDescendantOwnerTest(t *testing.T, run func(*testing.T)) {
	t.Helper()
	run(t)
}
