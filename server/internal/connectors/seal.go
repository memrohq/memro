package connectors

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// sealer encrypts provider credentials with AES-256-GCM. Each ciphertext
// starts with a version byte and the first 4 bytes of SHA-256(key), so a
// rotated key can still open values sealed with the previous one.
type sealer struct {
	keys []keyEntry // keys[0] seals; all of them open
}

type keyEntry struct {
	id   [4]byte
	aead cipher.AEAD
}

const sealVersion = 1

func newSealer(current, previous string) (*sealer, error) {
	s := &sealer{}
	for i, k := range []string{current, previous} {
		if k == "" {
			if i == 0 {
				return nil, errors.New("CONNECTOR_ENCRYPTION_KEY is not set")
			}
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(raw) != 32 {
			return nil, fmt.Errorf("connector encryption key %d must be 32 bytes, base64-encoded (openssl rand -base64 32)", i+1)
		}
		block, err := aes.NewCipher(raw)
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(raw)
		var e keyEntry
		copy(e.id[:], sum[:4])
		e.aead = aead
		s.keys = append(s.keys, e)
	}
	return s, nil
}

// Seal encrypts v (marshalled as JSON). The ciphertext is bound to aad, the
// connection ID, so a sealed value cannot be moved to another connection.
func (s *sealer) Seal(v any, aad string) ([]byte, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	k := s.keys[0]
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{sealVersion}, k.id[:]...)
	out = append(out, nonce...)
	return k.aead.Seal(out, nonce, plain, []byte(aad)), nil
}

// Open decrypts sealed into v. stale reports that it was sealed with the
// previous key and should be sealed again.
func (s *sealer) Open(sealed []byte, aad string, v any) (stale bool, err error) {
	if len(sealed) < 5 || sealed[0] != sealVersion {
		return false, errors.New("unsupported sealed value")
	}
	for i, k := range s.keys {
		if [4]byte(sealed[1:5]) != k.id {
			continue
		}
		ns := k.aead.NonceSize()
		if len(sealed) < 5+ns {
			return false, errors.New("sealed value too short")
		}
		plain, err := k.aead.Open(nil, sealed[5:5+ns], sealed[5+ns:], []byte(aad))
		if err != nil {
			return false, errors.New("sealed value failed authentication")
		}
		return i > 0, json.Unmarshal(plain, v)
	}
	return false, errors.New("sealed with an unknown key; set CONNECTOR_ENCRYPTION_KEY_PREVIOUS")
}
