package middleware

import (
	"context"
	"net/http"
)

func IpBlockMiddleware(ctx context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		//TODO
		next(writer, request)
	}
}
