package util

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const rateTickInterval = 5 * time.Second

var (
	m1Alpha  = 1 - math.Exp(-5.0/60.0)
	m5Alpha  = 1 - math.Exp(-5.0/300.0)
	m15Alpha = 1 - math.Exp(-5.0/900.0)
)

type Tracker interface {
	Mark(n int64)
	MarkHit()
	MarkMiss()
	M1Rate() float64
	M5Rate() float64
	M15Rate() float64
	Total() int64
	Hits() int64
	Misses() int64
	Stop()
}
type RateTracker struct {
	uncounted atomic.Int64
	total     atomic.Int64
	hits      atomic.Int64
	misses    atomic.Int64
	m1Rate    float64
	m5Rate    float64
	m15Rate   float64
	init      bool
	mu        sync.Mutex
	stopCh    chan struct{}
}

func NewRateTracker() Tracker {
	rt := &RateTracker{
		stopCh: make(chan struct{}),
	}
	rt.startTick()
	return rt
}

func NewNopRateTracker() Tracker {
	return &NoOpTracker{}
}

func (rt *RateTracker) Mark(n int64) {
	rt.uncounted.Add(n)
	rt.total.Add(n)
}

func (rt *RateTracker) MarkHit() {
	rt.hits.Add(1)
}

func (rt *RateTracker) MarkMiss() {
	rt.misses.Add(1)
}

func (rt *RateTracker) M1Rate() float64 {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.m1Rate
}

func (rt *RateTracker) M5Rate() float64 {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.m5Rate
}

func (rt *RateTracker) M15Rate() float64 {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.m15Rate
}

func (rt *RateTracker) Total() int64 {
	return rt.total.Load()
}

func (rt *RateTracker) Hits() int64 {
	return rt.hits.Load()
}

func (rt *RateTracker) Misses() int64 {
	return rt.misses.Load()
}

func (rt *RateTracker) Stop() {
	close(rt.stopCh)
}

func (rt *RateTracker) startTick() {
	ticker := time.NewTicker(rateTickInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-rt.stopCh:
				return
			case <-ticker.C:
				rt.tick()
			}
		}
	}()
}

func (rt *RateTracker) tick() {
	count := rt.uncounted.Swap(0)
	instantRate := float64(count) / rateTickInterval.Seconds()

	rt.mu.Lock()
	defer rt.mu.Unlock()

	if !rt.init {
		rt.m1Rate = instantRate
		rt.m5Rate = instantRate
		rt.m15Rate = instantRate
		rt.init = true
		return
	}
	rt.m1Rate += m1Alpha * (instantRate - rt.m1Rate)
	rt.m5Rate += m5Alpha * (instantRate - rt.m5Rate)
	rt.m15Rate += m15Alpha * (instantRate - rt.m15Rate)
}

type NoOpTracker struct{}

func (t *NoOpTracker) Mark(n int64)     {}
func (t *NoOpTracker) MarkHit()         {}
func (t *NoOpTracker) MarkMiss()        {}
func (t *NoOpTracker) M1Rate() float64  { return 0 }
func (t *NoOpTracker) M5Rate() float64  { return 0 }
func (t *NoOpTracker) M15Rate() float64 { return 0 }
func (t *NoOpTracker) Total() int64     { return 0 }
func (t *NoOpTracker) Hits() int64      { return 0 }
func (t *NoOpTracker) Misses() int64    { return 0 }
func (t *NoOpTracker) Stop()            {}
