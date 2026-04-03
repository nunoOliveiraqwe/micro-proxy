package store

import (
	"context"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
)

type ProxyMetricsStore interface {
	GetGlobalProxyMetrics(ctx context.Context) (*domain.GlobalProxyMetrics, error)
	UpdateGlobalProxyMetrics(ctx context.Context, metrics *domain.GlobalProxyMetrics) error
}
