//go:build linux

// Public resource planning emits an unsigned successor from retained local
// custody. It neither loads a private key nor activates a producer.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/urfoundation/sn/internal/durablepath"
	"github.com/urfoundation/sn/validator"
)

// The request hash commits every forecast and physical root/fence bound.
func runValidatorCapacityPreview(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("validator-capacity-preview", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("request", "", "Reviewed exact original config, retained heads, finite roots and future resource forecasts")
	hash := flags.String("request-sha256", "", "Exact sha256: digest of the unsigned planning request")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *path == "" || !planSha256(*hash) {
		fmt.Fprintln(stderr, "capacity preview requires an exact request and hash")
		return 2
	}
	if err := durablepath.Require(ctx); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	raw, err := readPlanReference(ctx, *path, planFileReference{Path: *path, Sha256: *hash}, 2*1024*1024)
	if err != nil {
		fmt.Fprintln(stderr, "capacity request:", err)
		return 2
	}
	var request validator.ProductionCapacityRequest
	if err := decodePlanJson(raw, &request); err != nil {
		fmt.Fprintln(stderr, "capacity request:", err)
		return 2
	}
	preview, err := validator.BuildProductionCapacityPreview(ctx, request)
	if err != nil {
		return storageInspectionFailure(stderr, err)
	}
	raw, err = json.Marshal(preview)
	if err := errors.Join(err, ctx.Err()); err != nil {
		return storageInspectionFailure(stderr, err)
	}
	raw = append(raw, '\n')
	written, err := stdout.Write(raw)
	if err == nil && written != len(raw) {
		err = io.ErrShortWrite
	}
	if err != nil {
		fmt.Fprintln(stderr, "capacity preview output:", err)
		return 1
	}
	return 0
}
