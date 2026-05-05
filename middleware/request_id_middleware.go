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

	"github.com/nunoOliveiraqwe/torii/middleware/ctx"
)

var requestIDCounter uint64

func RequestIDMiddleware(_ context.Context, next http.HandlerFunc, middlewareConf Config) http.HandlerFunc {
	prefix := ""
	if middlewareConf.Options != nil {
		requestIdPrefix := middlewareConf.Options["prefix"]
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
		localPrefix := prefix
		ctxStruct := ctx.GetContextStruct(r)
		reqId := ctxStruct.RequestId
		if reqId != "" {
			if !strings.HasPrefix(reqId, localPrefix) { //double injection, for example coming from global or default and this is applying at path levle
				localPrefix = fmt.Sprintf("%s-%s", reqId, localPrefix)
			} else {
				next.ServeHTTP(w, r)
				return
			}
		}
		reqId = generateRequestID(localPrefix)
		ctxStruct.RequestId = reqId
		next.ServeHTTP(w, r)
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

func GetRequestIDFromContext(r *http.Request) string {
	ctxStruct := ctx.GetContextStruct(r)
	return ctxStruct.RequestId
}

func generateRequestID(prefix string) string {
	nextId := atomic.AddUint64(&requestIDCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, nextId)
}
