package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// computeRetryAfter
// ---------------------------------------------------------------------------

func TestComputeRetryAfter_PositiveRate(t *testing.T) {
	// 1 req/s → ceil(1/1) = 1
	assert.Equal(t, "1", computeRetryAfter(1.0))
}

func TestComputeRetryAfter_FractionalRate(t *testing.T) {
	// 0.5 req/s → ceil(1/0.5) = 2
	assert.Equal(t, "2", computeRetryAfter(0.5))
}

func TestComputeRetryAfter_HighRate(t *testing.T) {
	// 100 req/s → ceil(1/100) = 1 (minimum)
	assert.Equal(t, "1", computeRetryAfter(100.0))
}

func TestComputeRetryAfter_ZeroRate(t *testing.T) {
	assert.Equal(t, "1", computeRetryAfter(0))
}

func TestComputeRetryAfter_NegativeRate(t *testing.T) {
	assert.Equal(t, "1", computeRetryAfter(-5))
}

func TestComputeRetryAfter_VerySmallRate(t *testing.T) {
	// 0.1 req/s → ceil(1/0.1) = 10
	assert.Equal(t, "10", computeRetryAfter(0.1))
}

// ---------------------------------------------------------------------------
// parseIntOption
// ---------------------------------------------------------------------------

func TestParseIntOption_Float64(t *testing.T) {
	m := map[string]interface{}{"burst": float64(10)}
	v, err := parseIntOption(m, "burst")
	require.NoError(t, err)
	assert.Equal(t, 10, v)
}

func TestParseIntOption_Int(t *testing.T) {
	m := map[string]interface{}{"burst": 5}
	v, err := parseIntOption(m, "burst")
	require.NoError(t, err)
	assert.Equal(t, 5, v)
}

func TestParseIntOption_Missing(t *testing.T) {
	m := map[string]interface{}{}
	_, err := parseIntOption(m, "burst")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required option")
}

func TestParseIntOption_WrongType(t *testing.T) {
	m := map[string]interface{}{"burst": "not-a-number"}
	_, err := parseIntOption(m, "burst")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a number")
}

// ---------------------------------------------------------------------------
// parseFloatOption
// ---------------------------------------------------------------------------

func TestParseFloatOption_Float64(t *testing.T) {
	m := map[string]interface{}{"rate-per-second": float64(2.5)}
	v, err := parseFloatOption(m, "rate-per-second")
	require.NoError(t, err)
	assert.Equal(t, 2.5, v)
}

func TestParseFloatOption_Int(t *testing.T) {
	m := map[string]interface{}{"rate-per-second": 3}
	v, err := parseFloatOption(m, "rate-per-second")
	require.NoError(t, err)
	assert.Equal(t, 3.0, v)
}

func TestParseFloatOption_Missing(t *testing.T) {
	m := map[string]interface{}{}
	_, err := parseFloatOption(m, "rate-per-second")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required option")
}

func TestParseFloatOption_WrongType(t *testing.T) {
	m := map[string]interface{}{"rate-per-second": "bad"}
	_, err := parseFloatOption(m, "rate-per-second")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a number")
}

// ---------------------------------------------------------------------------
// parseConfig
// ---------------------------------------------------------------------------

func validGlobalConfig() Config {
	return Config{
		Type: "RateLimiter",
		Options: map[string]interface{}{
			"mode": "global",
			"limiter-req": map[string]interface{}{
				"burst":           float64(5),
				"rate-per-second": float64(10),
			},
		},
	}
}

func validPerClientConfig() Config {
	return Config{
		Type: "RateLimiter",
		Options: map[string]interface{}{
			"mode": "per-client",
			"limiter-req": map[string]interface{}{
				"burst":           float64(3),
				"rate-per-second": float64(5),
			},
			"cache-ttl":        "1h",
			"cleanup-interval": "30m",
			"max-cache-size":   100000,
		},
	}
}

func TestParseConfig_ValidGlobal(t *testing.T) {
	conf, err := parseConfig(validGlobalConfig())
	require.NoError(t, err)
	assert.Equal(t, 5, conf.Burst)
	assert.Equal(t, 10.0, conf.RatePerSecond)
	assert.Equal(t, "global", conf.Mode)
	assert.Nil(t, conf.CacheOpt)
}

func TestParseConfig_ValidPerClient(t *testing.T) {
	conf, err := parseConfig(validPerClientConfig())
	require.NoError(t, err)
	assert.Equal(t, 3, conf.Burst)
	assert.Equal(t, 5.0, conf.RatePerSecond)
	assert.Equal(t, "per-client", conf.Mode)
	assert.NotNil(t, conf.CacheOpt)
}

func TestParseConfig_MissingLimiterReq(t *testing.T) {
	conf := Config{Options: map[string]interface{}{"mode": "global"}}
	_, err := parseConfig(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "limiter-req")
}

func TestParseConfig_LimiterReqBadType(t *testing.T) {
	conf := Config{Options: map[string]interface{}{
		"mode":        "global",
		"limiter-req": "not-a-map",
	}}
	_, err := parseConfig(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "map")
}

func TestParseConfig_MissingBurst(t *testing.T) {
	conf := Config{Options: map[string]interface{}{
		"mode": "global",
		"limiter-req": map[string]interface{}{
			"rate-per-second": float64(1),
		},
	}}
	_, err := parseConfig(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "burst")
}

func TestParseConfig_ZeroBurst(t *testing.T) {
	conf := Config{Options: map[string]interface{}{
		"mode": "global",
		"limiter-req": map[string]interface{}{
			"burst":           float64(0),
			"rate-per-second": float64(1),
		},
	}}
	_, err := parseConfig(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "positive integer")
}

func TestParseConfig_NegativeBurst(t *testing.T) {
	conf := Config{Options: map[string]interface{}{
		"mode": "global",
		"limiter-req": map[string]interface{}{
			"burst":           float64(-1),
			"rate-per-second": float64(1),
		},
	}}
	_, err := parseConfig(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "positive integer")
}

func TestParseConfig_MissingRatePerSecond(t *testing.T) {
	conf := Config{Options: map[string]interface{}{
		"mode": "global",
		"limiter-req": map[string]interface{}{
			"burst": float64(5),
		},
	}}
	_, err := parseConfig(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate-per-second")
}

func TestParseConfig_ZeroRatePerSecond(t *testing.T) {
	conf := Config{Options: map[string]interface{}{
		"mode": "global",
		"limiter-req": map[string]interface{}{
			"burst":           float64(5),
			"rate-per-second": float64(0),
		},
	}}
	_, err := parseConfig(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "positive number")
}

func TestParseConfig_NegativeRatePerSecond(t *testing.T) {
	conf := Config{Options: map[string]interface{}{
		"mode": "global",
		"limiter-req": map[string]interface{}{
			"burst":           float64(5),
			"rate-per-second": float64(-2),
		},
	}}
	_, err := parseConfig(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "positive number")
}

func TestParseConfig_InvalidMode(t *testing.T) {
	conf := Config{Options: map[string]interface{}{
		"mode": "invalid-mode",
		"limiter-req": map[string]interface{}{
			"burst":           float64(5),
			"rate-per-second": float64(1),
		},
	}}
	_, err := parseConfig(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid 'mode'")
}

func TestParseConfig_ModeNotString(t *testing.T) {
	conf := Config{Options: map[string]interface{}{
		"mode": 123,
		"limiter-req": map[string]interface{}{
			"burst":           float64(5),
			"rate-per-second": float64(1),
		},
	}}
	_, err := parseConfig(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "string")
}

func TestParseConfig_DefaultModeIsGlobal(t *testing.T) {
	conf := Config{Options: map[string]interface{}{
		"limiter-req": map[string]interface{}{
			"burst":           float64(5),
			"rate-per-second": float64(1),
		},
	}}
	result, err := parseConfig(conf)
	require.NoError(t, err)
	assert.Equal(t, "global", result.Mode)
}

// ---------------------------------------------------------------------------
// newLimiter
// ---------------------------------------------------------------------------

func TestNewLimiter_Global(t *testing.T) {
	l, err := newLimiter(&rateLimitConf{
		Burst:         5,
		RatePerSecond: 10,
		Mode:          "global",
	})
	require.NoError(t, err)
	assert.IsType(t, &globalLimiter{}, l)
}

func TestNewLimiter_PerClient(t *testing.T) {
	l, err := newLimiter(&rateLimitConf{
		Burst:         3,
		RatePerSecond: 5,
		Mode:          "per-client",
		CacheOpt: &util.CacheOptions{
			MaxEntries:      1000,
			TTL:             time.Hour,
			CleanupInterval: 30 * time.Minute,
		},
	})
	require.NoError(t, err)
	assert.IsType(t, &perClientLimiter{}, l)
}

// ---------------------------------------------------------------------------
// globalLimiter.limit
// ---------------------------------------------------------------------------

func TestGlobalLimiter_AllowsUpToBurst(t *testing.T) {
	l := &globalLimiter{
		internalLimiter: rate.NewLimiter(rate.Limit(5), 5),
		retryAfter:      "1",
	}

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		assert.True(t, l.limit(req, rec), "request %d should be allowed", i)
	}
}

func TestGlobalLimiter_RejectsOverBurst(t *testing.T) {
	// burst=2, rate very low so no tokens refill during test
	l := &globalLimiter{
		internalLimiter: rate.NewLimiter(rate.Limit(0.001), 2),
		retryAfter:      "1000",
	}

	// drain burst
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		require.True(t, l.limit(req, rec))
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	assert.False(t, l.limit(req, rec))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "1000", rec.Header().Get("Retry-After"))
}

// ---------------------------------------------------------------------------
// perClientLimiter.limit
// ---------------------------------------------------------------------------

func TestPerClientLimiter_AllowsUpToBurst(t *testing.T) {
	l := newTestPerClientLimiter(5, 5)
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		assert.True(t, l.limit(req, rec), "request %d should be allowed", i)
	}
}

func TestPerClientLimiter_RejectsOverBurst(t *testing.T) {
	l := newTestPerClientLimiter(0.001, 2)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		require.True(t, l.limit(req, rec))
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	assert.False(t, l.limit(req, rec))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestPerClientLimiter_IndependentClients(t *testing.T) {
	l := newTestPerClientLimiter(0.001, 1)

	// Client A exhausts its burst
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	require.True(t, l.limit(req, rec))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	assert.False(t, l.limit(req, rec))

	// Client B should still be allowed
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	assert.True(t, l.limit(req, rec))
}

func TestPerClientLimiter_BadRemoteAddr(t *testing.T) {
	l := newTestPerClientLimiter(10, 10)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "bad-addr" // no port → GetClientIP fails
	assert.False(t, l.limit(req, rec))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

// ---------------------------------------------------------------------------
// RateLimitMiddleware (integration-level)
// ---------------------------------------------------------------------------

func TestRateLimitMiddleware_GlobalAllowsRequests(t *testing.T) {
	called := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitMiddleware(context.Background(), next, validGlobalConfig())

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Equal(t, 5, called)
}

func TestRateLimitMiddleware_GlobalRejectsExcess(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	conf := Config{
		Type: "RateLimiter",
		Options: map[string]interface{}{
			"mode": "global",
			"limiter-req": map[string]interface{}{
				"burst":           float64(1),
				"rate-per-second": float64(0.001), // very slow refill
			},
		},
	}
	handler := RateLimitMiddleware(context.Background(), next, conf)

	// First request succeeds
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second request rejected
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestRateLimitMiddleware_PerClientAllowsRequests(t *testing.T) {
	called := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitMiddleware(context.Background(), next, validPerClientConfig())

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Equal(t, 3, called)
}

func TestRateLimitMiddleware_PerClientRejectsExcess(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	conf := Config{
		Type: "RateLimiter",
		Options: map[string]interface{}{
			"mode": "per-client",
			"limiter-req": map[string]interface{}{
				"burst":           float64(1),
				"rate-per-second": float64(0.001),
			},
			"cache-ttl":        "1h",
			"cleanup-interval": "1h",
		},
	}
	handler := RateLimitMiddleware(context.Background(), next, conf)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestRateLimitMiddleware_MisconfigReturnServiceUnavailable(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Missing limiter-req entirely → misconfig
	conf := Config{
		Type:    "RateLimiter",
		Options: map[string]interface{}{},
	}
	handler := RateLimitMiddleware(context.Background(), next, conf)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestPerClientLimiter(ratePerSec float64, burst int) *perClientLimiter {
	cache, _ := util.NewCache[*clientEntry](&util.CacheOptions{
		MaxEntries:      10000,
		TTL:             time.Hour,
		CleanupInterval: time.Hour,
	})
	return &perClientLimiter{
		clientCache:   cache,
		ratePerSecond: ratePerSec,
		burst:         burst,
		retryAfter:    computeRetryAfter(ratePerSec),
	}
}
