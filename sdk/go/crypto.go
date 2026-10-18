package memro

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// Keypair holds a hex-encoded ed25519 key pair.
type Keypair struct {
	PublicKey  string // hex-encoded — this is the agent_id
	PrivateKey string // hex-encoded — keep secret
}

// GenerateKeypair creates a new ed25519 key pair.
func GenerateKeypair() (*Keypair, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("memro: generate keypair: %w", err)
	}
	return &Keypair{
		PublicKey:  hex.EncodeToString(pub),
		PrivateKey: hex.EncodeToString(priv),
	}, nil
}

// SignedHeaders contains the authentication headers for a signed request.
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

// parsePrivateKey accepts a hex-encoded 32-byte seed (Python SDK, dashboard)
// or a 64-byte expanded private key (Go/JS SDKs).
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

// SignBody signs a request: the signature covers the timestamp followed by
// the exact body bytes sent (empty for GET and DELETE).
func SignBody(agentID, privateKeyHex string, body []byte) (SignedHeaders, error) {
	priv, err := parsePrivateKey(privateKeyHex)
	if err != nil {
		return SignedHeaders{}, err
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := ed25519.Sign(priv, append([]byte(timestamp), body...))

	return SignedHeaders{
		AgentID:     agentID,
		Signature:   hex.EncodeToString(signature),
		Timestamp:   timestamp,
		ContentType: "application/json",
	}, nil
}
