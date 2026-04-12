package util

import "sync"

type BufferListenerFunc[T any] func(entry *T)

type RingBuffer[T any] struct { //ring buffer
	mu       sync.RWMutex
	entries  []T
	pos      int
	count    int
	capacity int

	listenersMu    sync.RWMutex
	listeners      map[int]BufferListenerFunc[T]
	nextListenerID int
}

func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	return &RingBuffer[T]{
		entries:   make([]T, capacity),
		capacity:  capacity,
		listeners: make(map[int]BufferListenerFunc[T]),
	}
}

func (l *RingBuffer[T]) Add(entry T) {
	l.mu.Lock()
	l.entries[l.pos] = entry
	l.pos = (l.pos + 1) % l.capacity
	if l.count < l.capacity {
		l.count++
	}
	l.mu.Unlock()
	l.notifyListeners(&entry)
}

func (l *RingBuffer[T]) Recent(n int) []T {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if n > l.count {
		n = l.count
	}
	if n == 0 {
		return []T{}
	}

	result := make([]T, n)
	for i := 0; i < n; i++ {
		idx := (l.pos - 1 - i + l.capacity) % l.capacity
		result[i] = l.entries[idx]
	}
	return result
}

func (l *RingBuffer[T]) AddListener(fn BufferListenerFunc[T]) int {
	l.listenersMu.Lock()
	defer l.listenersMu.Unlock()
	id := l.nextListenerID
	l.nextListenerID++
	l.listeners[id] = fn
	return id
}

func (l *RingBuffer[T]) RemoveListener(id int) {
	l.listenersMu.Lock()
	defer l.listenersMu.Unlock()
	delete(l.listeners, id)
}

func (l *RingBuffer[T]) notifyListeners(entry *T) {
	l.listenersMu.RLock()
	defer l.listenersMu.RUnlock()
	for _, fn := range l.listeners {
		fn(entry)
	}
}
