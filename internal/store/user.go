package store

import (
	"context"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
)

type UserStore interface {
	GetUserById(ctx context.Context, id int) (*domain.User, error)
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	GetRolesForUser(ctx context.Context, username string) ([]domain.Role, error)
	UpdateUser(ctx context.Context, user *domain.User) error
	InsertUser(ctx context.Context, user *domain.User) error
}
