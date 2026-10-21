package auth

import (
	"crypto/sha256"
	"errors"
	"math/bits"
	"strconv"
	"time"
)

// Registration proof-of-work (ADR 0013). Registering an agent needs no
// account, so each new agent costs its creator about a second of CPU: a
// nonce whose SHA-256, over the public key and a timestamp, starts with
// enough zero bits. The proof is bound to the public key, and a key can be
// registered only once, so a proof cannot be reused for another agent.

// PowPrefix versions the hashed message.
const PowPrefix = "memro-identity-v1"

// PowMaxAge is how far a proof's timestamp may be from server time, in
// either direction. It is wider than MaxClockSkew because solving takes time.
const PowMaxAge = 10 * time.Minute

// maxNonceLen bounds the nonce so the server never hashes large input.
const maxNonceLen = 64

var (
	ErrPowMissing = errors.New("proof-of-work required")
	ErrPowStale   = errors.New("proof-of-work timestamp is too old or in the future")
	ErrPowNonce   = errors.New("proof-of-work nonce must be 1-64 letters or digits")
	ErrPowWeak    = errors.New("proof-of-work does not meet the difficulty")
)

// PowMessage is the exact text a client hashes:
// "memro-identity-v1:<public key>:<unix seconds>:<nonce>".
func PowMessage(publicKey string, timestamp int64, nonce string) string {
	return PowPrefix + ":" + publicKey + ":" + strconv.FormatInt(timestamp, 10) + ":" + nonce
}

// LeadingZeroBits counts the zero bits at the start of b.
func LeadingZeroBits(b []byte) int {
	n := 0
	for _, x := range b {
		if x == 0 {
			n += 8
			continue
		}
		return n + bits.LeadingZeros8(x)
	}
	return n
}

// VerifyPow checks a registration proof for publicKey at the given
// difficulty (zero bits). A difficulty of 0 or less accepts any well-formed
// proof.
func VerifyPow(publicKey string, timestamp int64, nonce string, difficulty int, now time.Time) error {
	if len(nonce) == 0 || len(nonce) > maxNonceLen {
		return ErrPowNonce
	}
	for _, c := range nonce {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return ErrPowNonce
		}
	}
	if d := now.Sub(time.Unix(timestamp, 0)); d > PowMaxAge || d < -PowMaxAge {
		return ErrPowStale
	}
	sum := sha256.Sum256([]byte(PowMessage(publicKey, timestamp, nonce)))
	if LeadingZeroBits(sum[:]) < difficulty {
		return ErrPowWeak
	}
	return nil
}

// SolvePow finds a nonce for publicKey at the given difficulty. Clients
// do this; the server uses it only in tests.
func SolvePow(publicKey string, timestamp int64, difficulty int) string {
	for i := uint64(0); ; i++ {
		nonce := strconv.FormatUint(i, 36)
		sum := sha256.Sum256([]byte(PowMessage(publicKey, timestamp, nonce)))
		if LeadingZeroBits(sum[:]) >= difficulty {
			return nonce
		}
	}
}
