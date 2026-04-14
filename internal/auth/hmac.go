package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// HMACHasher provides deterministic hashing for API keys using HMAC-SHA256.
// Unlike Argon2 (which uses a random salt and is intentionally slow for passwords),
// HMAC is deterministic — the same (secret, input) pair always produces the same
// Only implemented this because I need to cache those APiKey values, even
// with no caching I would still need to iterate the entire universe of APIKeys
// to i could compute the hash argon2 and compare it with the stored value, which is not scalable at all.
type HMACHasher struct {
	secret []byte
}

func NewHMACHasher(secret []byte) *HMACHasher {
	return &HMACHasher{secret: secret}
}

func (h *HMACHasher) Hash(key string) string {
	mac := hmac.New(sha256.New, h.secret)
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *HMACHasher) Matches(rawKey string, storedHash string) bool {
	return hmac.Equal([]byte(h.Hash(rawKey)), []byte(storedHash))
}
