package app

import (
	"fmt"

	"go.uber.org/zap"
)

type SystemConfigurationService struct {
	dataStore *DataStore
}

func NewSystemConfigurationService(store *DataStore) *SystemConfigurationService {
	return &SystemConfigurationService{
		dataStore: store,
	}
}

func (s *SystemConfigurationService) IsFirstTimeSetupCompleted() bool {
	conf, err :=
		s.dataStore.SystemConfigStore.GetSystemConfiguration()
	if err != nil {
		zap.S().Errorf("Failed to get system configuration: %v", err)
		return true // we don't let any config be done if we can't fetch from the db, if the FTS is done, auth will be called for
	}
	zap.S().Debugf("System configuration fetched: %+v", conf)
	return conf.IsFirstTimeSetupConcluded
}

func (s *SystemConfigurationService) CompleteFistTimeSetup() error {
	conf, err :=
		s.dataStore.SystemConfigStore.GetSystemConfiguration()
	if err != nil {
		zap.S().Errorf("Failed to get system configuration: %v", err)
		return fmt.Errorf("failed to get system configuration: %w", err)
	}
	zap.S().Debugf("System configuration fetched: %+v", conf)
	conf.IsFirstTimeSetupConcluded = true
	err = s.dataStore.SystemConfigStore.UpdateSystemConfiguration(conf)
	if err != nil {
		zap.S().Errorf("Failed to update system configuration: %v", err)
		return fmt.Errorf("failed to update system configuration: %w", err)
	}
	return nil
}
