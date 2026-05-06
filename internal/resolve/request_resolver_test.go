package resolve

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestResolverInfoMatchesRegisteredResolvers(t *testing.T) {
	require.NotEmpty(t, requestVarInfo)
	for _, info := range requestVarInfo {
		assert.NotEmpty(t, info.Key)
		assert.NotEmpty(t, info.Description)

		resolver := GetRequestResolver(info.Key)
		require.NotNil(t, resolver, "missing resolver for %s", info.Key)
	}
}

func TestRegisteredRequestResolvers(t *testing.T) {
	req := httptest.NewRequest("PATCH", "https://example.test/path?q=1", nil)
	req.RemoteAddr = "203.0.113.10:41234"
	req.Host = "app.example.test"
	req.RequestURI = "/path?q=1"

	tests := map[string]string{
		"$remote_addr": "203.0.113.10:41234",
		"$host":        "app.example.test",
		"$method":      "PATCH",
		"$uri":         "/path?q=1",
		"$scheme":      "https",
	}

	for key, expected := range tests {
		resolver := GetRequestResolver(key)
		require.NotNil(t, resolver, "missing resolver for %s", key)
		assert.Equal(t, expected, resolver(req))
	}
}
