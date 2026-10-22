// Package auth implements agent request signing, replay protection and
// dashboard session tokens.
package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MaxClockSkew is how far a request timestamp may drift from server time.
const MaxClockSkew = 5 * time.Minute

var (
	ErrBadPublicKey = errors.New("invalid public key format")
	ErrBadSignature = errors.New("invalid signature format")
	ErrBadTimestamp = errors.New("invalid timestamp format")
	ErrExpired      = errors.New("request timestamp expired")
	ErrVerify       = errors.New("invalid signature")
	ErrReplay       = errors.New("request already used")
)

func ParsePublicKey(pubHex string) (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(pubHex)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, ErrBadPublicKey
	}
	return ed25519.PublicKey(b), nil
}

// CheckTimestamp validates freshness of a unix-seconds timestamp header.
func CheckTimestamp(ts string, now time.Time) error {
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrBadTimestamp
	}
	d := now.Sub(time.Unix(sec, 0))
	if d > MaxClockSkew || d < -MaxClockSkew {
		return ErrExpired
	}
	return nil
}

// PayloadV1 is the legacy signing payload: timestamp followed by the body.
// It does not bind the method, path or query string.
func PayloadV1(ts string, body []byte) []byte {
	p := make([]byte, 0, len(ts)+len(body))
	p = append(p, ts...)
	return append(p, body...)
}

// PayloadV2 binds the request line as well:
// "<timestamp>\n<METHOD>\n<path?query>\n<body>".
func PayloadV2(ts, method, pathAndQuery string, body []byte) []byte {
	p := make([]byte, 0, len(ts)+len(method)+len(pathAndQuery)+len(body)+3)
	p = append(p, ts...)
	p = append(p, '\n')
	p = append(p, method...)
	p = append(p, '\n')
	p = append(p, pathAndQuery...)
	p = append(p, '\n')
	return append(p, body...)
}

func Verify(pub ed25519.PublicKey, payload []byte, sigHex string) error {
	if len(sigHex) != 2*ed25519.SignatureSize {
		return ErrBadSignature
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	return VerifyBytes(pub, payload, sig)
}

// VerifyBytes verifies a decoded ed25519 signature directly.
func VerifyBytes(pub ed25519.PublicKey, payload []byte, sig []byte) error {
	if len(sig) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	if !ed25519.Verify(pub, payload, sig) {
		return ErrVerify
	}
	return nil
}

// ReplayCache remembers signatures seen within the clock-skew window so a
// captured request cannot be sent twice.
type ReplayCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func NewReplayCache() *ReplayCache {
	return &ReplayCache{seen: map[string]time.Time{}}
}

// Use records sig and reports false if it was already used.
// It normalizes sig to canonical lowercase hex so case variations cannot
// bypass replay protection.
func (c *ReplayCache) Use(sig string, now time.Time) bool {
	key := strings.ToLower(sig)
	c.mu.Lock()
	defer c.mu.Unlock()
	if exp, ok := c.seen[key]; ok && now.Before(exp) {
		return false
	}
	c.seen[key] = now.Add(2 * MaxClockSkew)
	if len(c.seen) > 100_000 {
		for k, exp := range c.seen {
			if now.After(exp) {
				delete(c.seen, k)
			}
		}
	}
	return true
}

// UseBytes records the raw signature bytes, keyed on lowercase hex.
func (c *ReplayCache) UseBytes(sig []byte, now time.Time) bool {
	return c.Use(hex.EncodeToString(sig), now)
}

// --- Session tokens & OTP codes ------------------------------------------------

// NewSessionToken returns a random bearer token and its storage hash.
func NewSessionToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewOTP returns a uniformly random 6-digit code.
func NewOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// HashOTP binds the code to the user's email so equal codes hash differently.
func HashOTP(email, code string) string {
	sum := sha256.Sum256([]byte(email + ":" + code))
	return hex.EncodeToString(sum[:])
}

func EqualHash(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// GenerateKeyPair returns a hex public key and a hex 32-byte private seed.
// The seed form is what the Python SDK uses and what the JS SDK accepts.
func GenerateKeyPair() (pubHex, seedHex string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return hex.EncodeToString(pub), hex.EncodeToString(priv.Seed()), nil
}

// --- API keys & device codes -------------------------------------------------

// APIKeyPrefix marks Memro API keys so they are easy to recognise and to
// detect in leaked-secret scanners.
const APIKeyPrefix = "memro_sk_"

// NewAPIKey returns a new key, a short display prefix and its storage hash.
func NewAPIKey() (key, prefix, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", err
	}
	key = APIKeyPrefix + base64.RawURLEncoding.EncodeToString(b)
	return key, key[:len(APIKeyPrefix)+6], HashToken(key), nil
}

// userCodeAlphabet avoids vowels (no accidental words) and look-alike characters.
const userCodeAlphabet = "BCDFGHJKLMNPQRSTVWXZ"

// NewDeviceCode returns the secret the CLI polls with and the short code the
// user confirms in the dashboard (e.g. "KQZT-HMRW").
func NewDeviceCode() (deviceCode, userCode string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	deviceCode = base64.RawURLEncoding.EncodeToString(b)
	code := make([]byte, 8)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(userCodeAlphabet))))
		if err != nil {
			return "", "", err
		}
		code[i] = userCodeAlphabet[n.Int64()]
	}
	return deviceCode, string(code[:4]) + "-" + string(code[4:]), nil
}
