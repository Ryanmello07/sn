// Signing owns one canonical payload through its immediate local write. Only
// freshly encoded input uses that fast path; incoming mutable envelopes are
// canonicalized and authenticated independently on every verification.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
)

// The constructor is the only production source of these bytes. Json.Marshal
// supplies the same compact, HTML-escaped payload used by the legacy envelope.
// This owner is local to one call, not an authentication or corpus-wide cache.
type evidencePayloadOwner struct {
	encoded      []byte
	maximumBytes uint64
}

// Apply the exact typed admission before encoding. The admitted limit travels
// with those bytes so an immutable retry does not hash the raw source again.
func marshalEvidencePayload(cfg *ResolvedConfig, kind string, payload any) (*evidencePayloadOwner, error) {
	if cfg == nil || cfg.Config == nil || cfg.Public == nil {
		return nil, errors.New("release evidence payload owner is incomplete")
	}
	maximum, err := campaignEvidencePayloadLimitV2(cfg, kind, payload)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if maximum != 0 && uint64(len(encoded)) > maximum {
		return nil, errors.New("campaign metadata payload exceeds its configured wire owner")
	}
	return &evidencePayloadOwner{encoded: encoded, maximumBytes: maximum}, nil
}

// Marshal every actual envelope field with the standard encoder, replacing
// only the null payload token with separately encoded canonical payload bytes.
// This preserves field order, omitempty and escaping without scanning a large
// RawMessage again or duplicating the signed wire schema by hand.
func evidenceWireFraming(envelope *ReleaseEvidenceEnvelope) ([]byte, []byte, error) {
	if envelope == nil {
		return nil, nil, errors.New("release evidence envelope is missing")
	}
	metadata := *envelope
	metadata.Payload = nil
	encoded, err := json.Marshal(&metadata)
	if err != nil {
		return nil, nil, err
	}
	const marker = `"payload":null`
	index := bytes.Index(encoded, []byte(marker))
	if index < 0 {
		return nil, nil, errors.New("release evidence payload framing is missing")
	}
	start := index + len(marker) - len("null")
	return encoded[:start], encoded[start+len("null"):], nil
}

// Hash the original three byte spans in order. No full unsigned-envelope
// allocation, second payload encode or borrowed digest supplies this hash.
func evidenceDigestFromCanonicalPayload(envelope *ReleaseEvidenceEnvelope, payload []byte) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	if envelope == nil || len(payload) == 0 {
		return digest, errors.New("release evidence digest owner is missing")
	}
	unsigned := *envelope
	unsigned.ContentHash, unsigned.Signature = "", ""
	prefix, suffix, err := evidenceWireFraming(&unsigned)
	if err != nil {
		return digest, err
	}
	hash := sha256.New()
	_, _ = hash.Write(prefix)
	_, _ = hash.Write(payload)
	_, _ = hash.Write(suffix)
	copy(digest[:], hash.Sum(digest[:0]))
	return digest, nil
}

// Only use with the canonical payload owned by this call. Untrusted callers
// must go through marshalEvidenceEnvelope, which validates RawMessage afresh.
func marshalEvidenceWithCanonicalPayload(envelope *ReleaseEvidenceEnvelope, payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("release evidence wire payload is missing")
	}
	prefix, suffix, err := evidenceWireFraming(envelope)
	if err != nil {
		return nil, err
	}
	encoded := make([]byte, 0, len(prefix)+len(payload)+len(suffix))
	encoded = append(encoded, prefix...)
	encoded = append(encoded, payload...)
	encoded = append(encoded, suffix...)
	return encoded, nil
}

// A mutable envelope never borrows the local signing owner's admission.
func marshalEvidenceEnvelope(envelope *ReleaseEvidenceEnvelope) ([]byte, error) {
	if envelope == nil {
		return nil, errors.New("release evidence envelope is missing")
	}
	payload, err := json.Marshal(envelope.Payload)
	if err != nil {
		return nil, err
	}
	return marshalEvidenceWithCanonicalPayload(envelope, payload)
}
