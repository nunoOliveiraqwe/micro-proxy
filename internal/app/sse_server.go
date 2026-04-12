package app

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nunoOliveiraqwe/torii/metrics"
	"go.uber.org/zap"
)

type SSEEvent struct {
	Type string
	Data []byte
}

type SSEClient struct {
	ID     string
	Events chan SSEEvent
}

type SSEBroker struct {
	mu      sync.RWMutex
	clients map[string]*SSEClient
	nextID  int

	mgr *metrics.ConnectionMetricsManager

	listenersMu       sync.Mutex
	listeners         map[string]int
	metricsListenerID int
	errorListenerID   int
	requestListenerID int
	blockedListenerID int
}

func NewSSEBroker(mgr *metrics.ConnectionMetricsManager) *SSEBroker {
	b := &SSEBroker{
		clients:   make(map[string]*SSEClient),
		mgr:       mgr,
		listeners: make(map[string]int),
	}
	// Wildcard metric listener — fires for every connection's metric updates.
	b.metricsListenerID = mgr.AddWildcardListener(func(_ string, snapshot *metrics.Metric) {
		data, err := json.Marshal(snapshot)
		if err != nil {
			zap.S().Errorf("SSEBroker: failed to marshal metric snapshot: %v", err)
			return
		}
		b.broadcastAll(SSEEvent{Type: "metrics", Data: data})
	})
	b.errorListenerID = mgr.GetErrorLog().AddListener(func(entry *metrics.ErrorLogEntry) {
		data, err := json.Marshal(entry)
		if err != nil {
			zap.S().Errorf("SSEBroker: failed to marshal error entry: %v", err)
			return
		}
		b.broadcastAll(SSEEvent{Type: "proxy_error", Data: data})
	})
	b.requestListenerID = mgr.GetRequestLog().AddListener(func(entry *metrics.RequestLogEntry) {
		data, err := json.Marshal(entry)
		if err != nil {
			zap.S().Errorf("SSEBroker: failed to marshal request log entry: %v", err)
			return
		}
		b.broadcastAll(SSEEvent{Type: "proxy_request", Data: data})
	})
	b.blockedListenerID = mgr.GetBlockedLog().AddListener(func(entry *metrics.BlockLogEntry) {
		data, err := json.Marshal(entry)
		if err != nil {
			zap.S().Errorf("SSEBroker: failed to marshal blocked log entry: %v", err)
			return
		}
		b.broadcastAll(SSEEvent{Type: "proxy_blocked", Data: data})
	})
	return b
}

func (b *SSEBroker) Subscribe() *SSEClient {
	b.mu.Lock()
	id := fmt.Sprintf("sse-%d", b.nextID)
	b.nextID++
	client := &SSEClient{
		ID:     id,
		Events: make(chan SSEEvent, 64),
	}
	b.clients[id] = client
	b.mu.Unlock()

	allMetrics := b.mgr.GetAllMetrics()
	for _, metric := range allMetrics {
		if metric != nil {
			if data, err := json.Marshal(metric); err == nil {
				select {
				case client.Events <- SSEEvent{Type: "metrics", Data: data}:
				default:
				}
			}
		}
	}
	zap.S().Debugf("SSEBroker: client %s subscribed", id)
	return client
}

func (b *SSEBroker) Unsubscribe(client *SSEClient) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.clients[client.ID]; ok {
		close(client.Events)
		delete(b.clients, client.ID)
		zap.S().Debugf("SSEBroker: client %s unsubscribed", client.ID)
	}
}

func (b *SSEBroker) broadcastAll(event SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, client := range b.clients {
		select {
		case client.Events <- event:
		default:
			zap.S().Warnf("SSEBroker: dropping event for slow client %s", client.ID)
		}
	}
}

func (b *SSEBroker) Stop() {
	b.mgr.RemoveListener(b.metricsListenerID)
	b.mgr.GetErrorLog().RemoveListener(b.errorListenerID)
	b.mgr.GetRequestLog().RemoveListener(b.requestListenerID)
	b.mgr.GetBlockedLog().RemoveListener(b.blockedListenerID)

	b.mu.Lock()
	for _, c := range b.clients {
		close(c.Events)
	}
	b.clients = make(map[string]*SSEClient)
	b.mu.Unlock()

	zap.S().Info("SSEBroker stopped")
}
