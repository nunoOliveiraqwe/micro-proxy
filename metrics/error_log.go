package metrics

import (
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/util"
)

type ErrorLogEntry struct {
	Timestamp      time.Time `json:"timestamp"`
	ConnectionName string    `json:"connection_name"`
	RemoteAddress  string    `json:"remote_address"`
	StatusCode     int       `json:"status_code"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	LatencyMs      int64     `json:"latency_ms"`
}

func NewErrorLog(capacity int) *util.RingBuffer[ErrorLogEntry] {
	return util.NewRingBuffer[ErrorLogEntry](capacity)
}
