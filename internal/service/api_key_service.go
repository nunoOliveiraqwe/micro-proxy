package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/auth"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"go.uber.org/zap"
)

var ErrorInvalidApiKeyRequest = fmt.Errorf("invalid API key request: no data")
var ErrorInvalidScopesApiKeyRequest = fmt.Errorf("invalid API key request: invalid scopes")
var ErrorInvalidAliasApiKeyRequest = fmt.Errorf("invalid API key request: invalid alias")
var ErrorInvalidExpiryDateApiKeyRequest = fmt.Errorf("invalid API key request: invalid expiry date")
var ErrorDuplicatedAliasApiKeyRequest = fmt.Errorf("invalid API key request: duplicated alias")

type CreateApiKeyRequest struct {
	Alias      string   `json:"alias"`
	Scopes     []string `json:"scopes"`
	ExpiryDate time.Time
}

type apiKeyCacheEntry struct {
	allowedScopes map[string]byte
	expiresAt     time.Time
	lastSeen      time.Time
}

func (e *apiKeyCacheEntry) Touch() {
	e.lastSeen = time.Now()
}

func (e *apiKeyCacheEntry) GetLastReadAt() time.Time {
	return e.lastSeen
}

type ApiKeyService struct {
	store  store.ApiKeyStore
	cache  *util.Cache[*apiKeyCacheEntry]
	hasher *auth.HMACHasher
}

func NewApiKeyService(store store.ApiKeyStore, hmacSecret []byte) *ApiKeyService {
	zap.S().Info("Initializing API Key Service")

	cacheOpts := &util.CacheOptions{
		MaxEntries:      10000,
		TTL:             1 * time.Hour,
		CleanupInterval: 1 * time.Hour,
	}

	cache, err := util.NewCache[*apiKeyCacheEntry](cacheOpts)
	if err != nil {
		zap.S().Fatalf("Failed to initialize API key cache: %v", err)
	}

	return &ApiKeyService{
		store:  store,
		cache:  cache,
		hasher: auth.NewHMACHasher(hmacSecret),
	}
}

func (s *ApiKeyService) IsKeyValidForScope(key string, scope string) (bool, error) {
	// HMAC the raw key immediately — from this point on we only work with the hash.
	// The raw key is never stored anywhere (not in cache, not in DB).
	hashedKey := s.hasher.Hash(key)

	// Check cache first (keyed by hashed key, no raw keys in memory)
	entry, err := s.cache.GetValue(hashedKey)
	if err == nil {
		if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
			s.cache.Evict(hashedKey)
			return false, nil
		}
		_, ok := entry.allowedScopes[scope]
		return ok, nil
	}

	// Cache miss — look up the hashed key in the DB.
	// HMAC is deterministic, so WHERE key = hash works.
	valid, err := s.store.IsKeyValidForScope(context.Background(), hashedKey, scope)
	if err != nil {
		return false, fmt.Errorf("failed to check key scope: %w", err)
	}

	// On a valid key, populate the cache so subsequent checks are instant
	if valid {
		s.warmCache(hashedKey)
	}

	return valid, nil
}

func (s *ApiKeyService) CreateApiKey(ctx context.Context, apiKeyRequest *CreateApiKeyRequest) (*domain.ApiKey, error) {
	if apiKeyRequest == nil {
		return nil, ErrorInvalidApiKeyRequest
	} else if strings.EqualFold(apiKeyRequest.Alias, "") {
		return nil, ErrorInvalidAliasApiKeyRequest
	} else if apiKeyRequest.Scopes == nil || len(apiKeyRequest.Scopes) == 0 {
		return nil, ErrorInvalidScopesApiKeyRequest
	} else if !apiKeyRequest.ExpiryDate.IsZero() && apiKeyRequest.ExpiryDate.Before(time.Now()) {
		return nil, ErrorInvalidExpiryDateApiKeyRequest
	}

	zap.S().Debugf("Creating API key with alias %s and scopes %v", apiKeyRequest.Alias, apiKeyRequest.Scopes)

	//API keys have a torii wide scope, so we need to check if any key already exists with such as alias
	k, err := s.GetApiKey(ctx, apiKeyRequest.Alias)
	if err == nil && k != nil {
		zap.S().Warnf("API key with alias %s already exists, cannot create another one with the same alias", apiKeyRequest.Alias)
		return nil, ErrorDuplicatedAliasApiKeyRequest
	}
	scopeMap := make(map[domain.Scope]byte, len(apiKeyRequest.Scopes))
	for _, scope := range apiKeyRequest.Scopes {
		s, ok := domain.AvailableScopesMap[domain.Scope(scope)]
		if !ok {
			zap.S().Warnf("Invalid scope %s provided for API key creation", scope)
			return nil, ErrorInvalidScopesApiKeyRequest
		}
		scopeMap[domain.Scope(scope)] = s
	}

	rawKey := generateApiKey()
	hashedKey := s.hasher.Hash(rawKey)

	dbKey := &domain.ApiKey{
		Alias:     apiKeyRequest.Alias,
		Key:       hashedKey, // store the HMAC, not the raw key
		Scopes:    scopeMap,
		Expires:   apiKeyRequest.ExpiryDate,
		CreatedAt: time.Now().Unix(),
	}
	err = s.store.NewApiKey(ctx, dbKey)
	if err != nil {
		zap.S().Errorf("Failed to create API key with alias %s: %v", apiKeyRequest.Alias, err)
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	// Return the raw key to the caller — this is the only time it's visible
	returnKey := &domain.ApiKey{
		ID:        dbKey.ID,
		Alias:     apiKeyRequest.Alias,
		Key:       rawKey,
		Scopes:    scopeMap,
		Expires:   apiKeyRequest.ExpiryDate,
		CreatedAt: dbKey.CreatedAt,
	}

	zap.S().Debugf("API key with alias %s created successfully", apiKeyRequest.Alias)
	return returnKey, nil
}

func (s *ApiKeyService) DeleteApiKey(ctx context.Context, alias string) error {
	return s.store.DeleteApiKey(ctx, alias)
}

func (s *ApiKeyService) GetApiKey(ctx context.Context, alias string) (*domain.ApiKey, error) {
	zap.S().Debugf("Fetching API key by alias %s", alias)
	apiKey, err := s.store.GetApiKey(ctx, alias)
	if err != nil {
		zap.S().Warnf("Failed to fetch API key by alias %s: %v", alias, err)
		return nil, err
	}
	if apiKey == nil {
		return nil, nil
	}
	apiKey.Key = "" //we never return the actual API key outside the service (except creation obviously), so we clear it here to avoid any accidental leaks
	return apiKey, nil
}

func (s *ApiKeyService) GetAllApiKeys(ctx context.Context) []*domain.ApiKey {
	zap.S().Debugf("Fetching all API keys")
	apiKeys, err := s.store.GetAllApiKeys(ctx)
	if err != nil {
		zap.S().Warnf("Failed to fetch all API keys: %v", err)
		return nil
	}
	for _, apiKey := range apiKeys {
		apiKey.Key = "" //we never return the actual API key outside the service (except creation obviously), so we clear it here to avoid any accidental leaks
	}
	return apiKeys
}

// warmCache populates the in-memory cache after a successful DB validation.
// The cache is keyed by the HMAC-hashed key so no raw keys are ever held in
// memory. Subsequent requests still compute the HMAC (fast) but skip the DB
// round-trip entirely.
func (s *ApiKeyService) warmCache(hashedKey string) {
	apiKey, err := s.store.GetApiKeyByRawKey(context.Background(), hashedKey)
	if err != nil || apiKey == nil {
		zap.S().Debugf("Cache warm skipped for API key: %v", err)
		return
	}

	scopes := make(map[string]byte, len(apiKey.Scopes))
	for scope := range apiKey.Scopes {
		scopes[string(scope)] = 1
	}

	s.cache.CacheValue(hashedKey, &apiKeyCacheEntry{
		allowedScopes: scopes,
		expiresAt:     apiKey.Expires,
		lastSeen:      time.Now(),
	})
}

func generateApiKey() string {
	zap.S().Debug("Generating API key")
	return rand.Text()
}
