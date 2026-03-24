package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/app"
	"go.uber.org/zap"
)

func handleSSEGlobalMetrics(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveSSE(w, r, svc.GetSSEBroker())
	}
}

func serveSSE(w http.ResponseWriter, r *http.Request, broker *app.SSEBroker) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		zap.S().Warnf("SSE: could not clear write deadline: %v", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	client := broker.Subscribe()
	defer broker.Unsubscribe(client)

	zap.S().Infof("SSE client %s connected", client.ID)

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			zap.S().Infof("SSE client %s disconnected", client.ID)
			return
		case ev, ok := <-client.Events:
			if !ok {
				return
			}
			_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, ev.Data)
			if err != nil {
				zap.S().Debugf("SSE write error for client %s: %v", client.ID, err)
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			_, err := fmt.Fprintf(w, ":keepalive\n\n")
			if err != nil {
				zap.S().Debugf("SSE heartbeat error for client %s: %v", client.ID, err)
				return
			}
			flusher.Flush()
		}
	}
}
