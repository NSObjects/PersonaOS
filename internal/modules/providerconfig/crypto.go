package providerconfig

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

const ciphertextVersion byte = 1

// Cipher encrypts the complete service configuration. The service ID is
// authenticated as associated data so rows cannot be exchanged across services.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher accepts a base64-encoded 32-byte deployment key. It is never
// persisted together with the credentials.
func NewCipher(encoded string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return nil, errors.New("provider encryption key must be base64-encoded 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("invalid provider encryption key")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("cannot initialize provider encryption")
	}
	return &Cipher{aead: aead}, nil
}

// Seal encrypts a service snapshot using a fresh cryptographic nonce.
func (c *Cipher) Seal(service string, values Values) ([]byte, error) {
	if c == nil {
		return nil, errors.New("provider encryption key is not configured")
	}
	plain, err := json.Marshal(values)
	if err != nil {
		return nil, errors.New("cannot encode provider configuration")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.New("cannot generate provider configuration nonce")
	}
	out := append([]byte{ciphertextVersion}, nonce...)
	return c.aead.Seal(out, nonce, plain, []byte(service)), nil
}

// Open authenticates and decrypts the service snapshot. Errors never contain
// credential values or ciphertext.
func (c *Cipher) Open(service string, data []byte) (Values, error) {
	if c == nil {
		return nil, errors.New("provider encryption key is not configured")
	}
	n := c.aead.NonceSize()
	if len(data) < 1+n+c.aead.Overhead() || data[0] != ciphertextVersion {
		return nil, errors.New("invalid provider configuration ciphertext")
	}
	plain, err := c.aead.Open(nil, data[1:1+n], data[1+n:], []byte(service))
	if err != nil {
		return nil, errors.New("cannot authenticate provider configuration")
	}
	var values Values
	if err := json.Unmarshal(plain, &values); err != nil || values == nil {
		return nil, errors.New("invalid decrypted provider configuration")
	}
	return values, nil
}
