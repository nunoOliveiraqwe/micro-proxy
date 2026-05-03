package ctxkeys

type Key int

const (
	Port Key = iota
	Host
	Path
	ServerID
	MetricsMgr
	RequestID
	Logger
	CountryCode
	ContinentCode
	BlockInfo
	CacheInsightMgr
	OverrideMetricsName
)
