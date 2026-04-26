package trustedproxy

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"go.uber.org/zap"
)

type Matcher struct {
	trusted atomic.Pointer[netutil.SubnetTrie]
	header  string
}

func (r *Matcher) isTrusted(addr netip.Addr) bool {
	trie := r.trusted.Load()
	if trie == nil {
		return false
	}
	return trie.Contains(addr)
}

func (r *Matcher) WrapHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		resolved := r.Resolve(req)
		if resolved != req.RemoteAddr {
			zap.S().Debugf("trusted-proxy: rewriting RemoteAddr from %s to %s", req.RemoteAddr, resolved)
			req.RemoteAddr = resolved
		}
		next(w, req)
	}
}

func (r *Matcher) Resolve(req *http.Request) string {
	remoteIP, remotePort, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}

	addr, err := netip.ParseAddr(remoteIP)
	if err != nil {
		return req.RemoteAddr
	}

	if !r.isTrusted(addr) {
		return req.RemoteAddr
	}

	headerVal := req.Header.Get(r.header)
	if headerVal == "" {
		zap.S().Debugf("trusted-proxy: RemoteAddr %s is trusted but header %q is empty, falling back", remoteIP, r.header)
		return req.RemoteAddr
	}

	parts := strings.Split(headerVal, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		if candidate == "" {
			continue
		}
		cAddr, err := netip.ParseAddr(candidate)
		if err != nil {
			zap.S().Debugf("trusted-proxy: unparseable %s entry %q, treating as client", r.header, candidate)
			return net.JoinHostPort(candidate, remotePort)
		}
		if !r.isTrusted(cAddr) {
			return net.JoinHostPort(cAddr.String(), remotePort)
		}
	}

	first := strings.TrimSpace(parts[0])
	if first != "" {
		return net.JoinHostPort(first, remotePort)
	}
	return req.RemoteAddr
}
