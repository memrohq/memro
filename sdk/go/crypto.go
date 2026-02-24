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

// SignBody signs the given body bytes with the agent's private key.
func SignBody(agentID, privateKeyHex string, body []byte) (SignedHeaders, error) {
	privBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return SignedHeaders{}, fmt.Errorf("memro: decode private key: %w", err)
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		return SignedHeaders{}, fmt.Errorf("memro: invalid private key length: got %d, want %d", len(privBytes), ed25519.PrivateKeySize)
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := ed25519.Sign(ed25519.PrivateKey(privBytes), body)

	return SignedHeaders{
		AgentID:     agentID,
		Signature:   hex.EncodeToString(signature),
		Timestamp:   timestamp,
		ContentType: "application/json",
	}, nil
}
