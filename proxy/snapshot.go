package proxy

import "github.com/nunoOliveiraqwe/micro-proxy/metrics"

type ProxySnapshot struct {
	Port            int             `json:"port"`
	Backends        []string        `json:"backend"`
	Interface       string          `json:"interface"`
	MiddlewareChain []string        `json:"middleware_chain"`
	IsStarted       bool            `json:"is_started"`
	IsUsingHTTPS    bool            `json:"is_using_https"`
	IsUsingACME     bool            `json:"is_using_acme"`
	MetricsName     string          `json:"metrics_name"`
	Metric          *metrics.Metric `json:"metric"`
}
