package app

import (
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"github.com/nunoOliveiraqwe/torii/proxy"
	"go.uber.org/zap"
)

type ServiceStore struct {
	apiKeyService              *ApiKeyService
	userService                *UserService
	systemConfigurationService *SystemConfigurationService
	acmeService                *AcmeService
	acmeStore                  store.AcmeStore
}

func NewServiceStore(ds *DataStore, reloadAcme func() error, getProxies func() []*proxy.ProxySnapshot) *ServiceStore {
	conf, err := ds.SystemConfigStore.GetSystemConfiguration()
	if err != nil {
		zap.S().Errorf("Failed to get system configuration: %v", err)
		return nil
	}
	return &ServiceStore{
		apiKeyService:              NewApiKeyService(ds.ApiKeyStore, conf.ApiKeyHmacSecret),
		userService:                NewUserService(ds),
		systemConfigurationService: NewSystemConfigurationService(ds),
		acmeService:                NewAcmeService(ds.AcmeStore, reloadAcme, getProxies),
		acmeStore:                  ds.AcmeStore,
	}
}

func (s *ServiceStore) GetUserService() *UserService {
	return s.userService
}

func (s *ServiceStore) GetSystemConfigurationService() *SystemConfigurationService {
	return s.systemConfigurationService
}

func (s *ServiceStore) GetAcmeStore() store.AcmeStore {
	return s.acmeStore
}

func (s *ServiceStore) GetApiKeyService() *ApiKeyService {
	return s.apiKeyService
}

func (s *ServiceStore) GetAcmeService() *AcmeService {
	return s.acmeService
}
