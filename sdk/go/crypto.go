package memro

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/bits"
	"strconv"
	"time"
)

// Keypair holds a hex-encoded ed25519 key pair.
type Keypair struct {
	PublicKey  string // hex public key; this is the agent ID
	PrivateKey string // hex 32-byte seed; keep it secret
}

// GenerateKeypair creates a new ed25519 key pair.
func GenerateKeypair() (*Keypair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("memro: generate keypair: %w", err)
	}
	return &Keypair{PublicKey: hex.EncodeToString(pub), PrivateKey: hex.EncodeToString(priv.Seed())}, nil
}

// parsePrivateKey accepts a hex 32-byte seed or a 64-byte expanded key.
func parsePrivateKey(privateKeyHex string) (ed25519.PrivateKey, error) {
	b, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("memro: decode private key: %w", err)
	}
	switch len(b) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(b), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(b), nil
	}
	return nil, fmt.Errorf("memro: invalid private key length: got %d bytes, want %d or %d", len(b), ed25519.SeedSize, ed25519.PrivateKeySize)
}

// SignRequest returns the v2 signature headers for a request:
// ed25519 over "<timestamp>\n<METHOD>\n<path?query>\n<body>".
func SignRequest(agentID, privateKeyHex, method, pathAndQuery string, body []byte) (map[string]string, error) {
	priv, err := parsePrivateKey(privateKeyHex)
	if err != nil {
		return nil, err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := append([]byte(ts+"\n"+method+"\n"+pathAndQuery+"\n"), body...)
	return map[string]string{
		"X-Agent-Id":          agentID,
		"X-Timestamp":         ts,
		"X-Signature":         hex.EncodeToString(ed25519.Sign(priv, payload)),
		"X-Signature-Version": "2",
	}, nil
}

// SignedHeaders contains legacy v1 signature headers.
type SignedHeaders struct {
	AgentID     string
	Signature   string
	Timestamp   string
	ContentType string
}

// Headers returns the headers as a map suitable for HTTP requests.
func (h SignedHeaders) Headers() map[string]string {
	return map[string]string{
		"X-Agent-Id":   h.AgentID,
		"X-Signature":  h.Signature,
		"X-Timestamp":  h.Timestamp,
		"Content-Type": h.ContentType,
	}
}

// SignBody makes a legacy v1 signature over timestamp + body only.
//
// Deprecated: use SignRequest, which also covers the method, path and query.
func SignBody(agentID, privateKeyHex string, body []byte) (SignedHeaders, error) {
	priv, err := parsePrivateKey(privateKeyHex)
	if err != nil {
		return SignedHeaders{}, err
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := ed25519.Sign(priv, append([]byte(timestamp), body...))
	return SignedHeaders{AgentID: agentID, Signature: hex.EncodeToString(signature), Timestamp: timestamp, ContentType: "application/json"}, nil
}

// PowProof is a registration proof-of-work (ADR 0013).
type PowProof struct {
	Timestamp int64  `json:"timestamp"`
	Nonce     string `json:"nonce"`
}

// SolvePow finds a nonce whose SHA-256 over
// "memro-identity-v1:<publicKey>:<timestamp>:<nonce>" starts with difficulty
// zero bits. Difficulty 20 takes a fraction of a second.
func SolvePow(publicKey string, difficulty int) PowProof {
	ts := time.Now().Unix()
	prefix := "memro-identity-v1:" + publicKey + ":" + strconv.FormatInt(ts, 10) + ":"
	for i := uint64(0); ; i++ {
		nonce := strconv.FormatUint(i, 16)
		sum := sha256.Sum256([]byte(prefix + nonce))
		if leadingZeroBits(sum[:]) >= difficulty {
			return PowProof{Timestamp: ts, Nonce: nonce}
		}
	}
}

func leadingZeroBits(b []byte) int {
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
