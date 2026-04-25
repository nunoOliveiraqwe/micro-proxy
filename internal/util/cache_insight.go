package util

const CacheInsightKey = "cache-insight-mgr"

type CacheInsight interface {
	CacheName() string
	GetMaxLen() int
	GetCurrentNumberOfElements() int
	GetAllKeys() []string
	CacheInsertionM1Rate() float64
	CacheInsertionM5Rate() float64
	CacheInsertionM15Rate() float64
	CacheInsertionTotal() int64
	CacheHits() int64
	CacheMisses() int64
	//DeleteEntryForKey(key string) error
}

type CacheInsightManager struct {
	caches []CacheInsight
}

func NewCacheInsightManager() *CacheInsightManager {
	return &CacheInsightManager{
		caches: []CacheInsight{},
	}
}

func (m *CacheInsightManager) RegisterCache(cache CacheInsight) {
	m.caches = append(m.caches, cache)
}

func (m *CacheInsightManager) GetCaches() []CacheInsight {
	return m.caches
}
