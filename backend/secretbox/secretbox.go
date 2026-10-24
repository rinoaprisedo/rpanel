// Package secretbox provides AES-256-GCM encryption for secrets stored in the database.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

var ErrNoSecret = errors.New("SECRET_KEY is not configured; set it in the backend .env (openssl rand -hex 32)")

// Cipher encrypts secrets with AES-256-GCM. The key is the SHA-256 of the
// configured secret so any passphrase length works.
type Cipher struct{ aead cipher.AEAD }

func NewCipher(secret string) (*Cipher, error) {
	if secret == "" {
		return nil, ErrNoSecret
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plain string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(c.aead.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (c *Cipher) Decrypt(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || len(raw) < c.aead.NonceSize() {
		return "", errors.New("stored credential is corrupt")
	}
	n := c.aead.NonceSize()
	out, err := c.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return "", errors.New("cannot decrypt stored credential (wrong SECRET_KEY?)")
	}
	return string(out), nil
}
