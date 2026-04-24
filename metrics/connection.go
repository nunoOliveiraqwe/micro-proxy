package metrics

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/util"
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

	errorLog   *util.RingBuffer[ErrorLogEntry]
	requestLog *util.RingBuffer[RequestLogEntry]
	blockedLog *util.RingBuffer[BlockLogEntry]
}

type ConnectionMetric struct {
	serverId           string
	accumulatedMetrics *Metric
	connectionName     string
	parentMetric       *ConnectionMetric
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
		errorLog:             NewErrorLog(200), //maybe add a conf flag
		requestLog:           NewRequestLog(500),
		blockedLog:           NewBlockLog(500),
	}
	zap.S().Info("Creating a new global connection metric")
	h.TrackMetricsForConnection(globalMetricsConName, globalMetricsConName)
	return &h
}

func (h *ConnectionMetricsManager) TrackMetricsForConnection(serverId, connectionName string) MetricsReportFunc {
	zap.S().Debugf("Creating a new connection metric for connection %s", connectionName)

	//go's map iteration is wonky, it will always suprised me with it's out of order, so, if im iterating the creation of
	//proxy routes, i have no guarantees the parent will be created before the child, and if the
	//child comes first we create the parent, and the child holds the pointer, but then the parent comes allong
	// and gets created, this fucks things up, because the pointer of the child will be to another ref
	//so i got to check if it exists first, not because its suposed to, but due to how map iteration works in go, which is not deterministic,
	//and can cause the parent to be created after the child, which would cause the child to point to the wrong parent metric
	//this is a bit of a nightmare, but its the only way to ensure the parent-child relationships are correct regardless of the order of creation
	//i could also just require that the caller creates the parent metrics first, but that would be error prone and not very user friendly, so this is the best solution i guess

	_, ok := h.connectionMetricsMap[connectionName]
	if ok {
		zap.S().Infof("Connection metric for connection %s already exists, returning existing report function", connectionName)
		return func(reqMetric *RequestMetric) { //yes, its a new func, but metrics are then routed by name, so this works
			reqMetric.connectionName = connectionName
			select {
			case h.metricsChan <- reqMetric:
			case <-h.context.Done():
			}
		}
	}

	m := NewMetric()
	m.ConnectionName = connectionName
	parentName := deriveParentMetricName(connectionName)
	zap.S().Debugf("Derived parent metric name %s for connection %s", parentName, connectionName)

	var connectionMetric *ConnectionMetric

	if parentName != "" {
		con, exists := h.connectionMetricsMap[parentName]
		if !exists {
			zap.S().Warnf("Parent metric %s not found for connection %s, auto-creating it", parentName, connectionName)
			//we need to create the parent. This will recursively create any missing ancestors as well.
			h.TrackMetricsForConnection(serverId, parentName) // recursively create parent metric if it doesn't exist
			con, exists = h.connectionMetricsMap[parentName]  //has to exist
			if !exists {
				zap.S().Fatalf("Failed to create parent metric %s for connection %s", parentName, connectionName)
				return func(reqMetric *RequestMetric) {
					zap.S().Fatalf("Cannot report metric for connection %s because parent metric %s could not be created", connectionName, parentName)
				}
			}
		}
		connectionMetric = con
	}

	connMetric := &ConnectionMetric{
		serverId:           serverId,
		accumulatedMetrics: m,
		metricsLock:        sync.RWMutex{},
		connectionName:     connectionName,
		parentMetric:       connectionMetric,
	}
	h.addConnectionMetric(connMetric)

	return func(metric *RequestMetric) {
		metric.connectionName = connectionName
		select {
		case h.metricsChan <- metric:
		case <-h.context.Done():
		}
	}
}

func (h *ConnectionMetricsManager) GetErrorLog() *util.RingBuffer[ErrorLogEntry] {
	return h.errorLog
}

func (h *ConnectionMetricsManager) GetRequestLog() *util.RingBuffer[RequestLogEntry] {
	return h.requestLog
}

func (h *ConnectionMetricsManager) GetBlockedLog() *util.RingBuffer[BlockLogEntry] {
	return h.blockedLog
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

func (h *ConnectionMetricsManager) GetAllMetricsByServer(serverId string) []*Metric {
	zap.S().Debugf("Getting all connection metrics for server %s", serverId)
	metrics := make([]*Metric, 0, len(h.connectionMetricsMap))
	for _, conMetrics := range h.connectionMetricsMap {
		conMetrics.metricsLock.RLock()
		if conMetrics.serverId != serverId {
			conMetrics.metricsLock.RUnlock()
			continue
		}
		metrics = append(metrics, conMetrics.accumulatedMetrics.Copy())
		conMetrics.metricsLock.RUnlock()
	}
	return metrics
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

func (h *ConnectionMetricsManager) AddWildcardListener(fn MetricListenerFunc) int {
	return h.AddListener("", fn)
}

func (h *ConnectionMetricsManager) GetRegisteredConnectionNames() []string {
	names := make([]string, 0, len(h.connectionMetricsMap))
	for name := range h.connectionMetricsMap {
		names = append(names, name)
	}
	return names
}

func deriveParentMetricName(connectionName string) string {
	// Path-level → parent is everything before "-path-" (could be host-level or port-level).
	idx := strings.Index(connectionName, "-path-")
	if idx != -1 {
		return connectionName[:idx]
	}
	// Host-level → parent is the port-level metric.
	idx = strings.Index(connectionName, "-host-")
	if idx != -1 {
		return connectionName[:idx]
	}
	if connectionName == globalMetricsConName {
		return ""
	}
	return globalMetricsConName
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
	zap.S().Debugf("Updating connection metric for connection %s", metric.connectionName)
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

	for cur := conMetrics.parentMetric; cur != nil; cur = cur.parentMetric {
		cur.metricsLock.Lock()
		cur.accumulatedMetrics.AddRequestMetric(metric)
		parentSnapshot := cur.accumulatedMetrics.Copy()
		cur.metricsLock.Unlock()
		h.notifyListeners(cur.connectionName, parentSnapshot)
	}

	h.requestLog.Add(RequestLogEntry{
		Timestamp:      time.Now(),
		RemoteAddress:  metric.RemoteAddress,
		Host:           metric.Host,
		Country:        metric.Country,
		ConnectionName: metric.connectionName,
		StatusCode:     metric.StatusCode,
		Method:         metric.Method,
		Path:           metric.Path,
		LatencyMs:      metric.LatencyMs,
		BytesSent:      metric.BytesSent,
		BytesReceived:  metric.BytesReceived,
	})

	if metric.IsMiddlewareBlockedRequest {
		h.blockedLog.Add(BlockLogEntry{
			RemoteAddress:      metric.RemoteAddress,
			Host:               metric.Host,
			Timestamp:          time.Now(),
			Method:             metric.Method,
			Path:               metric.Path,
			Status:             metric.StatusCode,
			ConnectionName:     metric.connectionName,
			BlockReason:        metric.BlockReason,
			BlockingMiddleware: metric.BlockingMiddleware,
		})
	}

	if metric.Is5xxResponse {
		h.errorLog.Add(ErrorLogEntry{
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
