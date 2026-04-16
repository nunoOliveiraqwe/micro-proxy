package proxy

import (
	"net/http"
	"sync/atomic"
)

type SwappableHandler struct {
	handler atomic.Pointer[http.Handler]
}

func NewSwappableHandler(h http.Handler) *SwappableHandler {
	s := &SwappableHandler{}
	s.handler.Store(&h)
	return s
}

func (s *SwappableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := *s.handler.Load()
	h.ServeHTTP(w, r)
}

func (s *SwappableHandler) Swap(h http.Handler) http.Handler {
	old := s.handler.Swap(&h)
	return *old
}

func (s *SwappableHandler) Load() http.Handler {
	return *s.handler.Load()
}
