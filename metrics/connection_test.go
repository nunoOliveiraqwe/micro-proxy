package metrics

import (
	"context"
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
	reportTestMetrics := h.NewConnectionMetricHandler("test")
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
	reportTestMetrics := h.NewConnectionMetricHandler("test")

	reportTestMetrics(metricForTest)

	metricForTest2 := &RequestMetric{
		connectionName: "test2",
		BytesReceived:  102,
		BytesSent:      201,
	}
	reportTestMetrics2 := h.NewConnectionMetricHandler("test2")

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
