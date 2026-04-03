package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

func handleSSEGlobalMetrics(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveSSE(w, r, svc.GetSSEBroker())
	}
}

func serveSSE(w http.ResponseWriter, r *http.Request, broker *app.SSEBroker) {
	logger := middleware.GetRequestLoggerFromContext(r)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		logger.Warn("SSE: could not clear write deadline", zap.Error(err))
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	client := broker.Subscribe()
	defer broker.Unsubscribe(client)

	logger.Info("SSE client connected", zap.String("client_id", client.ID))

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			logger.Info("SSE client disconnected", zap.String("client_id", client.ID))
			return
		case ev, ok := <-client.Events:
			if !ok {
				return
			}
			_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, ev.Data)
			if err != nil {
				logger.Debug("SSE write error", zap.String("client_id", client.ID), zap.Error(err))
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			_, err := fmt.Fprintf(w, ":keepalive\n\n")
			if err != nil {
				logger.Debug("SSE heartbeat error", zap.String("client_id", client.ID), zap.Error(err))
				return
			}
			flusher.Flush()
		}
	}
}
