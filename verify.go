package ogxmparser

import (
	"crypto/ed25519"
	"crypto/sha256"
)

// SignatureStatus is a verifier's verdict on one signature (8.1).
type SignatureStatus int

const (
	// Unverifiable: unknown algorithm or covered section, or no public key.
	Unverifiable SignatureStatus = iota
	// Valid: the signature matches the payload under the key.
	Valid
	// Invalid: the signature does not match the payload.
	Invalid
)

func (s SignatureStatus) String() string {
	switch s {
	case Valid:
		return "valid"
	case Invalid:
		return "invalid"
	}
	return "unverifiable"
}

// SignatureReport is the verdict on one SIGN or MSIG.
type SignatureReport struct {
	// Analysis is the index of the analysis block a SIGN belongs to, or -1
	// for an MSIG.
	Analysis int
	KeyID    string
	Status   SignatureStatus
}

// KeyLookup returns the public key for a key id when the file carries none,
// or nil. Which keys to trust is the caller's decision, never the file's: a
// key embedded in the file only identifies the signer.
type KeyLookup func(keyID string) ed25519.PublicKey

// VerifySignatures checks every SIGN and MSIG (8.1, 8.6). A failed signature
// is reported, never a load error. keys may be nil.
func (f *File) VerifySignatures(keys KeyLookup) []SignatureReport {
	var out []SignatureReport
	for i, a := range f.Analyses {
		if a.Signature == nil {
			continue
		}
		out = append(out, SignatureReport{Analysis: i, KeyID: a.Signature.KeyID, Status: verify(a.Signature, f.analysisPayload(&f.Analyses[i]), keys)})
	}
	for i := range f.MatchSignatures {
		s := &f.MatchSignatures[i]
		status := Unverifiable
		payload, ok := f.matchPayload(s)
		switch {
		case !ok && s.Covers&1 != 0 && f.Clock == nil:
			status = Invalid
		case ok:
			status = verify(s, payload, keys)
		}
		out = append(out, SignatureReport{Analysis: -1, KeyID: s.KeyID, Status: status})
	}
	return out
}

// analysisPayload is the signing payload of an analysis block (8.1.1).
func (f *File) analysisPayload(a *Analysis) []byte {
	var md [32]byte
	switch {
	case f.Match != nil:
		md = f.MatchDigest()
	case a.MatchDigest != nil:
		md = *a.MatchDigest
	}
	h := sha256.New()
	h.Write(a.analPayload)
	h.Write(a.decsPayload)
	payload := append([]byte("OGXM-ANAL-SIG-v2"), md[:]...)
	return h.Sum(payload)
}

// matchPayload is the signing payload of an MSIG (8.6.1); false when it
// cannot be built: an unknown covered section, or a covered CLCK missing.
func (f *File) matchPayload(s *Signature) ([]byte, bool) {
	if s.Covers&^1 != 0 || (s.Covers&1 != 0 && f.Clock == nil) {
		return nil, false
	}
	md := f.MatchDigest()
	payload := append([]byte("OGXM-MTCH-SIG-v2"), md[:]...)
	if s.Covers&1 != 0 {
		sum := sha256.Sum256(f.Clock)
		payload = append(payload, sum[:]...)
	}
	return payload, true
}

func verify(s *Signature, payload []byte, keys KeyLookup) SignatureStatus {
	if s.Algorithm != 1 {
		return Unverifiable
	}
	key := ed25519.PublicKey(s.PublicKey)
	if len(key) == 0 && keys != nil {
		key = keys(s.KeyID)
	}
	if len(key) != ed25519.PublicKeySize {
		return Unverifiable
	}
	if len(s.Signature) != ed25519.SignatureSize || !ed25519.Verify(key, payload, s.Signature) {
		return Invalid
	}
	return Valid
}
