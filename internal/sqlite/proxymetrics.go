package sqlite

import (
	"context"
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/domain"
	"github.com/nunoOliveiraqwe/micro-proxy/internal/store"
)

// Ensure service implements interface.
var _ store.ProxyMetricsStore = (*ProxyMetricsStore)(nil)

type ProxyMetricsStore struct {
	db *DB
}

func NewProxyMetricsStore(db *DB) store.ProxyMetricsStore {
	return &ProxyMetricsStore{db: db}
}

func (s *ProxyMetricsStore) GetGlobalProxyMetrics(ctx context.Context) (*domain.GlobalProxyMetrics, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var m domain.GlobalProxyMetrics
	err = tx.QueryRowContext(ctx, `
		SELECT
			id,
			request_count,
			error_count,
			total_latency_ms,
			avg_response_time_ms,
			upstream_timeouts,
			bytes_sent,
			bytes_received,
			active_connections,
			request_2xx_count,
			request_3xx_count,
			request_4xx_count,
			request_5xx_count,
			cache_hits,
			cache_misses,
			ssl_handshakes,
			backend_errors,
			updated_at
		FROM global_proxy_metric
		WHERE id = 1`,
	).Scan(
		&m.ID,
		&m.RequestCount,
		&m.ErrorCount,
		&m.TotalLatencyMs,
		&m.AvgResponseTimeMs,
		&m.UpstreamTimeouts,
		&m.BytesSent,
		&m.BytesReceived,
		&m.ActiveConnections,
		&m.Request2xxCount,
		&m.Request3xxCount,
		&m.Request4xxCount,
		&m.Request5xxCount,
		&m.CacheHits,
		&m.CacheMisses,
		&m.SslHandshakes,
		&m.BackendErrors,
		&m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &m, nil
}

func (s *ProxyMetricsStore) UpdateGlobalProxyMetrics(ctx context.Context, metrics *domain.GlobalProxyMetrics) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := updateGlobalProxyMetrics(ctx, tx, metrics); err != nil {
		return err
	}
	return tx.Commit()
}

func updateGlobalProxyMetrics(ctx context.Context, tx *Tx, metrics *domain.GlobalProxyMetrics) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE global_proxy_metric SET
			request_count = ?,
			error_count = ?,
			total_latency_ms = ?,
			avg_response_time_ms = ?,
			upstream_timeouts = ?,
			bytes_sent = ?,
			bytes_received = ?,
			active_connections = ?,
			request_2xx_count = ?,
			request_3xx_count = ?,
			request_4xx_count = ?,
			request_5xx_count = ?,
			cache_hits = ?,
			cache_misses = ?,
			ssl_handshakes = ?,
			backend_errors = ?,
			updated_at = ?
		WHERE id = 1`,
		metrics.RequestCount,
		metrics.ErrorCount,
		metrics.TotalLatencyMs,
		metrics.AvgResponseTimeMs,
		metrics.UpstreamTimeouts,
		metrics.BytesSent,
		metrics.BytesReceived,
		metrics.ActiveConnections,
		metrics.Request2xxCount,
		metrics.Request3xxCount,
		metrics.Request4xxCount,
		metrics.Request5xxCount,
		metrics.CacheHits,
		metrics.CacheMisses,
		metrics.SslHandshakes,
		metrics.BackendErrors,
		time.Now().UTC().Truncate(time.Second),
	)
	return err
}
