package proxy

type ProxyConfigSnapshot struct {
	Port            int      `json:"port"`
	Interface       string   `json:"interface"`
	MiddlewareChain []string `json:"middleware_chain"`
	IsStarted       bool     `json:"is_started"`
	IsUsingHTTPS    bool     `json:"is_using_https"`
	IsUsingACME     bool     `json:"is_using_acme"`
}
