package metrics

import "sort"

const latencyBufferSize = 1000

type latencyRing struct {
	data  []int64
	pos   int
	count int
	cap   int
}

func newLatencyRing(capacity int) *latencyRing {
	return &latencyRing{
		data: make([]int64, capacity),
		cap:  capacity,
	}
}

func (r *latencyRing) Add(value int64) {
	r.data[r.pos] = value
	r.pos = (r.pos + 1) % r.cap
	if r.count < r.cap {
		r.count++
	}
}

func (r *latencyRing) Percentile(p float64) int64 {
	if r.count == 0 {
		return 0
	}
	sorted := make([]int64, r.count)
	if r.count < r.cap {
		copy(sorted, r.data[:r.count])
	} else {
		copy(sorted, r.data)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(r.count-1) * p / 100.0)
	if idx >= r.count {
		idx = r.count - 1
	}
	return sorted[idx]
}

func (r *latencyRing) reset() {
	r.pos = 0
	r.count = 0
}
