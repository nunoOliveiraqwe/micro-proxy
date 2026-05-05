package ctx

import (
	"context"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"go.uber.org/zap"
)

type ContextStruct struct {
	BlockInfo     *BlockInfo
	CountryCode   string
	ContinentCode string
	RequestId     string
	Logger        *zap.Logger
	ReqMetrics    *metrics.RequestMetric
}

func InjectContextStruct(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctxStuct := r.Context().Value(ctxkeys.ContextStruct)
		if ctxStuct != nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		c := ContextStruct{}
		ctx = context.WithValue(ctx, ctxkeys.ContextStruct, &c)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	}
}

func GetContextStruct(r *http.Request) *ContextStruct {
	if r == nil {
		zap.S().Error("GetContextStruct: request is nil")
		return &ContextStruct{}
	}
	v := r.Context().Value(ctxkeys.ContextStruct)
	if v == nil {
		// This should never happen, but if it does, we return an empty contextStruct to avoid panics in the middleware.
		return &ContextStruct{}
	}
	c, ok := v.(*ContextStruct)
	if !ok {
		// This should also never happen, but we log an error and return an empty contextStruct to avoid panics.
		zap.S().Error("GetContextStruct: context value is not of type *contextStruct")
		return &ContextStruct{}
	}
	return c
}
