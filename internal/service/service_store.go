package service

import (
	"github.com/nunoOliveiraqwe/torii/config"
	"go.uber.org/zap"
)

type ServiceStore struct {
	apiKeyService              *ApiKeyService
	userService                *UserService
	systemConfigurationService *SystemConfigurationService
	acmeService                *AcmeService
}

func NewServiceStore(ds *DataStore, acmeConf *config.AcmeConfig) *ServiceStore {
	conf, err := ds.SystemConfigStore.GetSystemConfiguration()
	if err != nil {
		zap.S().Errorf("Failed to get system configuration: %v", err)
		return nil
	}
	return &ServiceStore{
		apiKeyService:              NewApiKeyService(ds.ApiKeyStore, conf.ApiKeyHmacSecret),
		userService:                NewUserService(ds),
		systemConfigurationService: NewSystemConfigurationService(ds),
		acmeService:                NewAcmeService(ds.AcmeStore, acmeConf),
	}
}

func (s *ServiceStore) GetUserService() *UserService {
	return s.userService
}

func (s *ServiceStore) GetSystemConfigurationService() *SystemConfigurationService {
	return s.systemConfigurationService
}

func (s *ServiceStore) GetApiKeyService() *ApiKeyService {
	return s.apiKeyService
}

func (s *ServiceStore) GetAcmeService() *AcmeService {
	return s.acmeService
}
