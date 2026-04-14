package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHMACHasher_Deterministic(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!!")
	h := NewHMACHasher(secret)

	hash1 := h.Hash("my-api-key")
	hash2 := h.Hash("my-api-key")

	assert.Equal(t, hash1, hash2, "same input should always produce the same hash")
}

func TestHMACHasher_DifferentInputs(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!!")
	h := NewHMACHasher(secret)

	hash1 := h.Hash("key-a")
	hash2 := h.Hash("key-b")

	assert.NotEqual(t, hash1, hash2, "different inputs should produce different hashes")
}

func TestHMACHasher_DifferentSecrets(t *testing.T) {
	h1 := NewHMACHasher([]byte("secret-1"))
	h2 := NewHMACHasher([]byte("secret-2"))

	hash1 := h1.Hash("same-key")
	hash2 := h2.Hash("same-key")

	assert.NotEqual(t, hash1, hash2, "different secrets should produce different hashes")
}

func TestHMACHasher_Matches(t *testing.T) {
	secret := []byte("test-secret")
	h := NewHMACHasher(secret)

	raw := "my-api-key-12345"
	stored := h.Hash(raw)

	assert.True(t, h.Matches(raw, stored))
	assert.False(t, h.Matches("wrong-key", stored))
}

func TestHMACHasher_HashIsHex(t *testing.T) {
	h := NewHMACHasher([]byte("secret"))
	hash := h.Hash("key")

	// SHA256 produces 32 bytes = 64 hex chars
	assert.Len(t, hash, 64)
}
