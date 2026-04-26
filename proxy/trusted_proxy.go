package proxy

import (
	"context"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/trustedproxy"
	_ "github.com/nunoOliveiraqwe/torii/internal/trustedproxy/cloudflare" // register preset
	"go.uber.org/zap"
)

// wrapTrustedProxies wraps handler with a trusted-proxy resolver if cfg is
// non-nil and has a preset and/or at least one range.  The resolver rewrites
// r.RemoteAddr to the real client IP before the middleware chain runs, so
// every downstream call to netutil.GetClientIP transparently sees the correct
// address.
//
// The ctx controls the lifecycle of any background goroutines (e.g. preset
// CIDR refresh).  Cancelling ctx stops them.
func wrapTrustedProxies(ctx context.Context, handler http.HandlerFunc, cfg *config.TrustedProxiesConfig) http.HandlerFunc {
	if cfg == nil || (cfg.Preset == "" && len(cfg.Ranges) == 0) {
		return handler
	}

	resolver, err := trustedproxy.NewTrustedProxyResolverFromPreset(ctx, cfg.Preset, cfg.Ranges, cfg.Header, cfg.RefreshInterval)
	if err != nil {
		zap.S().Errorf("trusted-proxy: failed to initialise resolver: %v — requests will use raw RemoteAddr", err)
		return handler
	}

	return resolver.WrapHandler(handler)
}
