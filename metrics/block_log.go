package metrics

import (
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/util"
)

type BlockLogEntry struct {
	RemoteAddress      string    `json:"remote_address"`
	Timestamp          time.Time `json:"timestamp"`
	Method             string    `json:"method"`
	Path               string    `json:"path"`
	Status             int       `json:"status"`
	ConnectionName     string    `json:"connection_name"`
	BlockingMiddleware string    `json:"blocking_middleware"`
	BlockReason        string    `json:"block_reason"`
}

func NewBlockLog(capacity int) *util.RingBuffer[BlockLogEntry] {
	return util.NewRingBuffer[BlockLogEntry](capacity)
}
