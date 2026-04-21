package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newApiKeyStore(t *testing.T) *sqlite.DB {
	t.Helper()
	db := openTestDB(t)
	return db
}

// ---------------------------------------------------------------------------
// NewApiKey
// ---------------------------------------------------------------------------

func TestApiKeyStore_NewApiKey_Success(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "test-key",
		Key:       "raw-secret-key",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}

	err := store.NewApiKey(ctx, key)
	require.NoError(t, err)

	fetched, err := store.GetApiKey(ctx, "test-key")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "test-key", fetched.Alias)
	assert.Equal(t, "raw-secret-key", fetched.Key)
	assert.Contains(t, fetched.Scopes, domain.READ_STATS_SCOPE)
}

func TestApiKeyStore_NewApiKey_WithExpiry(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	expiry := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	key := &domain.ApiKey{
		Alias:     "expiring-key",
		Key:       "raw-key-123",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		Expires:   expiry,
		CreatedAt: time.Now().Unix(),
	}

	err := store.NewApiKey(ctx, key)
	require.NoError(t, err)

	fetched, err := store.GetApiKey(ctx, "expiring-key")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, expiry, fetched.Expires.UTC().Truncate(time.Second))
}

func TestApiKeyStore_NewApiKey_WithoutExpiry_ExpiresIsZero(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "no-expiry",
		Key:       "raw-key-456",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}

	err := store.NewApiKey(ctx, key)
	require.NoError(t, err)

	fetched, err := store.GetApiKey(ctx, "no-expiry")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.True(t, fetched.Expires.IsZero())
}

func TestApiKeyStore_NewApiKey_DuplicateAlias_Fails(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "dup",
		Key:       "key-1",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, store.NewApiKey(ctx, key))

	key2 := &domain.ApiKey{
		Alias:     "dup",
		Key:       "key-2",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}
	err := store.NewApiKey(ctx, key2)
	assert.Error(t, err)
}

func TestApiKeyStore_NewApiKey_DuplicateKey_Fails(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "alias-1",
		Key:       "same-raw-key",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, store.NewApiKey(ctx, key))

	key2 := &domain.ApiKey{
		Alias:     "alias-2",
		Key:       "same-raw-key",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}
	err := store.NewApiKey(ctx, key2)
	assert.Error(t, err)
}

func TestApiKeyStore_NewApiKey_MultipleScopes(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias: "multi-scope",
		Key:   "raw-multi",
		Scopes: map[domain.Scope]byte{
			domain.READ_STATS_SCOPE: 1,
		},
		CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, store.NewApiKey(ctx, key))

	fetched, err := store.GetApiKey(ctx, "multi-scope")
	require.NoError(t, err)
	assert.Len(t, fetched.Scopes, 1)
	assert.Contains(t, fetched.Scopes, domain.READ_STATS_SCOPE)
}

func TestApiKeyStore_NewApiKey_NoScopes(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "no-scopes",
		Key:       "raw-no-scopes",
		Scopes:    map[domain.Scope]byte{},
		CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, store.NewApiKey(ctx, key))

	fetched, err := store.GetApiKey(ctx, "no-scopes")
	require.NoError(t, err)
	assert.Empty(t, fetched.Scopes)
}

// ---------------------------------------------------------------------------
// DeleteApiKey
// ---------------------------------------------------------------------------

func TestApiKeyStore_DeleteApiKey_Success(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "to-delete",
		Key:       "raw-delete",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, store.NewApiKey(ctx, key))

	err := store.DeleteApiKey(ctx, "to-delete")
	require.NoError(t, err)

	fetched, err := store.GetApiKey(ctx, "to-delete")
	require.NoError(t, err)
	assert.Nil(t, fetched)
}

func TestApiKeyStore_DeleteApiKey_NonExistent_NoError(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	err := store.DeleteApiKey(ctx, "does-not-exist")
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// GetApiKey
// ---------------------------------------------------------------------------

func TestApiKeyStore_GetApiKey_NotFound(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	fetched, err := store.GetApiKey(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, fetched)
}

// ---------------------------------------------------------------------------
// GetApiKeyByRawKey
// ---------------------------------------------------------------------------

func TestApiKeyStore_GetApiKeyByRawKey_Success(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "by-raw",
		Key:       "the-raw-key",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, store.NewApiKey(ctx, key))

	fetched, err := store.GetApiKeyByRawKey(ctx, "the-raw-key")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "by-raw", fetched.Alias)
	assert.Contains(t, fetched.Scopes, domain.READ_STATS_SCOPE)
}

func TestApiKeyStore_GetApiKeyByRawKey_NotFound(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	fetched, err := store.GetApiKeyByRawKey(ctx, "no-such-key")
	require.NoError(t, err)
	assert.Nil(t, fetched)
}

// ---------------------------------------------------------------------------
// IsKeyValidForScope
// ---------------------------------------------------------------------------

func TestApiKeyStore_IsKeyValidForScope_ValidScope(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "scope-check",
		Key:       "raw-scope",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, store.NewApiKey(ctx, key))

	valid, err := store.IsKeyValidForScope(ctx, "raw-scope", string(domain.READ_STATS_SCOPE))
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestApiKeyStore_IsKeyValidForScope_InvalidScope(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "scope-check2",
		Key:       "raw-scope2",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, store.NewApiKey(ctx, key))

	valid, err := store.IsKeyValidForScope(ctx, "raw-scope2", "nonexistent_scope")
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestApiKeyStore_IsKeyValidForScope_NonExistentKey(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	valid, err := store.IsKeyValidForScope(ctx, "ghost-key", string(domain.READ_STATS_SCOPE))
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestApiKeyStore_IsKeyValidForScope_ExpiredKey(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "expired",
		Key:       "raw-expired",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		Expires:   time.Now().Add(-1 * time.Hour),
		CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, store.NewApiKey(ctx, key))

	valid, err := store.IsKeyValidForScope(ctx, "raw-expired", string(domain.READ_STATS_SCOPE))
	require.NoError(t, err)
	assert.False(t, valid, "expired key should not be valid")
}

func TestApiKeyStore_IsKeyValidForScope_NotExpiredKey(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	key := &domain.ApiKey{
		Alias:     "not-expired",
		Key:       "raw-not-expired",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		Expires:   time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, store.NewApiKey(ctx, key))

	valid, err := store.IsKeyValidForScope(ctx, "raw-not-expired", string(domain.READ_STATS_SCOPE))
	require.NoError(t, err)
	assert.True(t, valid)
}

// ---------------------------------------------------------------------------
// GetAllApiKeys
// ---------------------------------------------------------------------------

func TestApiKeyStore_GetAllApiKeys_Empty(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	keys, err := store.GetAllApiKeys(ctx)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestApiKeyStore_GetAllApiKeys_MultipleKeys(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	for i, alias := range []string{"key-a", "key-b", "key-c"} {
		require.NoError(t, store.NewApiKey(ctx, &domain.ApiKey{
			Alias:     alias,
			Key:       "raw-" + alias,
			Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
			CreatedAt: time.Now().Unix() + int64(i),
		}))
	}

	keys, err := store.GetAllApiKeys(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 3)
}

func TestApiKeyStore_GetAllApiKeys_ReturnsExpiryField(t *testing.T) {
	db := newApiKeyStore(t)
	store := sqlite.NewApiKeyStore(db)
	ctx := context.Background()

	expiry := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, store.NewApiKey(ctx, &domain.ApiKey{
		Alias:     "with-exp",
		Key:       "raw-exp",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		Expires:   expiry,
		CreatedAt: time.Now().Unix(),
	}))
	require.NoError(t, store.NewApiKey(ctx, &domain.ApiKey{
		Alias:     "without-exp",
		Key:       "raw-no-exp",
		Scopes:    map[domain.Scope]byte{domain.READ_STATS_SCOPE: 1},
		CreatedAt: time.Now().Unix(),
	}))

	keys, err := store.GetAllApiKeys(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 2)

	var withExpiry, withoutExpiry *domain.ApiKey
	for _, k := range keys {
		if k.Alias == "with-exp" {
			withExpiry = k
		} else {
			withoutExpiry = k
		}
	}
	assert.Equal(t, expiry, withExpiry.Expires.UTC().Truncate(time.Second))
	assert.True(t, withoutExpiry.Expires.IsZero())
}
