package ctxkeys

type Key int

const (
	Port Key = iota
	Host
	Path
	ServerID
	MetricsMgr
	OverrideMetricsName
	ContextStruct
	CacheInsightMgr
)
