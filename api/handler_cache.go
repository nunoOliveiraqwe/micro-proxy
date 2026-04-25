package api

import (
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/middleware"
)

type CacheInsightResponse struct {
	Name           string   `json:"name"`
	MaxEntries     int      `json:"max_entries"`
	CurrentEntries int      `json:"current_entries"`
	Keys           []string `json:"keys"`
	M1Rate         float64  `json:"m1_rate"`
	Hits           int64    `json:"hits"`
	InsertionTotal int64    `json:"insertion_total"`
}

func handleGetCacheInsights(service app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching cache insights")
		mgr := service.GetCacheInsightManager()

		caches := mgr.GetCaches()
		results := make([]CacheInsightResponse, 0, len(caches))
		for _, c := range caches {
			results = append(results, CacheInsightResponse{
				Name:           c.CacheName(),
				MaxEntries:     c.GetMaxLen(),
				CurrentEntries: c.GetCurrentNumberOfElements(),
				Keys:           c.GetAllKeys(),
				M1Rate:         c.CacheInsertionM1Rate(),
				Hits:           c.CacheHits(),
				InsertionTotal: c.CacheInsertionTotal(),
			})
		}
		WriteResponseAsJSON(results, w)
	}
}
