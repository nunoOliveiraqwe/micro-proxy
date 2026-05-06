package resolve

import "net/http"

type RequestResolver func(r *http.Request) string

var requestVars = map[string]RequestResolver{
	"$remote_addr": func(r *http.Request) string { return r.RemoteAddr },
	"$host":        func(r *http.Request) string { return r.Host },
	"$method":      func(r *http.Request) string { return r.Method },
	"$uri":         func(r *http.Request) string { return r.RequestURI },
	"$scheme":      func(r *http.Request) string { return requestScheme(r) },
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
