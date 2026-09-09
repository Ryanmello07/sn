// Exact tiny carrier boundaries reproduce base64/escaped-metadata capacity
// errors without allocating production-size proof corpora.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Real content hashes and raw bytes remain independent of the sizing helper.
func finalCaptureCapacityEntry(path string, raw []byte) FinalCollectedFileBundleEntry {
	return FinalCollectedFileBundleEntry{Path: path, ContentHash: bytesSHA256(raw), SizeBytes: uint64(len(raw)), Data: raw}
}

// Json escaping, null and empty data have different exact wire lengths.
func TestFinalCaptureCapacityEntryMatchesActualJson(t *testing.T) {
	for _, entry := range []FinalCollectedFileBundleEntry{
		finalCaptureCapacityEntry("empty.bin", []byte{}),
		finalCaptureCapacityEntry("nil.bin", nil),
		finalCaptureCapacityEntry("\u0001<&>\".bin", []byte{0, 255, 3, 4}),
		finalCaptureCapacityEntry("unicode-世界.bin", bytes.Repeat([]byte{255}, 17)),
	} {
		encoded, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		size, err := finalCollectedEntryEncodedBytes(entry)
		if err != nil || size != uint64(len(encoded)) {
			t.Fatalf("%q exact size = %d, actual %d: %v", entry.Path, size, len(encoded), err)
		}
	}
}

// The public raw-object ceiling applies to encoded carriers, not source bytes.
func TestFinalCaptureCapacityExactCeilingAndOneOver(t *testing.T) {
	entry := finalCaptureCapacityEntry("<escaped>.bin", []byte{1, 2, 3, 4})
	encoded, err := json.Marshal(FinalCollectedFileBundle{Schema: finalCollectedFileBundleSchema, Name: "closed<&>", Files: []FinalCollectedFileBundleEntry{entry}})
	if err != nil {
		t.Fatal(err)
	}
	ranges, err := finalCollectedBundleChunkRanges(t.Context(), "closed<&>", []FinalCollectedFileBundleEntry{entry}, uint64(len(encoded)))
	if err != nil || len(ranges) != 1 || ranges[0].first != 0 || ranges[0].end != 1 {
		t.Fatalf("exact encoded ceiling refused: %+v %v", ranges, err)
	}
	if _, err := finalCollectedBundleChunkRanges(t.Context(), "closed<&>", []FinalCollectedFileBundleEntry{entry}, uint64(len(encoded)-1)); err == nil {
		t.Fatal("one byte beyond encoded ceiling was accepted")
	}
}

// Partitioning retains the whole strictly ordered census exactly once.
func TestFinalCaptureCapacityChunksRetainExactCensus(t *testing.T) {
	var entries []FinalCollectedFileBundleEntry
	for index := 0; index < 17; index++ {
		entries = append(entries, finalCaptureCapacityEntry(fmt.Sprintf("proof-%03d.bin", index), bytes.Repeat([]byte{byte(index)}, index+3)))
	}
	const maximum = 700
	ranges, err := finalCollectedBundleChunkRanges(t.Context(), "proofs<&>", entries, maximum)
	if err != nil || len(ranges) < 2 {
		t.Fatalf("bounded partition: %+v %v", ranges, err)
	}
	next := 0
	for index, part := range ranges {
		if part.first != next || part.end <= part.first {
			t.Fatalf("source gap/repetition: %+v", ranges)
		}
		bundle := FinalCollectedFileBundle{Schema: finalCollectedFileBundleSchema, Name: fmt.Sprintf("proofs<&>-%03d-of-%03d", index+1, len(ranges)), Files: entries[part.first:part.end]}
		encoded, err := json.Marshal(bundle)
		if err != nil || len(encoded) > maximum {
			t.Fatalf("actual encoded chunk exceeds ceiling: %d %v", len(encoded), err)
		}
		if err := verifyFinalCollectedFileBundle(&bundle); err != nil {
			t.Fatal(err)
		}
		next = part.end
	}
	if next != len(entries) {
		t.Fatal("last source omitted")
	}
}

// A duplicate at a chunk boundary is not an independently valid chunk census.
func TestFinalCaptureCapacityRejectsCrossChunkDuplicates(t *testing.T) {
	entry := finalCaptureCapacityEntry("proof.bin", []byte{1})
	if _, err := finalCollectedBundleChunkRanges(t.Context(), "proofs", []FinalCollectedFileBundleEntry{entry, entry}, 400); err == nil {
		t.Fatal("duplicate source hidden by partitioning")
	}
	entry.SizeBytes++
	if _, err := finalCollectedBundleChunkRanges(t.Context(), "proofs", []FinalCollectedFileBundleEntry{entry}, 400); err == nil {
		t.Fatal("incorrect source byte count accepted")
	}
}

// Arithmetic is proven near uint64's ceiling using lengths, not allocations.
func TestFinalCaptureCapacityBase64Overflow(t *testing.T) {
	for _, size := range []uint64{0, 1, 2, 3, 4, 1000, math.MaxUint64 / 4 * 3} {
		encoded, err := finalCollectedBase64Bytes(size)
		if err != nil || encoded != ((size+2)/3)*4 {
			t.Fatalf("bounded base64 length %d: %d %v", size, encoded, err)
		}
	}
	for _, size := range []uint64{math.MaxUint64, math.MaxUint64 - 1, math.MaxUint64/4*3 + 1} {
		if _, err := finalCollectedBase64Bytes(size); err == nil {
			t.Fatalf("overflow length %d accepted", size)
		}
	}
}

// Cancellation has no output artifact; the same exact source remains retryable.
func TestFinalCaptureCapacityCancelledBeforePersistence(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	entries := []FinalCollectedFileBundleEntry{finalCaptureCapacityEntry("proof.bin", []byte{1, 2, 3})}
	if _, err := persistFinalCollectedBundleChunksContext(ctx, root, "proofs", entries); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled persistence: %v", err)
	}
	names, err := os.ReadDir(root)
	if err != nil || len(names) != 0 {
		t.Fatalf("cancelled owner wrote output: %v %v", names, err)
	}
	locators, err := persistFinalCollectedBundleChunksContext(t.Context(), root, "proofs", entries)
	if err != nil || len(locators) != 1 {
		t.Fatalf("actual retry: %+v %v", locators, err)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(locators[0].URI)))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := decodeFinalCollectedFileBundle(raw)
	if err != nil || len(bundle.Files) != 1 || !bytes.Equal(bundle.Files[0].Data, entries[0].Data) {
		t.Fatalf("persisted source differs: %+v %v", bundle, err)
	}
}
