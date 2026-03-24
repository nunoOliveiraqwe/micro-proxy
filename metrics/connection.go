package metrics

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

type MetricListenerFunc func(connectionName string, snapshot *Metric)

type metricListener struct {
	id             int
	connectionName string
	fn             MetricListenerFunc
}

type ConnectionMetricsManager struct {
	connectionMetricsMap map[string]*ConnectionMetric
	metricsChan          chan *RequestMetric
	numberOfWorkers      int
	context              context.Context
	cancel               context.CancelFunc

	listenersMu  sync.RWMutex
	listeners    map[int]*metricListener
	nextListenID int

	errorLog   *ErrorLog
	requestLog *RequestLog
}

type ConnectionMetric struct {
	accumulatedMetrics *Metric
	connectionName     string
	metricsLock        sync.RWMutex
}

type MetricsReportFunc func(reqMetric *RequestMetric)

const globalMetricsConName = "global"

func NewGlobalMetricsHandler(numberOfWorkers int, ctx context.Context) *ConnectionMetricsManager {
	zap.S().Debug("Creating connection metrics handler")
	ctx, cancel := context.WithCancel(ctx)
	h := ConnectionMetricsManager{
		connectionMetricsMap: make(map[string]*ConnectionMetric),
		metricsChan:          make(chan *RequestMetric),
		numberOfWorkers:      numberOfWorkers,
		context:              ctx,
		cancel:               cancel,
		listeners:            make(map[int]*metricListener),
		errorLog:             NewErrorLog(100),
		requestLog:           NewRequestLog(200),
	}
	zap.S().Info("Creating a new global connection metric")
	h.TrackMetricsForConnection(globalMetricsConName)
	return &h
}

func (h *ConnectionMetricsManager) addConnectionMetric(c *ConnectionMetric) {
	zap.S().Debugf("Adding connection metric for connection %s", c.connectionName)
	h.connectionMetricsMap[c.connectionName] = c
}

func (h *ConnectionMetricsManager) startCollectingMetrics() {
	waitG := sync.WaitGroup{}
	waitG.Add(h.numberOfWorkers)
	for i := 0; i < h.numberOfWorkers; i++ {
		go func() {
			h.collectGlobalMetrics()
			waitG.Done()
		}()
	}
	waitG.Wait()
	close(h.metricsChan)
}

func (h *ConnectionMetricsManager) collectGlobalMetrics() {
	for {
		select {
		case metric, ok := <-h.metricsChan:
			if !ok {
				return
			}
			h.updateConnectionMetrics(metric)
		case <-h.context.Done():
			return
		}
	}
}

func (h *ConnectionMetricsManager) updateConnectionMetrics(metric *RequestMetric) {
	zap.S().Infof("Updating connection metric for connection %s", metric.connectionName)
	conMetrics, ok := h.connectionMetricsMap[metric.connectionName]
	if !ok {
		zap.S().Warnf("Connection metric for connection %s not found", metric.connectionName)
		return
	}
	conMetrics.metricsLock.Lock()
	conMetrics.accumulatedMetrics.AddRequestMetric(metric)
	conSnapshot := conMetrics.accumulatedMetrics.Copy()
	conMetrics.metricsLock.Unlock()
	h.notifyListeners(metric.connectionName, conSnapshot)

	if metric.connectionName != globalMetricsConName {
		globalConMetrics, ok2 := h.connectionMetricsMap[globalMetricsConName]
		if !ok2 {
			zap.S().Errorf("no global connection metrics found")
			return
		}
		globalConMetrics.metricsLock.Lock()
		globalConMetrics.accumulatedMetrics.AddRequestMetric(metric)
		globalSnapshot := globalConMetrics.accumulatedMetrics.Copy()
		globalConMetrics.metricsLock.Unlock()

		h.notifyListeners(globalMetricsConName, globalSnapshot)
	}

	h.requestLog.Add(RequestLogEntry{
		Timestamp:      time.Now(),
		RemoteAddress:  metric.RemoteAddress,
		ConnectionName: metric.connectionName,
		StatusCode:     metric.StatusCode,
		Method:         metric.Method,
		Path:           metric.Path,
		LatencyMs:      metric.LatencyMs,
		BytesSent:      metric.BytesSent,
		BytesReceived:  metric.BytesReceived,
	})

	if metric.Is5xxResponse {
		h.errorLog.Add(ErrorEntry{
			Timestamp:      time.Now(),
			ConnectionName: metric.connectionName,
			RemoteAddress:  metric.RemoteAddress,
			StatusCode:     metric.StatusCode,
			Method:         metric.Method,
			Path:           metric.Path,
			LatencyMs:      metric.LatencyMs,
		})
	}
}

func (h *ConnectionMetricsManager) TrackMetricsForConnection(connectionName string) MetricsReportFunc {
	zap.S().Debugf("Creating a new connection metric for connection %s", connectionName)
	m := NewMetric()
	m.ConnectionName = connectionName
	connMetric := &ConnectionMetric{
		accumulatedMetrics: m,
		metricsLock:        sync.RWMutex{},
		connectionName:     connectionName,
	}
	h.addConnectionMetric(connMetric)
	return func(metric *RequestMetric) {
		metric.connectionName = connectionName
		h.metricsChan <- metric
	}
}

func (h *ConnectionMetricsManager) GetErrorLog() *ErrorLog {
	return h.errorLog
}

func (h *ConnectionMetricsManager) GetRequestLog() *RequestLog {
	return h.requestLog
}

func (h *ConnectionMetricsManager) GetMetricForConnection(connectionName string) *Metric {
	zap.S().Debugf("Getting connection metrics for connection %s", connectionName)
	conMetrics, ok := h.connectionMetricsMap[connectionName]
	if !ok {
		zap.S().Infof("Connection metric for connection %s not found", connectionName)
		return nil
	}
	conMetrics.metricsLock.RLock()
	defer conMetrics.metricsLock.RUnlock()
	return conMetrics.accumulatedMetrics.Copy()
}

func (h *ConnectionMetricsManager) GetAllMetrics() []*Metric {
	zap.S().Infof("Getting all connection metrics")
	metrics := make([]*Metric, 0, len(h.connectionMetricsMap))
	for _, conMetrics := range h.connectionMetricsMap {
		conMetrics.metricsLock.RLock()
		metrics = append(metrics, conMetrics.accumulatedMetrics.Copy())
		conMetrics.metricsLock.RUnlock()
	}
	return metrics
}

func (h *ConnectionMetricsManager) GetGlobalMetrics() *Metric {
	return h.GetMetricForConnection(globalMetricsConName)
}

func (h *ConnectionMetricsManager) StartCollectingMetrics() {
	go h.startCollectingMetrics()
}

func (h *ConnectionMetricsManager) StopCollectingMetrics() {
	h.cancel()
}

func (h *ConnectionMetricsManager) AddListener(connectionName string, fn MetricListenerFunc) int {
	h.listenersMu.Lock()
	defer h.listenersMu.Unlock()
	id := h.nextListenID
	h.nextListenID++
	h.listeners[id] = &metricListener{
		id:             id,
		connectionName: connectionName,
		fn:             fn,
	}
	zap.S().Debugf("Added metric listener %d for connection %s", id, connectionName)
	return id
}

func (h *ConnectionMetricsManager) AddGlobalListener(fn MetricListenerFunc) int {
	return h.AddListener(globalMetricsConName, fn)
}

func (h *ConnectionMetricsManager) RemoveListener(id int) bool {
	h.listenersMu.Lock()
	defer h.listenersMu.Unlock()
	_, ok := h.listeners[id]
	if ok {
		delete(h.listeners, id)
		zap.S().Debugf("Removed metric listener %d", id)
	}
	return ok
}

func (h *ConnectionMetricsManager) notifyListeners(connectionName string, snapshot *Metric) {
	h.listenersMu.RLock()
	defer h.listenersMu.RUnlock()
	for _, l := range h.listeners {
		// Wildcard listeners (empty connectionName) receive all events.
		if l.connectionName == "" || l.connectionName == connectionName {
			l.fn(connectionName, snapshot)
		}
	}
}

func (h *ConnectionMetricsManager) AddWildcardListener(fn MetricListenerFunc) int {
	return h.AddListener("", fn)
}
