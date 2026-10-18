package memro

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

// The server verifies ed25519(timestamp || body); both key formats must work.
func TestSignBodyMatchesServerPayload(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	body := []byte(`{"content":"hi"}`)
	for name, key := range map[string]string{
		"64-byte key":  hex.EncodeToString(priv),
		"32-byte seed": hex.EncodeToString(priv.Seed()),
	} {
		h, err := SignBody(hex.EncodeToString(pub), key, body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		sig, _ := hex.DecodeString(h.Signature)
		if !ed25519.Verify(pub, append([]byte(h.Timestamp), body...), sig) {
			t.Fatalf("%s: signature does not cover timestamp+body", name)
		}
	}
	if _, err := SignBody("x", "abcd", body); err == nil {
		t.Fatal("expected error for a short key")
	}
}
