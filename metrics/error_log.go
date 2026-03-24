package metrics

import (
	"sync"
	"time"
)

type ErrorEntry struct {
	Timestamp      time.Time `json:"timestamp"`
	ConnectionName string    `json:"connection_name"`
	RemoteAddress  string    `json:"remote_address"`
	StatusCode     int       `json:"status_code"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	LatencyMs      int64     `json:"latency_ms"`
}

type ErrorListenerFunc func(entry *ErrorEntry)

type ErrorLog struct {
	mu       sync.RWMutex
	entries  []ErrorEntry
	pos      int
	count    int
	capacity int

	listenersMu    sync.RWMutex
	listeners      map[int]ErrorListenerFunc
	nextListenerID int
}

func NewErrorLog(capacity int) *ErrorLog {
	return &ErrorLog{
		entries:   make([]ErrorEntry, capacity),
		capacity:  capacity,
		listeners: make(map[int]ErrorListenerFunc),
	}
}

func (l *ErrorLog) Add(entry ErrorEntry) {
	l.mu.Lock()
	l.entries[l.pos] = entry
	l.pos = (l.pos + 1) % l.capacity
	if l.count < l.capacity {
		l.count++
	}
	l.mu.Unlock()
	l.notifyListeners(&entry)
}

func (l *ErrorLog) Recent(n int) []ErrorEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if n > l.count {
		n = l.count
	}
	if n == 0 {
		return []ErrorEntry{}
	}

	result := make([]ErrorEntry, n)
	for i := 0; i < n; i++ {
		idx := (l.pos - 1 - i + l.capacity) % l.capacity
		result[i] = l.entries[idx]
	}
	return result
}

func (l *ErrorLog) AddListener(fn ErrorListenerFunc) int {
	l.listenersMu.Lock()
	defer l.listenersMu.Unlock()
	id := l.nextListenerID
	l.nextListenerID++
	l.listeners[id] = fn
	return id
}

func (l *ErrorLog) RemoveListener(id int) {
	l.listenersMu.Lock()
	defer l.listenersMu.Unlock()
	delete(l.listeners, id)
}

func (l *ErrorLog) notifyListeners(entry *ErrorEntry) {
	l.listenersMu.RLock()
	defer l.listenersMu.RUnlock()
	for _, fn := range l.listeners {
		fn(entry)
	}
}
