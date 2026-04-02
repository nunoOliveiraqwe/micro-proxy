package metrics

import (
	"sync"
	"time"
)

type RequestLogEntry struct {
	RemoteAddress  string    `json:"remote_address"`
	Country        string    `json:"country"`
	Timestamp      time.Time `json:"timestamp"`
	ConnectionName string    `json:"connection_name"`
	StatusCode     int       `json:"status_code"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	LatencyMs      int64     `json:"latency_ms"`
	BytesSent      int64     `json:"bytes_sent"`
	BytesReceived  int64     `json:"bytes_received"`
}

type RequestLogListenerFunc func(entry *RequestLogEntry)

type RequestLog struct {
	mu       sync.RWMutex
	entries  []RequestLogEntry
	pos      int
	count    int
	capacity int

	listenersMu    sync.RWMutex
	listeners      map[int]RequestLogListenerFunc
	nextListenerID int
}

func NewRequestLog(capacity int) *RequestLog {
	return &RequestLog{
		entries:   make([]RequestLogEntry, capacity),
		capacity:  capacity,
		listeners: make(map[int]RequestLogListenerFunc),
	}
}

func (l *RequestLog) Add(entry RequestLogEntry) {
	l.mu.Lock()
	l.entries[l.pos] = entry
	l.pos = (l.pos + 1) % l.capacity
	if l.count < l.capacity {
		l.count++
	}
	l.mu.Unlock()
	l.notifyListeners(&entry)
}

func (l *RequestLog) Recent(n int) []RequestLogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if n > l.count {
		n = l.count
	}
	if n == 0 {
		return []RequestLogEntry{}
	}

	result := make([]RequestLogEntry, n)
	for i := 0; i < n; i++ {
		idx := (l.pos - 1 - i + l.capacity) % l.capacity
		result[i] = l.entries[idx]
	}
	return result
}

func (l *RequestLog) AddListener(fn RequestLogListenerFunc) int {
	l.listenersMu.Lock()
	defer l.listenersMu.Unlock()
	id := l.nextListenerID
	l.nextListenerID++
	l.listeners[id] = fn
	return id
}

func (l *RequestLog) RemoveListener(id int) {
	l.listenersMu.Lock()
	defer l.listenersMu.Unlock()
	delete(l.listeners, id)
}

func (l *RequestLog) notifyListeners(entry *RequestLogEntry) {
	l.listenersMu.RLock()
	defer l.listenersMu.RUnlock()
	for _, fn := range l.listeners {
		fn(entry)
	}
}
