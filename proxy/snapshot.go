package proxy

import "github.com/nunoOliveiraqwe/micro-proxy/metrics"

type PathSnapshot struct {
	Pattern     string   `json:"pattern"`
	Middlewares []string `json:"middlewares"`
}

type RouteSnapshot struct {
	Host        string         `json:"host,omitempty"`
	Backend     string         `json:"backend"`
	Middlewares []string       `json:"middlewares"`
	Paths       []PathSnapshot `json:"paths,omitempty"`
}

type ProxySnapshot struct {
	Port            int               `json:"port"`
	Backends        []string          `json:"backend"`
	Interface       string            `json:"interface"`
	MiddlewareChain []string          `json:"middleware_chain"`
	IsStarted       bool              `json:"is_started"`
	IsUsingHTTPS    bool              `json:"is_using_https"`
	IsUsingACME     bool              `json:"is_using_acme"`
	Metrics         []*metrics.Metric `json:"metrics"`
	Routes          []RouteSnapshot   `json:"routes,omitempty"`
}
