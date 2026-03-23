package app

type ServiceStore struct {
	userService                *UserService
	systemConfigurationService *SystemConfigurationService
}

func NewServiceStore(store *DataStore) *ServiceStore {
	return &ServiceStore{
		userService:                NewUserService(store),
		systemConfigurationService: NewSystemConfigurationService(store),
	}
}

func (s *ServiceStore) GetUserService() *UserService {
	return s.userService
}

func (s *ServiceStore) GetSystemConfigurationService() *SystemConfigurationService {
	return s.systemConfigurationService
}
