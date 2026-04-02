package metrics

import "fmt"

type Metric struct {
	ConnectionName    string `json:"connection_name"`
	RequestCount      int64  `json:"request_count"`
	ErrorCount        int64  `json:"error_count"`
	TotalLatencyMs    int64  `json:"total_latency_ms"`
	AvgResponseTimeMs int64  `json:"avg_response_time_ms"`
	P50Ms             int64  `json:"p50_ms"`
	P95Ms             int64  `json:"p95_ms"`
	P99Ms             int64  `json:"p99_ms"`
	UpstreamTimeouts  int64  `json:"upstream_timeouts"`
	BytesSent         int64  `json:"bytes_sent"`
	BytesReceived     int64  `json:"bytes_received"`
	ActiveConnections int64  `json:"active_connections"`
	Request2xxCount   int64  `json:"request_2xx_count"`
	Request3xxCount   int64  `json:"request_3xx_count"`
	Request4xxCount   int64  `json:"request_4xx_count"`
	Request5xxCount   int64  `json:"request_5xx_count"`
	CacheHits         int64  `json:"cache_hits"`
	CacheMisses       int64  `json:"cache_misses"`

	// latencies is the internal ring buffer used to compute percentiles.
	// It is not serialised to JSON and is only present on accumulated metrics.
	latencies *latencyRing
}

type RequestMetric struct {
	RemoteAddress  string
	Country        string
	connectionName string
	LatencyMs      int64
	IsTimedOut     bool
	BytesSent      int64
	BytesReceived  int64
	Is2xxResponse  bool
	Is3xxResponse  bool
	Is4xxResponse  bool
	Is5xxResponse  bool
	StatusCode     int
	Path           string
	Method         string
}

// NewMetric creates a Metric with an initialised latency ring buffer.
func NewMetric() *Metric {
	return &Metric{
		latencies: newLatencyRing(latencyBufferSize),
	}
}

func (m *Metric) AddRequestMetric(metric *RequestMetric) {
	m.RequestCount++
	m.TotalLatencyMs += metric.LatencyMs
	m.BytesSent += metric.BytesSent
	m.BytesReceived += metric.BytesReceived
	if metric.Is2xxResponse {
		m.Request2xxCount++
	} else if metric.Is3xxResponse {
		m.Request3xxCount++
	} else if metric.Is4xxResponse {
		m.Request4xxCount++
	} else if metric.Is5xxResponse {
		m.Request5xxCount++
		m.ErrorCount++
	}
	if metric.IsTimedOut {
		m.UpstreamTimeouts++
	}
	m.AvgResponseTimeMs = m.TotalLatencyMs / m.RequestCount
	if m.latencies != nil {
		m.latencies.Add(metric.LatencyMs)
	}
}

func (m *Metric) Add(other *Metric) {
	m.RequestCount += other.RequestCount
	m.ErrorCount += other.ErrorCount
	m.TotalLatencyMs += other.TotalLatencyMs
	m.AvgResponseTimeMs += other.AvgResponseTimeMs
	m.UpstreamTimeouts += other.UpstreamTimeouts
	m.BytesSent += other.BytesSent
	m.BytesReceived += other.BytesReceived
	m.ActiveConnections += other.ActiveConnections
	m.Request2xxCount += other.Request2xxCount
	m.Request3xxCount += other.Request3xxCount
	m.Request4xxCount += other.Request4xxCount
	m.Request5xxCount += other.Request5xxCount
	m.CacheHits += other.CacheHits
	m.CacheMisses += other.CacheMisses
}

func (m *Metric) Copy() *Metric {
	cp := &Metric{
		ConnectionName:    m.ConnectionName,
		RequestCount:      m.RequestCount,
		ErrorCount:        m.ErrorCount,
		TotalLatencyMs:    m.TotalLatencyMs,
		AvgResponseTimeMs: m.AvgResponseTimeMs,
		UpstreamTimeouts:  m.UpstreamTimeouts,
		BytesSent:         m.BytesSent,
		BytesReceived:     m.BytesReceived,
		ActiveConnections: m.ActiveConnections,
		Request2xxCount:   m.Request2xxCount,
		Request3xxCount:   m.Request3xxCount,
		Request4xxCount:   m.Request4xxCount,
		Request5xxCount:   m.Request5xxCount,
		CacheHits:         m.CacheHits,
		CacheMisses:       m.CacheMisses,
	}
	if m.latencies != nil {
		cp.P50Ms = m.latencies.Percentile(50)
		cp.P95Ms = m.latencies.Percentile(95)
		cp.P99Ms = m.latencies.Percentile(99)
	}
	return cp
}

func (m *Metric) Reset() {
	lat := m.latencies
	*m = Metric{}
	if lat != nil {
		lat.reset()
		m.latencies = lat
	}
}

func (m *Metric) String() string {
	return fmt.Sprintf("Metric{ConnectionName: %s, RequestCount: %d,"+
		" ErrorCount: %d, TotalLatencyMs: %d, AvgResponseTimeMs: %d,"+
		" P50Ms: %d, P95Ms: %d, P99Ms: %d, UpstreamTimeouts: %d,"+
		" BytesSent: %d, BytesReceived: %d, ActiveConnections: %d, Request2xxCount: %d, "+
		"Request3xxCount: %d, Request4xxCount: %d, Request5xxCount: %d, CacheHits: %d,"+
		" CacheMisses: %d",
		m.ConnectionName,
		m.RequestCount,
		m.ErrorCount,
		m.TotalLatencyMs,
		m.AvgResponseTimeMs,
		m.P50Ms,
		m.P95Ms,
		m.P99Ms,
		m.UpstreamTimeouts,
		m.BytesSent,
		m.BytesReceived,
		m.ActiveConnections,
		m.Request2xxCount,
		m.Request3xxCount,
		m.Request4xxCount,
		m.Request5xxCount,
		m.CacheHits,
		m.CacheMisses,
	)
}

func ProxyPathMetricsName(port, path string) string {
	if path == "" {
		return ProxyMetricsName(port)
	}
	return fmt.Sprintf("metric-port-%s-path-%s", port, path)
}

func ProxyMetricsName(port string) string {
	return fmt.Sprintf("metric-port-%s", port)
}
