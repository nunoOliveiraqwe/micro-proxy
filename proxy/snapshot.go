package proxy

import "github.com/nunoOliveiraqwe/torii/metrics"

type PathSnapshot struct {
	Pattern     string   `json:"pattern"`
	Backend     string   `json:"backend,omitempty"`
	Middlewares []string `json:"middlewares"`
}

type RouteSnapshot struct {
	Host              string         `json:"host,omitempty"`
	Backend           string         `json:"backend"`
	Middlewares       []string       `json:"middlewares"`
	GlobalMiddlewares []string       `json:"global_middlewares"`
	Paths             []PathSnapshot `json:"paths,omitempty"`
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
	ErrorMessage    string            `json:"errorMessage,omitempty"`
}

func collectMiddlewareNames(routes []RouteSnapshot) []string {
	seen := make(map[string]struct{})
	var names []string
	for _, r := range routes {
		for _, n := range r.GlobalMiddlewares {
			if _, ok := seen[n]; !ok {
				seen[n] = struct{}{}
				names = append(names, n)
			}
		}
		for _, n := range r.Middlewares {
			if _, ok := seen[n]; !ok {
				seen[n] = struct{}{}
				names = append(names, n)
			}
		}
		for _, p := range r.Paths {
			for _, n := range p.Middlewares {
				if _, ok := seen[n]; !ok {
					seen[n] = struct{}{}
					names = append(names, n)
				}
			}
		}

	}
	return names
}
