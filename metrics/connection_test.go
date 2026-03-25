package metrics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewConnectionMetricsHandler(t *testing.T) {
	ctx := context.Background()
	h := NewGlobalMetricsHandler(2, ctx)
	assert.NotNil(t, h)
	assert.Equal(t, 2, h.numberOfWorkers)
	assert.NotNil(t, h.connectionMetricsMap[globalMetricsConName])
}

func TestUpdateConnectionMetrics(t *testing.T) {
	ctx := context.Background()
	h := NewGlobalMetricsHandler(2, ctx)
	h.StartCollectingMetrics()

	time.Sleep(100 * time.Millisecond)
	metric := &RequestMetric{
		connectionName: "test",
		BytesReceived:  100,
		BytesSent:      200,
	}
	reportTestMetrics := h.TrackMetricsForConnection("test-server", "test")
	reportTestMetrics(metric)

	deadLine := time.Now().Add(1 * time.Second)
	for {
		result := h.GetMetricForConnection("test")
		if result != nil && result.BytesReceived != 0 {
			break
		}
		if time.Now().After(deadLine) {
			assert.Fail(t, "timeout waiting for metrics")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	result := h.GetMetricForConnection("test")
	assert.NotNil(t, result)
	assert.Equal(t, int64(100), result.BytesReceived)
	assert.Equal(t, int64(200), result.BytesSent)

	deadLine = time.Now().Add(1 * time.Second)
	for {
		global := h.GetMetricForConnection(globalMetricsConName)
		if global != nil && global.BytesReceived != 0 {
			break
		}
		if time.Now().After(deadLine) {
			assert.Fail(t, "timeout waiting for metrics")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	global := h.GetMetricForConnection(globalMetricsConName)
	assert.NotNil(t, global)
	assert.Equal(t, int64(100), global.BytesReceived)
	assert.Equal(t, int64(200), global.BytesSent)
}

func TestGlobalAccumulatesFromSeveralConMetrics(t *testing.T) {
	ctx := context.Background()
	h := NewGlobalMetricsHandler(2, ctx)
	h.StartCollectingMetrics()

	time.Sleep(100 * time.Millisecond)
	metricForTest := &RequestMetric{
		connectionName: "test",
		BytesReceived:  100,
		BytesSent:      200,
	}
	reportTestMetrics := h.TrackMetricsForConnection("test-server", "test")

	reportTestMetrics(metricForTest)

	metricForTest2 := &RequestMetric{
		connectionName: "test2",
		BytesReceived:  102,
		BytesSent:      201,
	}
	reportTestMetrics2 := h.TrackMetricsForConnection("test-server", "test2")

	reportTestMetrics2(metricForTest2)

	deadLine := time.Now().Add(10 * time.Second)
	for {
		global := h.GetMetricForConnection(globalMetricsConName)
		if global != nil && global.BytesReceived%10 == 2 {
			break
		}
		if time.Now().After(deadLine) {
			assert.Fail(t, "timeout waiting for metrics")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	global := h.GetMetricForConnection(globalMetricsConName)
	assert.NotNil(t, global)
	assert.Equal(t, metricForTest2.BytesReceived+metricForTest.BytesReceived,
		global.BytesReceived)
	assert.Equal(t, metricForTest2.BytesSent+metricForTest.BytesSent,
		global.BytesSent)

}

func TestAddAndRemoveListener(t *testing.T) {
	ctx := context.Background()
	h := NewGlobalMetricsHandler(2, ctx)

	called := false
	id := h.AddListener("test", func(connectionName string, snapshot *Metric) {
		called = true
	})
	assert.Equal(t, 0, id)

	id2 := h.AddListener("test", func(connectionName string, snapshot *Metric) {})
	assert.Equal(t, 1, id2)

	ok := h.RemoveListener(id)
	assert.True(t, ok)

	ok = h.RemoveListener(id)
	assert.False(t, ok, "removing the same listener twice should return false")

	assert.False(t, called, "listener should not have been called yet")
}

func TestListenerReceivesSnapshotOnMetricUpdate(t *testing.T) {
	ctx := context.Background()
	h := NewGlobalMetricsHandler(2, ctx)
	h.StartCollectingMetrics()

	var mu sync.Mutex
	var received []*Metric
	var receivedNames []string

	h.AddListener("test", func(connectionName string, snapshot *Metric) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, snapshot)
		receivedNames = append(receivedNames, connectionName)
	})

	time.Sleep(100 * time.Millisecond)
	report := h.TrackMetricsForConnection("test-server", "test")
	report(&RequestMetric{BytesReceived: 50, BytesSent: 75})

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			assert.Fail(t, "timeout waiting for listener callback")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "test", receivedNames[0])
	assert.Equal(t, int64(50), received[0].BytesReceived)
	assert.Equal(t, int64(75), received[0].BytesSent)
}

func TestGlobalListenerReceivesUpdates(t *testing.T) {
	ctx := context.Background()
	h := NewGlobalMetricsHandler(2, ctx)
	h.StartCollectingMetrics()

	var mu sync.Mutex
	var globalSnapshots []*Metric

	h.AddGlobalListener(func(connectionName string, snapshot *Metric) {
		mu.Lock()
		defer mu.Unlock()
		globalSnapshots = append(globalSnapshots, snapshot)
	})

	time.Sleep(100 * time.Millisecond)
	report := h.TrackMetricsForConnection("conn1-server", "conn1")
	report(&RequestMetric{BytesReceived: 10, BytesSent: 20})

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(globalSnapshots)
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			assert.Fail(t, "timeout waiting for global listener callback")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, int64(10), globalSnapshots[0].BytesReceived)
	assert.Equal(t, int64(20), globalSnapshots[0].BytesSent)
}

func TestRemovedListenerStopsReceiving(t *testing.T) {
	ctx := context.Background()
	h := NewGlobalMetricsHandler(2, ctx)
	h.StartCollectingMetrics()

	var mu sync.Mutex
	callCount := 0

	id := h.AddListener("test", func(connectionName string, snapshot *Metric) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
	})

	time.Sleep(100 * time.Millisecond)
	report := h.TrackMetricsForConnection("test-server", "test")
	report(&RequestMetric{BytesReceived: 1, BytesSent: 1})

	// Wait for the first callback.
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := callCount
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			assert.Fail(t, "timeout waiting for listener callback")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Remove the listener and send another metric.
	h.RemoveListener(id)
	report(&RequestMetric{BytesReceived: 1, BytesSent: 1})

	// Give workers time to process.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, callCount, "listener should not be called after removal")
}

func TestPathMetricAutoCreatesParent(t *testing.T) {
	ctx := context.Background()
	h := NewGlobalMetricsHandler(2, ctx)

	// Register a path-level metric — the port-level parent should be auto-created.
	h.TrackMetricsForConnection("http-8081", "metric-port-8081-path-/api/v1/users")

	parent := h.GetMetricForConnection("metric-port-8081")
	assert.NotNil(t, parent, "port-level parent should be auto-created")
	assert.Equal(t, "metric-port-8081", parent.ConnectionName)
}

func TestHierarchyPropagation(t *testing.T) {
	ctx := context.Background()
	h := NewGlobalMetricsHandler(2, ctx)
	h.StartCollectingMetrics()
	time.Sleep(100 * time.Millisecond)

	// Register two path-level metrics under the same port.
	report1 := h.TrackMetricsForConnection("http-8081", "metric-port-8081-path-/api/v1/users")
	report2 := h.TrackMetricsForConnection("http-8081", "metric-port-8081-path-/api/v2/users")

	report1(&RequestMetric{BytesReceived: 100, BytesSent: 200})
	report2(&RequestMetric{BytesReceived: 50, BytesSent: 75})

	// Wait for both to propagate.
	deadline := time.Now().Add(2 * time.Second)
	for {
		global := h.GetMetricForConnection(globalMetricsConName)
		if global != nil && global.RequestCount >= 2 {
			break
		}
		if time.Now().After(deadline) {
			assert.Fail(t, "timeout waiting for global metrics")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Each leaf should have its own count.
	leaf1 := h.GetMetricForConnection("metric-port-8081-path-/api/v1/users")
	assert.Equal(t, int64(1), leaf1.RequestCount)
	assert.Equal(t, int64(100), leaf1.BytesReceived)

	leaf2 := h.GetMetricForConnection("metric-port-8081-path-/api/v2/users")
	assert.Equal(t, int64(1), leaf2.RequestCount)
	assert.Equal(t, int64(50), leaf2.BytesReceived)

	// Parent should aggregate both children.
	parent := h.GetMetricForConnection("metric-port-8081")
	assert.NotNil(t, parent)
	assert.Equal(t, int64(2), parent.RequestCount)
	assert.Equal(t, int64(150), parent.BytesReceived)
	assert.Equal(t, int64(275), parent.BytesSent)

	// Global should equal parent (only one port).
	global := h.GetMetricForConnection(globalMetricsConName)
	assert.Equal(t, int64(2), global.RequestCount)
	assert.Equal(t, int64(150), global.BytesReceived)
	assert.Equal(t, int64(275), global.BytesSent)
}

func TestGetRegisteredConnectionNames(t *testing.T) {
	ctx := context.Background()
	h := NewGlobalMetricsHandler(2, ctx)

	h.TrackMetricsForConnection("http-8081", "metric-port-8081-path-/api/v1/users")
	h.TrackMetricsForConnection("http-9090", "metric-port-9090")

	names := h.GetRegisteredConnectionNames()
	assert.Contains(t, names, "global")
	assert.Contains(t, names, "metric-port-8081")                    // auto-created parent
	assert.Contains(t, names, "metric-port-8081-path-/api/v1/users") // leaf
	assert.Contains(t, names, "metric-port-9090")                    // standalone port
	assert.Len(t, names, 4)
}
