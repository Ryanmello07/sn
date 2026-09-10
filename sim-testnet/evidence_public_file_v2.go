// Public file readback owns exact returned source bytes once. Producer-shaped
// carriers get fresh grammar/hash/signature proof without repeated Json scans;
// every other historical spelling uses the original strict decoder.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// Only an independently supplied signed manifest entry supplies these fields.
// Plain base64 is checked by the standard decoder, never treated as trusted
// solely because the enclosing metadata or requested hash matches.
func canonicalPublicCampaignFileDataV2(limits campaignEvidenceLimits, runId, scope string, entry campaignEvidenceFileEntry, encoded []byte) ([]byte, bool, error) {
	expected := campaignEvidenceFilePayload{
		Schema: campaignEvidenceFileSchema, RunID: runId, Scope: scope,
		Path: entry.Path, ContentHash: entry.ContentHash, Size: entry.Size, Data: []byte{},
	}
	framing, err := json.Marshal(expected)
	if err != nil {
		return nil, false, err
	}
	if !bytes.HasSuffix(framing, []byte("\"}")) {
		return nil, false, errors.New("public file canonical framing is incomplete")
	}
	prefix := framing[:len(framing)-2]
	if len(encoded) < len(prefix)+2 || !bytes.HasPrefix(encoded, prefix) || !bytes.HasSuffix(encoded, []byte("\"}")) {
		return nil, false, nil
	}
	source := encoded[len(prefix) : len(encoded)-2]
	// Raw CR/LF are accepted by base64 but forbidden in Json strings. Escaped
	// historical data, array/null data and different metadata use the fallback.
	if bytes.IndexByte(source, '\r') >= 0 || bytes.IndexByte(source, '\n') >= 0 || bytes.IndexByte(source, '\\') >= 0 || bytes.IndexByte(source, '"') >= 0 {
		return nil, false, nil
	}
	if err := validateCampaignMetadataRawSizeV2(limits, entry.Path, entry.Size); err != nil {
		return nil, true, err
	}
	if entry.Size > uint64(^uint(0)>>1)-2 || uint64(len(source)) != ((entry.Size+2)/3)*4 {
		return nil, true, errors.New("public file base64 differs from its exact source byte owner")
	}
	// Establish the exact destination capacity even for malformed input.
	// Whole-string Decode rejects any data after padding and retains the
	// historical non-strict padding-bit rule; no chunk can reset padding.
	padding := 0
	for index := len(source); index > 0 && source[index-1] == '='; index-- {
		padding++
		if padding > 2 {
			return nil, true, errors.New("public file base64 has excessive final padding")
		}
	}
	decodedBytes := uint64(len(source)/4*3) - uint64(padding)
	if decodedBytes != entry.Size {
		return nil, true, errors.New("public file base64 padding differs from its exact source byte owner")
	}
	raw := make([]byte, int(entry.Size))
	count, err := base64.StdEncoding.Decode(raw, source)
	if err != nil {
		return nil, true, err
	}
	if uint64(count) != entry.Size || bytesSHA256(raw) != entry.ContentHash {
		return nil, true, errors.New("compact public source differs from its exact signed manifest entry")
	}
	return raw, true, nil
}

// The fast path proves exact outer framing and typed payload grammar in this
// invocation, then hashes those bytes afresh and recovers the signer. Nothing
// is cached across files, replicas, mutable envelopes or later reads.
func verifyPublicCampaignFileWireV2(limits campaignEvidenceLimits, runId, scope string, entry campaignEvidenceFileEntry, wire []byte) (*ReleaseEvidenceEnvelope, []byte, bool, error) {
	limit, err := limits.fileEnvelopeBytes(entry.Path, entry.Size)
	if err != nil || len(wire) == 0 || uint64(len(wire)) > uint64(limit) {
		return nil, nil, false, errors.Join(errors.New("public file exceeds its signed source-size bound"), err)
	}
	envelope, decodeErr := decodeFinalPriorCarrierEnvelopeV2(wire)
	if decodeErr == nil && verifyFinalCanonicalPriorWireV2(&envelope, envelope.Payload, wire) == nil {
		if err := validateReleaseEvidenceIdentity(&envelope); err != nil {
			return nil, nil, true, err
		}
		raw, canonical, err := canonicalPublicCampaignFileDataV2(limits, runId, scope, entry, envelope.Payload)
		if err != nil {
			return nil, nil, canonical, err
		}
		if canonical {
			digest, err := evidenceDigestFromCanonicalPayload(&envelope, envelope.Payload)
			if err != nil {
				return nil, nil, true, err
			}
			if err := verifyEvidenceSignature(&envelope, digest, nil); err != nil {
				return nil, nil, true, err
			}
			if err := validateCampaignMetadataRawV2(limits, entry.Path, raw); err != nil {
				return nil, nil, true, err
			}
			return &envelope, raw, true, nil
		}
	}
	// This destination is fresh: a failed borrowed split cannot lend metadata
	// to a historical envelope with omitted, reordered or duplicate fields.
	envelope = ReleaseEvidenceEnvelope{}
	if err := decodeStrictJSONBytes(wire, &envelope); err != nil {
		return nil, nil, false, err
	}
	if err := verifyEvidence(&envelope, nil); err != nil {
		return nil, nil, false, err
	}
	var payload campaignEvidenceFilePayload
	if err := decodeStrictJSONBytes(envelope.Payload, &payload); err != nil {
		return nil, nil, false, err
	}
	if payload.Schema != campaignEvidenceFileSchema || payload.RunID != runId || payload.Scope != scope || payload.Path != entry.Path || payload.Size != entry.Size || payload.ContentHash != entry.ContentHash || uint64(len(payload.Data)) != entry.Size || bytesSHA256(payload.Data) != entry.ContentHash {
		return nil, nil, false, errors.New("compact public source differs from its exact signed manifest entry")
	}
	if err := validateCampaignMetadataRawV2(limits, entry.Path, payload.Data); err != nil {
		return nil, nil, false, err
	}
	return &envelope, payload.Data, false, nil
}
