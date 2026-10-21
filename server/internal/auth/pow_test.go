package auth

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

const powKey = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"

func TestLeadingZeroBits(t *testing.T) {
	cases := []struct {
		in   []byte
		want int
	}{
		{[]byte{0x80}, 0},
		{[]byte{0x01}, 7},
		{[]byte{0x00, 0x10}, 11},
		{[]byte{0x00, 0x00}, 16},
	}
	for _, c := range cases {
		if got := LeadingZeroBits(c.in); got != c.want {
			t.Errorf("LeadingZeroBits(%x) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPowSolveAndVerify(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ts := now.Unix()
	nonce := SolvePow(powKey, ts, 12)
	if err := VerifyPow(powKey, ts, nonce, 12, now); err != nil {
		t.Fatalf("valid proof rejected: %v", err)
	}
	sum := sha256.Sum256([]byte(PowMessage(powKey, ts, nonce)))
	if LeadingZeroBits(sum[:]) < 12 {
		t.Fatal("SolvePow returned a weak nonce")
	}
}

func TestPowRejects(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ts := now.Unix()
	nonce := SolvePow(powKey, ts, 12)
	other := "f" + powKey[1:]

	cases := []struct {
		name  string
		key   string
		ts    int64
		nonce string
		bits  int
		want  error
	}{
		{"other key", other, ts, nonce, 12, ErrPowWeak},
		{"too weak for higher difficulty", powKey, ts, nonce, 40, ErrPowWeak},
		{"stale", powKey, ts - int64(PowMaxAge/time.Second) - 1, nonce, 0, ErrPowStale},
		{"future", powKey, ts + int64(PowMaxAge/time.Second) + 1, nonce, 0, ErrPowStale},
		{"empty nonce", powKey, ts, "", 0, ErrPowNonce},
		{"bad nonce", powKey, ts, "abc:def", 0, ErrPowNonce},
		{"long nonce", powKey, ts, string(make([]byte, 65)), 0, ErrPowNonce},
	}
	for _, c := range cases {
		if err := VerifyPow(c.key, c.ts, c.nonce, c.bits, now); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

// The message format is a contract with the SDKs, so pin it.
func TestPowMessageFormat(t *testing.T) {
	if got, want := PowMessage("ab", 12, "z9"), "memro-identity-v1:ab:12:z9"; got != want {
		t.Fatalf("PowMessage = %q, want %q", got, want)
	}
}
