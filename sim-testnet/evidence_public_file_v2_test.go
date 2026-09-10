// Real public entrypoints retain both replica reads while exact producer
// framing avoids repeated general Json work. Independent legacy oracles
// preserve historical wire, signature, source and failure behavior.
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Each fixture owns its signing key, mutable transport and all response
// buffers. Closing a response may destroy the transport's borrowed bytes.
type publicCampaignFileFixtureV2 struct {
	cfg        *ResolvedConfig
	key        *ecdsa.PrivateKey
	payload    campaignEvidenceFilePayload
	entry      campaignEvidenceFileEntry
	envelope   *ReleaseEvidenceEnvelope
	wire       []byte
	secondWire []byte
	public     *PublicDeploymentManifest
	probe      *liveScenarioProbe
	requests   int
	closes     int
	borrowed   []byte
	closeAt    int
	closeErr   error
	onClose    func(int)
}

// The old whole-envelope encoder supplies both the unsigned digest and wire.
// No production canonical decoder or digest signs this compatibility oracle.
func publicCampaignFileSignTestV2(t *testing.T, key *ecdsa.PrivateKey, envelope *ReleaseEvidenceEnvelope) []byte {
	t.Helper()
	envelope.ContentHash, envelope.Signature = "", ""
	unsigned, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(unsigned)
	signature, err := crypto.Sign(digest[:], key)
	if err != nil {
		t.Fatal(err)
	}
	envelope.ContentHash = "sha256:" + hex.EncodeToString(digest[:])
	envelope.Signature = "0x" + hex.EncodeToString(signature)
	wire, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

// Source bytes are deterministic, but every signature uses a fresh test key.
func newPublicCampaignFileFixtureV2(t *testing.T, size int) *publicCampaignFileFixtureV2 {
	t.Helper()
	cfg := campaignMetadataConfigTestV2(t)
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, size)
	for index := range raw {
		raw[index] = byte(index % 251)
	}
	fixture := &publicCampaignFileFixtureV2{cfg: cfg, key: key}
	fixture.payload = campaignEvidenceFilePayload{Schema: campaignEvidenceFileSchema, RunID: "synthetic-public-file", Scope: "run", Path: "population/synthetic-source.bin", ContentHash: bytesSHA256(raw), Size: uint64(len(raw)), Data: raw}
	encoded, err := json.Marshal(fixture.payload)
	if err != nil {
		t.Fatal(err)
	}
	fixture.envelope, fixture.wire = priorCarrierDecodeSignedWireTestV2(t, cfg, key, fixture.payload.RunID, encoded)
	fixture.entry = campaignEvidenceFileEntry{Path: fixture.payload.Path, ContentHash: fixture.payload.ContentHash, Size: fixture.payload.Size, EnvelopeHash: fixture.envelope.ContentHash}
	fixture.public = &PublicDeploymentManifest{
		DeploymentID: cfg.Config.Deployment.DeploymentID, ChainID: cfg.ChainID, GenesisHash: cfg.Public.Chain.GenesisHash, Netuid: cfg.Netuid,
		Operators: []PublicOperator{{NoID: 1, APIURL: "https://no1.example"}, {NoID: 2, APIURL: "https://no2.example"}},
	}
	fixture.probe = &liveScenarioProbe{cfg: cfg, client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		fixture.requests++
		operator := (fixture.requests-1)%2 + 1
		if request.URL.Host != fmt.Sprintf("no%d.example", operator) || request.URL.Query().Get("hash") != fixture.entry.EnvelopeHash {
			return nil, errors.New("synthetic public file request escaped its exact route")
		}
		wire := fixture.wire
		if operator == 2 && fixture.secondWire != nil {
			wire = fixture.secondWire
		}
		fixture.borrowed = append(fixture.borrowed[:0], wire...)
		requestIndex := fixture.requests
		body := &campaignReadbackBodyFailureV2{
			ReadCloser: io.NopCloser(bytes.NewReader(fixture.borrowed)),
			onClose: func() {
				clear(fixture.borrowed)
				fixture.closes++
				if fixture.onClose != nil {
					fixture.onClose(requestIndex)
				}
			},
		}
		if requestIndex == fixture.closeAt {
			body.closeErr = fixture.closeErr
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: request, Body: body}, nil
	})}}
	return fixture
}

// This is the pre-fix decoder and original whole-envelope digest, not the
// new canonical helper. Its returned bytes are compared at the public caller.
func publicCampaignFileLegacyReadTestV2(fixture *publicCampaignFileFixtureV2, wire []byte) ([]byte, error) {
	var envelope ReleaseEvidenceEnvelope
	if err := decodeStrictJSONBytes(wire, &envelope); err != nil {
		return nil, err
	}
	if err := validateReleaseEvidenceIdentity(&envelope); err != nil {
		return nil, err
	}
	unsigned, err := evidenceUnsignedBytes(&envelope)
	if err != nil {
		return nil, err
	}
	if err := verifyEvidenceSignature(&envelope, sha256.Sum256(unsigned), nil); err != nil {
		return nil, err
	}
	public := fixture.public
	if envelope.ContentHash != fixture.entry.EnvelopeHash || envelope.Kind != campaignEvidenceFileKind || envelope.RunID != fixture.payload.RunID || envelope.DeploymentID != public.DeploymentID || envelope.ChainID != public.ChainID || envelope.Netuid != public.Netuid || !strings.EqualFold(envelope.GenesisHash, public.GenesisHash) || envelope.Signer != fixture.envelope.Signer {
		return nil, errors.New("legacy public signed identity differs")
	}
	var payload campaignEvidenceFilePayload
	if err := decodeStrictJSONBytes(envelope.Payload, &payload); err != nil {
		return nil, err
	}
	if payload.Schema != campaignEvidenceFileSchema || payload.RunID != fixture.payload.RunID || payload.Scope != fixture.payload.Scope || payload.Path != fixture.entry.Path || payload.Size != fixture.entry.Size || payload.ContentHash != fixture.entry.ContentHash || uint64(len(payload.Data)) != fixture.entry.Size || bytesSHA256(payload.Data) != fixture.entry.ContentHash {
		return nil, errors.New("legacy public exact source differs")
	}
	limits, err := campaignEvidenceLimitsForConfig(fixture.cfg)
	if err != nil {
		return nil, err
	}
	if err := validateCampaignMetadataRawV2(limits, fixture.entry.Path, payload.Data); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

// A canonical source is authenticated once in each call; both origin bodies
// still close. Returned raw bytes cannot mutate any subsequent read's proof.
func TestCampaignEvidenceReadbackV2CanonicalFilesKeepFreshOwnedBytes(t *testing.T) {
	t.Parallel()
	for _, size := range []int{0, 1, 2, 3, 48*1024 - 1, 48 * 1024, 48*1024 + 1, 64*1024 + 1} {
		fixture := newPublicCampaignFileFixtureV2(t, size)
		before := bytes.Clone(fixture.wire)
		work := 0
		raw, err := fixture.probe.readPublicCampaignSourceWithObserverV2(t.Context(), fixture.public, fixture.payload.RunID, fixture.envelope.Signer.Hex(), fixture.payload.Scope, fixture.entry, func(canonical bool) {
			work++
			if !canonical {
				t.Error("canonical public file repeated general Json decoding")
			}
		})
		if err != nil || !bytes.Equal(raw, fixture.payload.Data) || work != 1 || fixture.requests != 2 || fixture.closes != 2 {
			t.Fatalf("canonical public owner size=%d work=%d reads=%d closes=%d: %v", size, work, fixture.requests, fixture.closes, err)
		}
		clear(raw)
		fresh, err := fixture.probe.readPublicCampaignSourceV2(t.Context(), fixture.public, fixture.payload.RunID, fixture.envelope.Signer.Hex(), fixture.payload.Scope, fixture.entry)
		legacy, legacyErr := publicCampaignFileLegacyReadTestV2(fixture, fixture.wire)
		if err != nil || legacyErr != nil || !bytes.Equal(fresh, fixture.payload.Data) || !bytes.Equal(fresh, legacy) || !bytes.Equal(before, fixture.wire) || fixture.requests != 4 || fixture.closes != 4 {
			t.Fatalf("default public owner borrowed prior raw/transport bytes size=%d: %v %v", size, err, legacyErr)
		}
	}
}

// Valid historical inner/outer spellings keep their original exact admission;
// byte-different origins still cannot borrow one another's signed identity.
func TestCampaignEvidenceReadbackV2CanonicalFilesPreserveLegacySpellings(t *testing.T) {
	t.Parallel()
	fixture := newPublicCampaignFileFixtureV2(t, 1)
	originalPayload := bytes.Clone(fixture.envelope.Payload)
	dataStart := bytes.Index(originalPayload, []byte(",\"data\":"))
	if dataStart < 0 {
		t.Fatal("source fixture has no data field")
	}
	cases := []struct {
		name    string
		payload []byte
		outer   string
	}{
		{name: "escaped-data", payload: bytes.Replace(originalPayload, []byte("\"AA==\""), []byte("\"\\u0041A==\""), 1)},
		{name: "escaped-linefeed", payload: bytes.Replace(originalPayload, []byte("\"AA==\""), []byte("\"AA\\u000a==\""), 1)},
		{name: "array-data", payload: bytes.Replace(originalPayload, []byte("\"AA==\""), []byte("[0]"), 1)},
		{name: "case-data", payload: bytes.Replace(originalPayload, []byte("\"data\""), []byte("\"DATA\""), 1)},
		{name: "duplicate-schema", payload: bytes.Replace(originalPayload, []byte("\"schema\":"), []byte("\"schema\":\"discarded\",\"schema\":"), 1)},
		{name: "reordered-payload", payload: []byte("{\"data\":\"AA==\"," + string(originalPayload[1:dataStart]) + "}")},
		{name: "non-strict-padding-bits", payload: bytes.Replace(originalPayload, []byte("\"AA==\""), []byte("\"AB==\""), 1)},
		{name: "outer-whitespace", payload: originalPayload, outer: "whitespace"},
		{name: "outer-field-order", payload: originalPayload, outer: "reorder"},
		{name: "outer-duplicate-field", payload: originalPayload, outer: "duplicate"},
	}
	for _, item := range cases {
		envelope := *fixture.envelope
		envelope.Payload = bytes.Clone(item.payload)
		wire := publicCampaignFileSignTestV2(t, fixture.key, &envelope)
		switch item.outer {
		case "whitespace":
			wire = append(append([]byte(" \n"), wire...), '\n')
		case "reorder":
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(wire, &fields); err != nil {
				t.Fatal(err)
			}
			var err error
			wire, err = json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
		case "duplicate":
			wire = bytes.Replace(wire, []byte("\"kind\":"), []byte("\"kind\":\"discarded\",\"kind\":"), 1)
		}
		fixture.envelope, fixture.wire, fixture.entry.EnvelopeHash = &envelope, wire, envelope.ContentHash
		fixture.requests, fixture.closes = 0, 0
		legacy, legacyErr := publicCampaignFileLegacyReadTestV2(fixture, wire)
		work := 0
		raw, err := fixture.probe.readPublicCampaignSourceWithObserverV2(t.Context(), fixture.public, fixture.payload.RunID, envelope.Signer.Hex(), fixture.payload.Scope, fixture.entry, func(canonical bool) {
			work++
			if canonical != (item.name == "non-strict-padding-bits") {
				t.Errorf("%s borrowed the wrong structural owner: canonical=%t", item.name, canonical)
			}
		})
		if err != nil || legacyErr != nil || !bytes.Equal(raw, legacy) || !bytes.Equal(raw, fixture.payload.Data) || work != 1 || fixture.requests != 2 || fixture.closes != 2 {
			t.Fatalf("%s historical public parity: %v %v work=%d reads=%d closes=%d", item.name, err, legacyErr, work, fixture.requests, fixture.closes)
		}
		fixture.secondWire = append(bytes.Clone(wire), ' ')
		fixture.requests, fixture.closes = 0, 0
		if raw, err := fixture.probe.readPublicCampaignSourceV2(t.Context(), fixture.public, fixture.payload.RunID, envelope.Signer.Hex(), fixture.payload.Scope, fixture.entry); raw != nil || err == nil || fixture.requests != 2 || fixture.closes != 2 {
			t.Fatalf("%s different original replicas accepted: %v reads=%d", item.name, err, fixture.requests)
		}
		fixture.secondWire = nil
	}
}

// Correctly re-signed wrong metadata still fails its original typed consumer;
// unsigned mutations and independently requested routes never reuse trust.
func TestCampaignEvidenceReadbackV2CanonicalFilesRejectFreshAuthorityMutations(t *testing.T) {
	t.Parallel()
	fixture := newPublicCampaignFileFixtureV2(t, 3)
	originalEnvelope, originalWire, originalEntry := *fixture.envelope, bytes.Clone(fixture.wire), fixture.entry
	for _, fault := range []string{"signature", "unsigned-data", "signed-data", "schema", "run", "scope", "path", "raw-hash", "raw-size", "null-data", "unknown-payload", "unknown-envelope", "trailing-value", "truncated", "kind", "deployment", "chain", "netuid", "genesis", "requested-hash", "requested-signer", "raw-bound"} {
		envelope, entry := originalEnvelope, originalEntry
		payload := fixture.payload
		payload.Data = bytes.Clone(fixture.payload.Data)
		signer := originalEnvelope.Signer.Hex()
		resign := true
		switch fault {
		case "signature":
			resign = false
			envelope.Signature = "0x" + strings.Repeat("00", crypto.SignatureLength)
		case "unsigned-data":
			resign = false
			payload.Data[0]++
		case "signed-data":
			payload.Data[0]++
		case "schema":
			payload.Schema = "synthetic-wrong-schema"
		case "run":
			payload.RunID += "-foreign"
		case "scope":
			payload.Scope = "reference"
		case "path":
			payload.Path = "population/foreign-source.bin"
		case "raw-hash":
			payload.ContentHash = "sha256:" + strings.Repeat("ab", sha256.Size)
		case "raw-size":
			payload.Size++
		case "null-data":
			payload.Data = nil
		case "kind":
			envelope.Kind = campaignEvidenceManifestKind
		case "deployment":
			envelope.DeploymentID += "-foreign"
		case "chain":
			envelope.ChainID++
		case "netuid":
			envelope.Netuid++
		case "genesis":
			envelope.GenesisHash = "0x" + strings.Repeat("ab", sha256.Size)
		case "requested-hash":
			entry.EnvelopeHash = "sha256:" + strings.Repeat("ab", sha256.Size)
		case "requested-signer":
			signer = "0x" + strings.Repeat("ab", 20)
		case "raw-bound":
			entry.Size = maximumCampaignEvidenceRawFileBytes + 1
		}
		var err error
		envelope.Payload, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if fault == "unknown-payload" {
			envelope.Payload = append([]byte("{\"synthetic_unknown\":true,"), envelope.Payload[1:]...)
		}
		wire := []byte(nil)
		if resign {
			wire = publicCampaignFileSignTestV2(t, fixture.key, &envelope)
			if fault != "requested-hash" {
				entry.EnvelopeHash = envelope.ContentHash
			}
		} else {
			wire, err = json.Marshal(&envelope)
			if err != nil {
				t.Fatal(err)
			}
		}
		switch fault {
		case "unknown-envelope":
			wire = append([]byte("{\"synthetic_unknown\":true,"), wire[1:]...)
		case "trailing-value":
			wire = append(wire, []byte(" null")...)
		case "truncated":
			wire = wire[:len(wire)-1]
		}
		fixture.envelope, fixture.entry, fixture.wire = &envelope, entry, wire
		fixture.requests, fixture.closes = 0, 0
		raw, err := fixture.probe.readPublicCampaignSourceV2(t.Context(), fixture.public, fixture.payload.RunID, signer, fixture.payload.Scope, entry)
		wantRequests := 1
		if fault == "raw-bound" {
			wantRequests = 0
		}
		if raw != nil || err == nil || fixture.requests != wantRequests || fixture.closes != wantRequests {
			t.Fatalf("%s authority mutation accepted: %v reads=%d closes=%d", fault, err, fixture.requests, fixture.closes)
		}
		fixture.envelope, fixture.entry, fixture.wire = &originalEnvelope, originalEntry, originalWire
		fixture.requests, fixture.closes = 0, 0
		raw, err = fixture.probe.readPublicCampaignSourceV2(t.Context(), fixture.public, fixture.payload.RunID, originalEnvelope.Signer.Hex(), fixture.payload.Scope, originalEntry)
		if err != nil || !bytes.Equal(raw, fixture.payload.Data) || fixture.requests != 2 || fixture.closes != 2 {
			t.Fatalf("%s poisoned fresh original authentication: %v", fault, err)
		}
	}
}

// Exact count preflight protects destination bounds even for hostile padding.
// Whole-string legacy Decode is the independent oracle at former chunk edges.
func TestCampaignEvidenceReadbackV2CanonicalFilesRetainBase64Grammar(t *testing.T) {
	t.Parallel()
	fixture := newPublicCampaignFileFixtureV2(t, 1)
	limits, err := campaignEvidenceLimitsForConfig(fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		encoded string
		size    int
	}{
		{name: "excess-padding", encoded: "A===", size: 1},
		{name: "missing-padding", encoded: "AAA", size: 2},
		{name: "data-after-padding", encoded: "AA==AAAA", size: 4},
		{name: "wrong-alphabet", encoded: "AA?=", size: 2},
		{name: "raw-linefeed", encoded: "AA\n==", size: 1},
		{name: "raw-carriage-return", encoded: "AA\r==", size: 1},
		{name: "raw-quote", encoded: "AA\"=", size: 1},
		{name: "raw-backslash", encoded: "AA\\=", size: 1},
		{name: "invalid-utf8", encoded: "AA\xff=", size: 1},
		{name: "decoded-one-over", encoded: "AAAA", size: 2},
		{name: "decoded-one-under", encoded: "AA==", size: 2},
	}
	for _, boundary := range []int{1024, 64 * 1024} {
		cases = append(cases, struct {
			name    string
			encoded string
			size    int
		}{name: fmt.Sprintf("internal-padding-%d", boundary), encoded: strings.Repeat("A", boundary-4) + "AA==AAAA", size: boundary/4*3 + 1})
	}
	for _, item := range cases {
		raw := make([]byte, item.size)
		payload := fixture.payload
		payload.Size, payload.ContentHash, payload.Data = uint64(len(raw)), bytesSHA256(raw), []byte{}
		prefix, err := json.Marshal(payload)
		if err != nil || !bytes.HasSuffix(prefix, []byte("\"}")) {
			t.Fatalf("%s fixture metadata: %v", item.name, err)
		}
		encoded := append(append(bytes.Clone(prefix[:len(prefix)-2]), []byte(item.encoded)...), '"', '}')
		entry := fixture.entry
		entry.Size, entry.ContentHash = payload.Size, payload.ContentHash
		var legacy campaignEvidenceFilePayload
		legacyErr := decodeStrictJSONBytes(encoded, &legacy)
		if legacyErr == nil && uint64(len(legacy.Data)) == entry.Size && bytesSHA256(legacy.Data) == entry.ContentHash {
			t.Fatalf("%s is not an independent malformed source", item.name)
		}
		decoded, canonical, actualErr := canonicalPublicCampaignFileDataV2(limits, payload.RunID, payload.Scope, entry, encoded)
		if actualErr == nil && canonical || decoded != nil {
			t.Fatalf("%s malformed base64 acquired a canonical source: canonical=%t err=%v", item.name, canonical, actualErr)
		}
		if json.Valid(encoded) {
			envelope := *fixture.envelope
			envelope.Payload = encoded
			wire := publicCampaignFileSignTestV2(t, fixture.key, &envelope)
			fixture.entry, fixture.envelope, fixture.wire = entry, &envelope, wire
			fixture.entry.EnvelopeHash = envelope.ContentHash
			fixture.requests, fixture.closes = 0, 0
			if decoded, err := fixture.probe.readPublicCampaignSourceV2(t.Context(), fixture.public, payload.RunID, envelope.Signer.Hex(), payload.Scope, fixture.entry); decoded != nil || err == nil || fixture.requests != 1 || fixture.closes != 1 {
				t.Fatalf("%s signed malformed file accepted: %v", item.name, err)
			}
		}
	}
	// Empty data and every quantum remainder retain the complete decoded
	// source and standard whole-string oracle.
	for _, size := range []int{0, 1, 2, 3, 48*1024 - 1, 48 * 1024, 48*1024 + 1} {
		raw := make([]byte, size)
		encoded := base64.StdEncoding.EncodeToString(raw)
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || !bytes.Equal(raw, decoded) {
			t.Fatal("standard base64 fixture differs")
		}
		payload := fixture.payload
		payload.Size, payload.ContentHash, payload.Data = uint64(size), bytesSHA256(raw), raw
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		entry := campaignEvidenceFileEntry{Path: payload.Path, Size: payload.Size, ContentHash: payload.ContentHash}
		actual, canonical, err := canonicalPublicCampaignFileDataV2(limits, payload.RunID, payload.Scope, entry, body)
		if err != nil || !canonical || !bytes.Equal(actual, decoded) {
			t.Fatalf("canonical base64 boundary %d: %v", size, err)
		}
	}
}

// Real body Close remains a pre-admission barrier, including the identical
// second replica. No original bytes or later network request escape failure.
func TestCampaignEvidenceReadbackV2CanonicalFilesJoinEveryCloseAndCancellation(t *testing.T) {
	t.Parallel()
	for _, failAt := range []int{1, 2} {
		for _, canceled := range []bool{false, true} {
			fixture := newPublicCampaignFileFixtureV2(t, 3)
			ctx, cancel := context.WithCancel(t.Context())
			sentinel := errors.New("synthetic canonical response close failure")
			fixture.closeAt, fixture.closeErr = failAt, sentinel
			if canceled {
				fixture.onClose = func(request int) {
					if request == failAt {
						cancel()
					}
				}
			}
			raw, err := fixture.probe.readPublicCampaignSourceV2(ctx, fixture.public, fixture.payload.RunID, fixture.envelope.Signer.Hex(), fixture.payload.Scope, fixture.entry)
			cancel()
			if raw != nil || !errors.Is(err, sentinel) || canceled && !errors.Is(err, context.Canceled) || fixture.requests != failAt || fixture.closes != failAt {
				t.Fatalf("replica%d canceled=%t escaped close failure: %v reads=%d closes=%d", failAt, canceled, err, fixture.requests, fixture.closes)
			}
			fixture.closeAt, fixture.closeErr, fixture.onClose = 0, nil, nil
			fixture.requests, fixture.closes = 0, 0
			raw, err = fixture.probe.readPublicCampaignSourceV2(t.Context(), fixture.public, fixture.payload.RunID, fixture.envelope.Signer.Hex(), fixture.payload.Scope, fixture.entry)
			if err != nil || !bytes.Equal(raw, fixture.payload.Data) || fixture.requests != 2 || fixture.closes != 2 {
				t.Fatalf("fresh default read failed after replica%d close rejection: %v", failAt, err)
			}
		}
	}
	fixture := newPublicCampaignFileFixtureV2(t, 3)
	fixture.onClose = func(request int) {
		if request == 2 {
			fixture.public.ChainID++
		}
	}
	if raw, err := fixture.probe.readPublicCampaignSourceV2(t.Context(), fixture.public, fixture.payload.RunID, fixture.envelope.Signer.Hex(), fixture.payload.Scope, fixture.entry); raw != nil || err == nil || fixture.requests != 2 || fixture.closes != 2 {
		t.Fatalf("equal second replica reused changed public identity: %v", err)
	}
}

// Process-global allocation measurements stay serial. Subtract the exact
// same real two-origin transfer/equality owner with an already-owned fixture
// decoder; actual default verification may add its raw owner, not Json copies.
func TestCampaignEvidenceReadbackV2CanonicalFilesBoundDefaultDecodeAllocation(t *testing.T) {
	fixture := newPublicCampaignFileFixtureV2(t, 1024*1024)
	limits, err := campaignEvidenceLimitsForConfig(fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	limit, err := limits.fileEnvelopeBytes(fixture.entry.Path, fixture.entry.Size)
	if err != nil {
		t.Fatal(err)
	}
	measure := func(actual bool) int64 {
		var callErr error
		var returned []byte
		result := testing.Benchmark(func(b *testing.B) {
			fixture.requests, fixture.closes = 0, 0
			for range b.N {
				if actual {
					returned, callErr = fixture.probe.readPublicCampaignSourceV2(t.Context(), fixture.public, fixture.payload.RunID, fixture.envelope.Signer.Hex(), fixture.payload.Scope, fixture.entry)
				} else {
					// This control does no verification and claims none. It
					// measures only the shared transport's allocation owner.
					_, callErr = fixture.probe.fetchReplicatedCampaignEnvelopeWithDecodeV2(t.Context(), fixture.public, fixture.entry.EnvelopeHash, campaignEvidenceFileKind, fixture.payload.RunID, fixture.envelope.Signer.Hex(), limit, limits, func([]byte) (*ReleaseEvidenceEnvelope, error) {
						return fixture.envelope, nil
					})
				}
				if callErr != nil {
					break
				}
			}
		})
		if callErr != nil || result.N <= 0 || fixture.requests != 2*result.N || fixture.closes != 2*result.N {
			t.Fatalf("allocation owner actual=%t failed: %v n=%d reads=%d closes=%d", actual, callErr, result.N, fixture.requests, fixture.closes)
		}
		if actual && !bytes.Equal(returned, fixture.payload.Data) {
			t.Fatal("default decoder allocation benchmark returned another source")
		}
		return result.AllocedBytesPerOp()
	}
	transportBytes := measure(false)
	actualBytes := measure(true)
	if actualBytes <= transportBytes || actualBytes-transportBytes >= int64(2*len(fixture.payload.Data)) {
		t.Fatalf("default public file repeated payload-sized Json allocation: actual=%d transport=%d excess=%d raw=%d", actualBytes, transportBytes, actualBytes-transportBytes, len(fixture.payload.Data))
	}
	t.Logf("actual default public decoder allocation bytes/op: total=%d shared two-origin transport=%d source-and-authentication=%d", actualBytes, transportBytes, actualBytes-transportBytes)
}
