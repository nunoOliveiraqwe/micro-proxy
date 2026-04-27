package service

import (
	"context"
	"testing"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/auth"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var testHmacSecret = []byte("test-hmac-secret-32-bytes-long!!")

func init() {
	logger := zap.NewNop()
	zap.ReplaceGlobals(logger)
}

// ---------------------------------------------------------------------------
// Mock ApiKeyStore
// ---------------------------------------------------------------------------

type mockApiKeyStore struct {
	mock.Mock
}

var _ store.ApiKeyStore = (*mockApiKeyStore)(nil)

func (m *mockApiKeyStore) NewApiKey(ctx context.Context, key *domain.ApiKey) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}
func (m *mockApiKeyStore) DeleteApiKey(ctx context.Context, alias string) error {
	args := m.Called(ctx, alias)
	return args.Error(0)
}
func (m *mockApiKeyStore) IsKeyValidForScope(ctx context.Context, key string, scope string) (bool, error) {
	args := m.Called(ctx, key, scope)
	return args.Bool(0), args.Error(1)
}
func (m *mockApiKeyStore) GetApiKey(ctx context.Context, alias string) (*domain.ApiKey, error) {
	args := m.Called(ctx, alias)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ApiKey), args.Error(1)
}
func (m *mockApiKeyStore) GetApiKeyByRawKey(ctx context.Context, key string) (*domain.ApiKey, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ApiKey), args.Error(1)
}
func (m *mockApiKeyStore) GetAllApiKeys(ctx context.Context) ([]*domain.ApiKey, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.ApiKey), args.Error(1)
}

// ---------------------------------------------------------------------------
// CreateApiKey – validation
// ---------------------------------------------------------------------------

func TestCreateApiKey_NilRequest(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	_, err := svc.CreateApiKey(context.Background(), nil)
	assert.ErrorIs(t, err, ErrorInvalidApiKeyRequest)
}

func TestCreateApiKey_EmptyAlias(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	_, err := svc.CreateApiKey(context.Background(), &CreateApiKeyRequest{
		Alias:  "",
		Scopes: []string{"read_stats"},
	})
	assert.ErrorIs(t, err, ErrorInvalidAliasApiKeyRequest)
}

func TestCreateApiKey_NoScopes(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	_, err := svc.CreateApiKey(context.Background(), &CreateApiKeyRequest{
		Alias:  "my-key",
		Scopes: nil,
	})
	assert.ErrorIs(t, err, ErrorInvalidScopesApiKeyRequest)
}

func TestCreateApiKey_EmptyScopes(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	_, err := svc.CreateApiKey(context.Background(), &CreateApiKeyRequest{
		Alias:  "my-key",
		Scopes: []string{},
	})
	assert.ErrorIs(t, err, ErrorInvalidScopesApiKeyRequest)
}

func TestCreateApiKey_InvalidScope(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	// GetApiKey should return nil (no existing key)
	ms.On("GetApiKey", mock.Anything, "my-key").Return(nil, nil)

	_, err := svc.CreateApiKey(context.Background(), &CreateApiKeyRequest{
		Alias:  "my-key",
		Scopes: []string{"nonexistent_scope"},
	})
	assert.ErrorIs(t, err, ErrorInvalidScopesApiKeyRequest)
}

func TestCreateApiKey_ExpiredExpiryDate(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	_, err := svc.CreateApiKey(context.Background(), &CreateApiKeyRequest{
		Alias:      "my-key",
		Scopes:     []string{"read_stats"},
		ExpiryDate: time.Now().Add(-1 * time.Hour),
	})
	assert.ErrorIs(t, err, ErrorInvalidExpiryDateApiKeyRequest)
}

func TestCreateApiKey_DuplicateAlias(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	ms.On("GetApiKey", mock.Anything, "existing").Return(&domain.ApiKey{
		Alias: "existing",
	}, nil)

	_, err := svc.CreateApiKey(context.Background(), &CreateApiKeyRequest{
		Alias:  "existing",
		Scopes: []string{"read_stats"},
	})
	assert.ErrorIs(t, err, ErrorDuplicatedAliasApiKeyRequest)
}

func TestCreateApiKey_Success(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	ms.On("GetApiKey", mock.Anything, "new-key").Return(nil, nil)
	ms.On("NewApiKey", mock.Anything, mock.AnythingOfType("*domain.ApiKey")).Return(nil)

	apiKey, err := svc.CreateApiKey(context.Background(), &CreateApiKeyRequest{
		Alias:  "new-key",
		Scopes: []string{"read_stats"},
	})
	require.NoError(t, err)
	require.NotNil(t, apiKey)
	assert.Equal(t, "new-key", apiKey.Alias)
	assert.NotEmpty(t, apiKey.Key, "generated key should not be empty")
	assert.True(t, apiKey.CreatedAt > 0)

	// Verify the key stored in the DB is the HMAC, not the raw key
	storedCall := ms.Calls[1] // NewApiKey call
	storedKey := storedCall.Arguments.Get(1).(*domain.ApiKey)
	hasher := auth.NewHMACHasher(testHmacSecret)
	assert.Equal(t, hasher.Hash(apiKey.Key), storedKey.Key, "DB should store HMAC, not raw key")
}

func TestCreateApiKey_WithFutureExpiry_Success(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	expiry := time.Now().Add(48 * time.Hour)
	ms.On("GetApiKey", mock.Anything, "expiring").Return(nil, nil)
	ms.On("NewApiKey", mock.Anything, mock.AnythingOfType("*domain.ApiKey")).Return(nil)

	apiKey, err := svc.CreateApiKey(context.Background(), &CreateApiKeyRequest{
		Alias:      "expiring",
		Scopes:     []string{"read_stats"},
		ExpiryDate: expiry,
	})
	require.NoError(t, err)
	assert.Equal(t, expiry, apiKey.Expires)
}

// ---------------------------------------------------------------------------
// GetApiKey – key is redacted
// ---------------------------------------------------------------------------

func TestGetApiKey_RedactsRawKey(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	ms.On("GetApiKey", mock.Anything, "my-key").Return(&domain.ApiKey{
		Alias: "my-key",
		Key:   "super-secret-raw-key",
	}, nil)

	apiKey, err := svc.GetApiKey(context.Background(), "my-key")
	require.NoError(t, err)
	assert.Equal(t, "", apiKey.Key, "raw key must be redacted")
}

// ---------------------------------------------------------------------------
// GetAllApiKeys – keys are redacted
// ---------------------------------------------------------------------------

func TestGetAllApiKeys_RedactsRawKeys(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	ms.On("GetAllApiKeys", mock.Anything).Return([]*domain.ApiKey{
		{Alias: "a", Key: "secret-a"},
		{Alias: "b", Key: "secret-b"},
	}, nil)

	keys := svc.GetAllApiKeys(context.Background())
	require.Len(t, keys, 2)
	for _, k := range keys {
		assert.Empty(t, k.Key, "raw key must be redacted for alias %s", k.Alias)
	}
}

func TestGetAllApiKeys_StoreError_ReturnsNil(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	ms.On("GetAllApiKeys", mock.Anything).Return(nil, assert.AnError)

	keys := svc.GetAllApiKeys(context.Background())
	assert.Nil(t, keys)
}

// ---------------------------------------------------------------------------
// DeleteApiKey
// ---------------------------------------------------------------------------

func TestDeleteApiKey_DelegatesToStore(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)

	ms.On("DeleteApiKey", mock.Anything, "victim").Return(nil)

	err := svc.DeleteApiKey(context.Background(), "victim")
	assert.NoError(t, err)
	ms.AssertCalled(t, "DeleteApiKey", mock.Anything, "victim")
}

// ---------------------------------------------------------------------------
// IsKeyValidForScope – cache and expiry
// ---------------------------------------------------------------------------

func TestIsKeyValidForScope_ValidKey_CacheMiss(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)
	hasher := auth.NewHMACHasher(testHmacSecret)
	hashed := hasher.Hash("raw-key")

	ms.On("IsKeyValidForScope", mock.Anything, hashed, "read_stats").Return(true, nil)
	ms.On("GetApiKeyByRawKey", mock.Anything, hashed).Return(&domain.ApiKey{
		Alias:  "test",
		Key:    hashed,
		Scopes: map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
	}, nil)

	valid, err := svc.IsKeyValidForScope("raw-key", "read_stats")
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestIsKeyValidForScope_InvalidKey(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)
	hasher := auth.NewHMACHasher(testHmacSecret)
	hashed := hasher.Hash("bad-key")

	ms.On("IsKeyValidForScope", mock.Anything, hashed, "read_stats").Return(false, nil)

	valid, err := svc.IsKeyValidForScope("bad-key", "read_stats")
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestIsKeyValidForScope_CacheHit(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)
	hasher := auth.NewHMACHasher(testHmacSecret)
	hashed := hasher.Hash("raw-key")

	// First call: cache miss → store hit → warm cache
	ms.On("IsKeyValidForScope", mock.Anything, hashed, "read_stats").Return(true, nil).Once()
	ms.On("GetApiKeyByRawKey", mock.Anything, hashed).Return(&domain.ApiKey{
		Alias:  "test",
		Key:    hashed,
		Scopes: map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
	}, nil).Once()

	valid, _ := svc.IsKeyValidForScope("raw-key", "read_stats")
	assert.True(t, valid)

	// Second call should hit cache — no new store calls
	valid, err := svc.IsKeyValidForScope("raw-key", "read_stats")
	require.NoError(t, err)
	assert.True(t, valid)

	// store.IsKeyValidForScope should have been called only once
	ms.AssertNumberOfCalls(t, "IsKeyValidForScope", 1)
}

func TestIsKeyValidForScope_CachedExpiredKey_ReturnsFalse(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)
	hasher := auth.NewHMACHasher(testHmacSecret)
	hashed := hasher.Hash("raw-key")

	// First call: cache miss → store says valid → warm cache with expired key
	ms.On("IsKeyValidForScope", mock.Anything, hashed, "read_stats").Return(true, nil).Once()
	ms.On("GetApiKeyByRawKey", mock.Anything, hashed).Return(&domain.ApiKey{
		Alias:   "test",
		Key:     hashed,
		Scopes:  map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		Expires: time.Now().Add(-1 * time.Minute), // already expired
	}, nil).Once()

	// First call succeeds (store says valid before cache is warm)
	valid, _ := svc.IsKeyValidForScope("raw-key", "read_stats")
	assert.True(t, valid)

	// Second call hits cache, discovers expired → evicts, returns false
	valid, err := svc.IsKeyValidForScope("raw-key", "read_stats")
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestIsKeyValidForScope_WrongScope(t *testing.T) {
	ms := new(mockApiKeyStore)
	svc := NewApiKeyService(ms, testHmacSecret)
	hasher := auth.NewHMACHasher(testHmacSecret)
	hashed := hasher.Hash("raw-key")

	ms.On("IsKeyValidForScope", mock.Anything, hashed, "nonexistent_scope").Return(false, nil)

	valid, err := svc.IsKeyValidForScope("raw-key", "nonexistent_scope")
	require.NoError(t, err)
	assert.False(t, valid)
}
