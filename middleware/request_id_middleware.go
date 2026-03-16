package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

const requestIDHeader = "X-Request-ID"

const requestIdContextKey = "requestID"

var requestIDCounter uint64

func RequestIDMiddleware(next http.HandlerFunc, middlewareConf MiddlewareConfiguration) http.HandlerFunc {
	prefix := ""
	if middlewareConf.Config != nil {
		requestIdPrefix := middlewareConf.Config["prefix"]
		if requestIdPrefix != nil {
			if prefixStr, ok := requestIdPrefix.(string); ok {
				prefix = prefixStr
			}
		}
	}
	if prefix == "" {
		prefix = generateRequestPrefix()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		reqId := getRequestIDFromContext(r.Context())
		if reqId != "" {
			next.ServeHTTP(w, r)
			return
		}
		reqId = generateRequestID(prefix, requestIDCounter)
		ctx := context.WithValue(r.Context(), requestIdContextKey, reqId)
		next.ServeHTTP(w, r.WithContext(ctx))
	}

}

func generateRequestPrefix() string {
	// from https://github.com/zenazn/goji/blob/master/web/middleware/request_id.go
	hostname, err := os.Hostname()
	if hostname == "" || err != nil {
		hostname = "localhost"
	}
	var buf [12]byte
	var b64 string
	for len(b64) < 10 {
		rand.Read(buf[:])
		b64 = base64.StdEncoding.EncodeToString(buf[:])
		b64 = strings.NewReplacer("+", "", "/", "").Replace(b64)
	}
	return fmt.Sprintf("%s/%s", hostname, b64[0:10])
}

func getRequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	val := ctx.Value(requestIDHeader)
	if val == nil {
		return ""
	}
	if requestID, ok := val.(string); ok {
		return requestID
	}
	return ""
}

func generateRequestID(prefix string, id uint64) string {
	nextId := atomic.AddUint64(&requestIDCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, nextId)
}
