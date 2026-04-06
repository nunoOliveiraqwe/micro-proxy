package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

func buildHttpRevProxy(backend string) (*httputil.ReverseProxy, error) {
	zap.S().Infof("Building proxy for HTTP server with target URL: %s", backend)
	parsedUrl, err := url.Parse(backend)
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: rewriteProxyRequest(parsedUrl),
	}
	return proxy, nil
}

func rewriteProxyRequest(proxyUrl *url.URL) func(r *httputil.ProxyRequest) {
	return func(r *httputil.ProxyRequest) {
		r.SetURL(proxyUrl)
		r.SetXForwarded()
		r.Out.Header.Set("X-Origin-Host", proxyUrl.Host)
		logger := middleware.GetRequestLoggerFromContext(r.In)
		logger.Debug("Rewriting request to target", zap.String("target", proxyUrl.String()), zap.String("x-forwarded-for", r.Out.Header.Get("X-Forwarded-For")))
	}
}

func buildDefaultHttpHandler(proxy *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Proxying request to target", zap.String("target", r.URL.String()), zap.String("x-forwarded-for", r.Header.Get("X-Forwarded-For")))
		proxy.ServeHTTP(w, r)
	}
}
