package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveRecentLogLimit(t *testing.T) {
	t.Run("uses capacity when no limit is requested", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/requests", nil)

		assert.Equal(t, 1000, resolveRecentLogLimit(req, 1000))
	})

	t.Run("uses requested limit below capacity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/requests?limit=200", nil)

		assert.Equal(t, 200, resolveRecentLogLimit(req, 1000))
	})

	t.Run("caps requested limit at capacity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/requests?limit=5000", nil)

		assert.Equal(t, 1000, resolveRecentLogLimit(req, 1000))
	})

	t.Run("falls back for invalid requested limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/requests?limit=nope", nil)

		assert.Equal(t, 1000, resolveRecentLogLimit(req, 1000))
	})

	t.Run("uses default when capacity is unavailable", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/requests", nil)

		assert.Equal(t, 1000, resolveRecentLogLimit(req, 0))
	})
}

func TestResolveRecentLogPage(t *testing.T) {
	t.Run("returns limit and offset inside capacity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/requests?limit=200&offset=400", nil)

		limit, offset := resolveRecentLogPage(req, 1000)

		assert.Equal(t, 200, limit)
		assert.Equal(t, 400, offset)
	})

	t.Run("clips offset and limit to capacity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/requests?limit=200&offset=950", nil)

		limit, offset := resolveRecentLogPage(req, 1000)

		assert.Equal(t, 50, limit)
		assert.Equal(t, 950, offset)
	})
}

func TestSliceRecentLogs(t *testing.T) {
	entries := []int{0, 1, 2, 3, 4}

	assert.Equal(t, []int{2, 3}, sliceRecentLogs(entries, 2, 2))
	assert.Empty(t, sliceRecentLogs(entries, 5, 2))
}
