package ctx

import (
	"context"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"go.uber.org/zap"
)

type contextStruct struct {
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
		c := contextStruct{}
		ctx = context.WithValue(ctx, ctxkeys.ContextStruct, &c)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	}
}

func GetContextStruct(r *http.Request) *contextStruct {
	v := r.Context().Value(ctxkeys.ContextStruct)
	if v == nil {
		// This should never happen, but if it does, we return an empty contextStruct to avoid panics in the middleware.
		return &contextStruct{}
	}
	c, ok := v.(*contextStruct)
	if !ok {
		// This should also never happen, but we log an error and return an empty contextStruct to avoid panics.
		zap.S().Error("GetContextStruct: context value is not of type *contextStruct")
		return &contextStruct{}
	}
	return c
}
