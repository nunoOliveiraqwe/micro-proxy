package store

import (
	"github.com/nunoOliveiraqwe/torii/internal/domain"
)

type SystemConfigStore interface {
	GetSystemConfiguration() (*domain.SystemConfiguration, error)
	UpdateSystemConfiguration(config *domain.SystemConfiguration) error
}
