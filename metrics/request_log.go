package metrics

import (
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/util"
)

type RequestLogEntry struct {
	RemoteAddress  string    `json:"remote_address"`
	Country        string    `json:"country"`
	Timestamp      time.Time `json:"timestamp"`
	ConnectionName string    `json:"connection_name"` //to be able to filter by, or else this would have to be per connection metric
	StatusCode     int       `json:"status_code"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	LatencyMs      int64     `json:"latency_ms"`
	BytesSent      int64     `json:"bytes_sent"`
	BytesReceived  int64     `json:"bytes_received"`
}

func NewRequestLog(capacity int) *util.RingBuffer[RequestLogEntry] {
	return util.NewRingBuffer[RequestLogEntry](capacity)
}
