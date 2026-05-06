package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/nunoOliveiraqwe/torii/internal/resolve"
	"github.com/nunoOliveiraqwe/torii/middleware/ctx"
	"go.uber.org/zap"
)

const (
	setHeadersReqKey   = "set-headers-req"
	setHeadersResKey   = "set-headers-res"
	stripHeadersReqKey = "strip-headers-req"
	stripHeadersResKey = "strip-headers-res"
	cmpHeadersReqKey   = "cmp-headers-req"
)

type headerRule interface {
	applyRequest(r *http.Request) bool
	applyResponse(w http.ResponseWriter)
}

type headersConfig struct {
	rules []headerRule
}

type headersWriter struct {
	w             http.ResponseWriter
	headerWritten bool
	conf          *headersConfig
}

func (hw *headersWriter) Header() http.Header {
	return hw.w.Header()
}

func (hw *headersWriter) flushResponseHeaders() {
	if hw.headerWritten {
		return
	}
	hw.headerWritten = true

	for _, rule := range hw.conf.rules {
		rule.applyResponse(hw.w)
	}
}

func (hw *headersWriter) Write(b []byte) (int, error) {
	hw.flushResponseHeaders()
	return hw.w.Write(b)
}

func (hw *headersWriter) WriteHeader(code int) {
	hw.flushResponseHeaders()
	hw.w.WriteHeader(code)
}

func (hw *headersWriter) Flush() {
	hw.flushResponseHeaders()
	if f, ok := hw.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (hw *headersWriter) Unwrap() http.ResponseWriter {
	return hw.w
}

type setRequestHeaderRule struct {
	header   string
	resolver func(r *http.Request) string
}

func (rul *setRequestHeaderRule) applyRequest(r *http.Request) bool {
	resolvedVal := rul.resolver(r)

	logger := GetRequestLoggerFromContext(r)
	logger.Debug("Applying request header",
		zap.String("header", rul.header),
	)

	r.Header.Set(rul.header, resolvedVal)
	return true
}

func (rul *setRequestHeaderRule) applyResponse(w http.ResponseWriter) {}

type compareRequestHeaderRule struct {
	header   string
	resolver func(r *http.Request) string
}

func (rul *compareRequestHeaderRule) applyRequest(r *http.Request) bool {
	expected := rul.resolver(r)
	actual := r.Header.Get(rul.header)

	if actual != expected {
		logger := GetRequestLoggerFromContext(r)
		logger.Debug("Header value does not match expected value",
			zap.String("header", rul.header),
		)

		ctx.CreateAndAddBlockInfo(r, "headers", fmt.Sprintf("header-%s-mismatch-value", rul.header))
		return false
	}

	return true
}

func (rul *compareRequestHeaderRule) applyResponse(w http.ResponseWriter) {}

type setResponseHeaderRule struct {
	header string
	value  string
}

func (rul *setResponseHeaderRule) applyRequest(r *http.Request) bool {
	return true
}

func (rul *setResponseHeaderRule) applyResponse(w http.ResponseWriter) {
	w.Header().Set(rul.header, rul.value)
}

type stripRequestHeaderRule struct {
	header string
}

func (rul *stripRequestHeaderRule) applyRequest(r *http.Request) bool {
	r.Header.Del(rul.header)
	return true
}

func (rul *stripRequestHeaderRule) applyResponse(w http.ResponseWriter) {}

type stripResponseHeaderRule struct {
	header string
}

func (rul *stripResponseHeaderRule) applyRequest(r *http.Request) bool {
	return true
}

func (rul *stripResponseHeaderRule) applyResponse(w http.ResponseWriter) {
	w.Header()[http.CanonicalHeaderKey(rul.header)] = nil
}

func compileValueResolver(raw string) (func(r *http.Request) string, error) {
	if raw == "" {
		return nil, fmt.Errorf("empty header value")
	}

	if !strings.HasPrefix(raw, "$") {
		val := raw
		return func(r *http.Request) string {
			return val
		}, nil
	}

	if len(raw) == 1 {
		return nil, fmt.Errorf("header value cannot be just $")
	}

	if reqResolver := resolve.GetRequestResolver(raw); reqResolver != nil {
		return reqResolver, nil
	}

	return compileStaticValueResolver(raw)
}

func compileStaticValueResolver(raw string) (func(r *http.Request) string, error) {
	if raw == "" {
		return nil, fmt.Errorf("empty header value")
	}

	if !strings.HasPrefix(raw, "$") {
		val := raw
		return func(r *http.Request) string {
			return val
		}, nil
	}

	if len(raw) == 1 {
		return nil, fmt.Errorf("header value cannot be just $")
	}

	if !strings.Contains(raw, ":") {
		return nil, fmt.Errorf("unknown request resolver or invalid value resolver syntax: %s", raw)
	}

	val, err := resolve.ResolveValue(raw)
	if err != nil {
		return nil, err
	}

	return func(r *http.Request) string {
		return val
	}, nil
}

func HeadersMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	h := parseConfiguration(conf)
	if h == nil {
		zap.S().Error("HeadersMiddleware: failed to initialize header middleware. Failing closed.")
		return func(w http.ResponseWriter, request *http.Request) {
			http.Error(w, "HeadersMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	if len(h.rules) == 0 {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		for _, rule := range h.rules {
			if !rule.applyRequest(r) {
				GetRequestLoggerFromContext(r).Warn("Request headers do not match expected values")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		hw := &headersWriter{
			w:    w,
			conf: h,
		}

		defer hw.flushResponseHeaders()
		next(hw, r)
	}
}

func parseConfiguration(conf Config) *headersConfig {
	zap.S().Debugf("Parsing configuration for headers middleware: %+v", conf)

	if conf.Options == nil {
		zap.S().Error("Options for headers middleware is nil")
		return nil
	}

	h := &headersConfig{
		rules: make([]headerRule, 0),
	}
	var parseErr error

	parseErr = parseConfigMap(conf, setHeadersReqKey, func(k, v string) error {
		resolver, err := compileValueResolver(v)
		if err != nil {
			return fmt.Errorf("request set header %s: %w", k, err)
		}

		h.rules = append(h.rules, &setRequestHeaderRule{
			header:   k,
			resolver: resolver,
		})
		return nil
	})
	if parseErr != nil {
		zap.S().Errorf("Invalid headers middleware configuration: %v", parseErr)
		return nil
	}

	parseErr = parseConfigMap(conf, cmpHeadersReqKey, func(k, v string) error {
		resolver, err := compileValueResolver(v)
		if err != nil {
			return fmt.Errorf("request compare header %s: %w", k, err)
		}

		h.rules = append(h.rules, &compareRequestHeaderRule{
			header:   k,
			resolver: resolver,
		})
		return nil
	})
	if parseErr != nil {
		zap.S().Errorf("Invalid headers middleware configuration: %v", parseErr)
		return nil
	}

	parseErr = parseConfigMap(conf, setHeadersResKey, func(k, v string) error {
		resolver, err := compileStaticValueResolver(v)
		if err != nil {
			return fmt.Errorf("response set header %s: %w", k, err)
		}

		h.rules = append(h.rules, &setResponseHeaderRule{
			header: k,
			value:  resolver(nil),
		})
		return nil
	})
	if parseErr != nil {
		zap.S().Errorf("Invalid headers middleware configuration: %v", parseErr)
		return nil
	}

	parseErr = parseConfigSlice(conf, stripHeadersReqKey, func(v string) error {
		h.rules = append(h.rules, &stripRequestHeaderRule{
			header: http.CanonicalHeaderKey(v),
		})
		return nil
	})
	if parseErr != nil {
		zap.S().Errorf("Invalid headers middleware configuration: %v", parseErr)
		return nil
	}

	parseErr = parseConfigSlice(conf, stripHeadersResKey, func(v string) error {
		h.rules = append(h.rules, &stripResponseHeaderRule{
			header: http.CanonicalHeaderKey(v),
		})
		return nil
	})
	if parseErr != nil {
		zap.S().Errorf("Invalid headers middleware configuration: %v", parseErr)
		return nil
	}

	return h
}

func parseConfigMap(conf Config, key string, setFunc func(key, val string) error) error {
	rawMap, exists := conf.Options[key]
	if !exists {
		return nil
	}

	castedMap, ok := rawMap.(map[string]interface{})
	if !ok {
		return fmt.Errorf("%s must be a map[string]interface{}", key)
	}

	for rawKey, rawVal := range castedMap {
		castedVal, ok := rawVal.(string)
		if !ok {
			return fmt.Errorf("%s header %s value must be a string", key, rawKey)
		}

		canonicalKey := http.CanonicalHeaderKey(rawKey)
		if canonicalKey == "" {
			return fmt.Errorf("%s header key cannot be empty", key)
		}

		if err := setFunc(canonicalKey, castedVal); err != nil {
			return err
		}
	}

	return nil
}

func parseConfigSlice(conf Config, key string, setFunc func(val string) error) error {
	rawSlice, exists := conf.Options[key]
	if !exists {
		return nil
	}

	castedSlice, ok := rawSlice.([]interface{})
	if !ok {
		return fmt.Errorf("%s must be a []interface{}", key)
	}

	for _, rawVal := range castedSlice {
		castedVal, ok := rawVal.(string)
		if !ok {
			return fmt.Errorf("%s header value must be a string", key)
		}

		if http.CanonicalHeaderKey(castedVal) == "" {
			return fmt.Errorf("%s header value cannot be empty", key)
		}

		if err := setFunc(castedVal); err != nil {
			return err
		}
	}

	return nil
}
