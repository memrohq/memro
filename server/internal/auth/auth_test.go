package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func sign(t *testing.T, priv ed25519.PrivateKey, payload []byte) string {
	t.Helper()
	return hex.EncodeToString(ed25519.Sign(priv, payload))
}

func TestVerifyV1AndV2(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	body := []byte(`{"content":"hi"}`)

	if err := Verify(pub, PayloadV1(ts, body), sign(t, priv, PayloadV1(ts, body))); err != nil {
		t.Fatalf("v1: %v", err)
	}
	v2 := PayloadV2(ts, "POST", "/memory", body)
	if err := Verify(pub, v2, sign(t, priv, v2)); err != nil {
		t.Fatalf("v2: %v", err)
	}
	// A v2 signature must not validate for another path.
	other := PayloadV2(ts, "DELETE", "/identity/x", body)
	if err := Verify(pub, other, sign(t, priv, v2)); err != ErrVerify {
		t.Fatalf("expected ErrVerify for path change, got %v", err)
	}
}

// TestSharedVector pins the signing payload so every SDK can test against
// the same known input and output.
func TestSharedVector(t *testing.T) {
	seed, _ := hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	if hex.EncodeToString(pub) != "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a" {
		t.Fatal("unexpected public key for RFC 8032 test seed")
	}
	payload := PayloadV1("1700000000", []byte(`{"a":1}`))
	if string(payload) != `1700000000{"a":1}` {
		t.Fatalf("v1 payload changed: %q", payload)
	}
	if err := Verify(pub, payload, sign(t, priv, payload)); err != nil {
		t.Fatal(err)
	}
}

func TestCheckTimestamp(t *testing.T) {
	now := time.Now()
	if err := CheckTimestamp(strconv.FormatInt(now.Unix(), 10), now); err != nil {
		t.Fatal(err)
	}
	if err := CheckTimestamp(strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10), now); err != ErrExpired {
		t.Fatalf("expected expired, got %v", err)
	}
	if err := CheckTimestamp("abc", now); err != ErrBadTimestamp {
		t.Fatalf("expected bad timestamp, got %v", err)
	}
}

func TestReplayCache(t *testing.T) {
	c := NewReplayCache()
	now := time.Now()
	if !c.Use("sig", now) {
		t.Fatal("first use rejected")
	}
	if c.Use("sig", now.Add(time.Second)) {
		t.Fatal("replay accepted")
	}
	if !c.Use("sig", now.Add(11*time.Minute)) {
		t.Fatal("expired entry should be reusable")
	}
}

func TestOTPAndTokens(t *testing.T) {
	code, err := NewOTP()
	if err != nil || len(code) != 6 {
		t.Fatalf("bad otp %q %v", code, err)
	}
	if !EqualHash(HashOTP("a@b.co", code), HashOTP("a@b.co", code)) {
		t.Fatal("hash mismatch")
	}
	tok, h, err := NewSessionToken()
	if err != nil || HashToken(tok) != h {
		t.Fatal("token hash mismatch")
	}
	pub, seed, err := GenerateKeyPair()
	if err != nil || len(pub) != 64 || len(seed) != 64 {
		t.Fatalf("bad keypair %d %d %v", len(pub), len(seed), err)
	}
}
