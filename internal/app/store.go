package app

import (
	"github.com/nunoOliveiraqwe/torii/internal/sqlite"
	"github.com/nunoOliveiraqwe/torii/internal/store"
)

type DataStore struct {
	UserStore         store.UserStore
	RoleStore         store.RoleStore
	SystemConfigStore store.SystemConfigStore
	ProxyMetricsStore store.ProxyMetricsStore
	AcmeStore         store.AcmeStore
}

func NewDataStore(db *sqlite.DB) *DataStore {
	return &DataStore{
		UserStore:         sqlite.NewUserStore(db),
		RoleStore:         sqlite.NewRoleStore(db),
		SystemConfigStore: sqlite.NewSystemConfigStore(db),
		ProxyMetricsStore: sqlite.NewProxyMetricsStore(db),
		AcmeStore:         sqlite.NewAcmeStore(db),
	}
}
