package service

import (
	"context"
	"fmt"

	"github.com/nunoOliveiraqwe/torii/internal/auth"
	"go.uber.org/zap"
)

type UserService struct {
	dataStore       *DataStore
	passwordEncoder auth.Encoder
	validator       PasswordValidator
}

func NewUserService(dataStore *DataStore) *UserService {
	return &UserService{
		dataStore:       dataStore,
		passwordEncoder: auth.NewDefaultEncoder(),
		validator:       NewDefaultPasswordValidator(),
	}
}

func (s *UserService) PasswordMatchesForUser(password, username string) error {
	u, err := s.dataStore.UserStore.GetUserByUsername(context.Background(), username)
	if err != nil {
		return fmt.Errorf("failed to get user by username: %w", err)
	}
	err = s.passwordEncoder.Matches(password, u.Password)
	if err != nil {
		return fmt.Errorf("password does not match: %w", err)
	}
	return nil
}

func (s *UserService) SetPasswordForUser(password, username string) error {
	zap.S().Debugf("Setting password for user %s", username)
	err := s.validator.IsValidPassword(password)
	if err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}

	u, err := s.dataStore.UserStore.GetUserByUsername(context.Background(), username)
	if err != nil {
		return fmt.Errorf("failed to get user by username: %w", err)
	}
	hashedPassword, err := s.passwordEncoder.Encrypt(password)
	if err != nil {
		return fmt.Errorf("failed to encode password: %w", err)
	}
	u.Password = hashedPassword
	err = s.dataStore.UserStore.UpdateUser(context.Background(), u)
	if err != nil {
		return fmt.Errorf("failed to update user with new password: %w", err)
	}
	return nil
}
