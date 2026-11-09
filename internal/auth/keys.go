package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	KeyPrefixLength = 8
	SecretEntropyBytes = 24
	KeySchemePrefix = "trk_live_"
)

var (
	ErrInvalidKeyFormat = errors.New("invalid api key format: expected trk_live_<prefix>_<secret>")
	ErrKeyNotFound      = errors.New("api key not found")
	ErrKeyRevoked       = errors.New("api key has been revoked")
	ErrUnauthorized     = errors.New("unauthorized")
)

// GeneratedKey holds the plaintext secret (returned only once at creation)
// and the public prefix and hash to be persisted in PostgreSQL.
type GeneratedKey struct {
	Plaintext string 
	Prefix    string 
	Hash      string
}

// GenerateKey creates a new secure random API key
// and is never retrievable once discarded.
func GenerateKey() (*GeneratedKey, error) {
	prefixBytes, err := randomBytes(KeyPrefixLength / 2)
	if err != nil {
		return nil, fmt.Errorf("generate key prefix: %w", err)
	}
	prefix := KeySchemePrefix + hex.EncodeToString(prefixBytes)

	secretBytes, err := randomBytes(SecretEntropyBytes)
	if err != nil {
		return nil, fmt.Errorf("generate key secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)

	plaintext := prefix + "_" + secret
	return &GeneratedKey{
		Plaintext: plaintext,
		Prefix:    prefix,
		Hash:      HashKey(plaintext),
	}, nil
}

func HashKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// ExtractPrefix parses a plaintext tracker API key and extracts its public lookup prefix
func ExtractPrefix(plaintext string) (string, error) {
	plaintext = strings.TrimSpace(plaintext)
	rest, ok := strings.CutPrefix(plaintext, KeySchemePrefix)
	if !ok {
		return "", ErrInvalidKeyFormat
	}

	prefix, secret, found := strings.Cut(rest, "_")
	if !found || len(prefix) != KeyPrefixLength || secret == "" {
		return "", ErrInvalidKeyFormat
	}

	return KeySchemePrefix + prefix, nil
}

// VerifyHash compares a plaintext key against a stored SHA-256 hash in constant time
func VerifyHash(plaintext, expectedHash string) bool {
	computed := HashKey(plaintext)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(expectedHash)) == 1
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
