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

const setHeadersReqKey = "set-headers-req"
const setHeadersResKey = "set-headers-res"

const stripHeadersReqKey = "strip-headers-req"
const stripHeadersResKey = "strip-headers-res"

const cmpHeadersReqKey = "cmp-headers-req"

type headersConfig struct {
	setHeadersReq   map[string]string
	setHeadersRes   map[string]string
	stripHeadersReq map[string]bool
	stripHeadersRes map[string]bool
	cmpHeadersReq   map[string]string
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
	if !hw.headerWritten {
		hw.headerWritten = true
		hw.conf.stripHeadersFromResponse(hw.w)
	}
}

func (hw *headersWriter) Write(bytes []byte) (int, error) {
	hw.flushResponseHeaders()
	return hw.w.Write(bytes)
}

func (hw *headersWriter) WriteHeader(code int) {
	hw.flushResponseHeaders()
	hw.w.WriteHeader(code)
}

func (hw *headersWriter) Flush() {
	if f, ok := hw.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (hw *headersWriter) Unwrap() http.ResponseWriter {
	return hw.w
}

func (h *headersConfig) applySetRequestHeaders(r *http.Request) {
	for key, val := range h.setHeadersReq {
		r.Header.Set(key, val)
	}
}

func (h *headersConfig) applySetResponseHeaders(w http.ResponseWriter) {
	for key, val := range h.setHeadersRes {
		w.Header().Set(key, val)
	}
}

func (h *headersConfig) stripHeadersFromRequest(r *http.Request) {
	for header := range h.stripHeadersReq {
		r.Header.Del(header)
	}
}

func (h *headersConfig) stripHeadersFromResponse(w http.ResponseWriter) {
	for header := range h.stripHeadersRes {
		// Nil instead of calling Del so the key still exists in the map.
		// Go's net/http auto-adds certain headers (e.g. Date) when the
		// key is missing; a nil value prevents that while also not being
		// written
		w.Header()[http.CanonicalHeaderKey(header)] = nil
	}
}

func (h *headersConfig) compareHeadersInRequest(r *http.Request) bool {
	for header, expectedVal := range h.cmpHeadersReq {
		actualVal := r.Header.Get(header)
		if actualVal != expectedVal {
			zap.S().Debugf("Header %s value %s does not match expected value", header, actualVal) //don't leak the value
			ctx.CreateAndAddBlockInfo(r, "headers", fmt.Sprintf("header-%s-mismatch-value", header))
			return false
		}
	}
	return true
}

func HeadersMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	h := parseConfiguration(conf)
	if h == nil {
		zap.S().Errorf("HeadersMiddleware: failed to initialize header writer. Failing closed.")
		return func(w http.ResponseWriter, request *http.Request) {
			http.Error(w, "HeadersMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}
	hasElements := len(h.setHeadersReq) > 0 ||
		len(h.setHeadersRes) > 0 ||
		len(h.stripHeadersReq) > 0 ||
		len(h.stripHeadersRes) > 0 || len(h.cmpHeadersReq) > 0
	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)
		logger.Debug("Applying headers middleware for request")
		if !hasElements {
			next(w, r)
			return
		}
		isAllMatch := h.compareHeadersInRequest(r)
		if !isAllMatch {
			logger.Warn("Request headers do not match expected values")

			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.applySetRequestHeaders(r)
		h.stripHeadersFromRequest(r)
		h.applySetResponseHeaders(w)
		hw := &headersWriter{w: w, conf: h}
		next(hw, r)
	}
}

func parseConfiguration(conf Config) *headersConfig {
	zap.S().Debugf("Parsing configuration for headers middleware: %+v", conf)
	if conf.Options == nil {
		zap.S().Error("Options for headers middleware is nil")
		return nil
	}
	h := headersConfig{
		setHeadersReq:   make(map[string]string),
		setHeadersRes:   make(map[string]string),
		stripHeadersReq: make(map[string]bool),
		stripHeadersRes: make(map[string]bool),
		cmpHeadersReq:   make(map[string]string),
	}
	parseConfigMap(conf, setHeadersReqKey, func(key, val string) {
		h.setHeadersReq[key] = val
	})
	parseConfigMap(conf, setHeadersResKey, func(key, val string) {
		h.setHeadersRes[key] = val
	})
	parseConfigMap(conf, cmpHeadersReqKey, func(key, val string) {
		h.cmpHeadersReq[key] = val
	})
	parseHeaderConfigSlice(conf, stripHeadersResKey, func(val string) {
		h.stripHeadersRes[val] = true
	})
	parseHeaderConfigSlice(conf, stripHeadersReqKey, func(val string) {
		h.stripHeadersReq[val] = true
	})
	return &h
}

func parseHeaderConfigSlice(conf Config, key string, setFunc func(val string)) {
	zap.S().Debugf("Parsing config slice for key: %s", key)
	headerSlice, exists := conf.Options[key]
	zap.S().Debugf("Headers configuration for key %s, exists: %v", key, exists)
	if exists {
		for _, val := range headerSlice.([]interface{}) {
			castedVal, ok := val.(string)
			if !ok {
				zap.S().Errorf("Header value for key %s is not of type string", key)
				continue
			}
			setFunc(castedVal)
		}
		return
	}
	zap.S().Warnf("Headers configuration for key %s not found, skipping", key)
}

func parseConfigMap(conf Config, key string, setFunc func(key, val string)) {
	zap.S().Debugf("Parsing config map for key: %s", key)
	headerMap, exists := conf.Options[key]
	zap.S().Debugf("Headers configuration for key %s, exists: %v", key, exists)
	if exists {
		castedMap, ok := headerMap.(map[string]interface{})
		if !ok {
			zap.S().Errorf("Headers configuration for key %s is not of type map[string]string", key)
		} else {
			parseHeadersMap(castedMap, setFunc)
		}
	}
}

func parseHeadersMap(headerMap map[string]interface{}, setFunc func(key, val string)) {
	zap.S().Debugf("Parsed set headers configuration: %v", headerMap)
	for key, val := range headerMap {
		castedVal, ok := val.(string)
		if !ok {
			zap.S().Errorf("Header value for key %s is not of type string", key)
			continue
		}
		parsedKey, parsedVal := parseHeader(key, castedVal)
		if parsedKey == "" {
			zap.S().Warnf("Parsed key for header %s is empty, skipping", key)
			continue
		}
		setFunc(parsedKey, parsedVal)
	}
}

func parseHeader(header, headerValue string) (string, string) {
	zap.S().Debugf("Parsing header: %s with value: %s", header, headerValue)
	if header == "" || headerValue == "" {
		zap.S().Error("Header or header value cannot be empty")
		return "", ""
	}

	if strings.HasPrefix(headerValue, "$") {
		zap.S().Debugf("Header value starts with $, resolving")
		if len(headerValue) == 1 {
			zap.S().Error("Header value cannot be just $")
			return "", ""
		}
		//$file:
		firstIndexOf := strings.Index(headerValue, ":")
		if firstIndexOf < 0 {
			zap.S().Errorf("Header resolver syntax invalid (missing ':'): %s", headerValue)
			return "", ""
		}
		resolverKey := headerValue[1:firstIndexOf]
		if resolverKey == "" {
			zap.S().Error("Header resolver key cannot be empty")
			return "", ""
		}
		resolver := resolve.GetResolver(resolverKey)
		if resolver == nil {
			zap.S().Errorf("Header resolver not found for key: %s", resolverKey)
			return "", ""
		}
		valueToResolve := headerValue[firstIndexOf+1:]
		resolvedValue, err := resolver.Resolve(valueToResolve)
		if err != nil {
			zap.S().Errorf("Failed to resolve header value: %s", err)
			return "", ""
		}
		zap.S().Debugf("Resolved value for header value %s", headerValue)
		return header, resolvedValue
	}
	zap.S().Debugf("Header value does not start with $, returning as is")
	return header, headerValue
}
