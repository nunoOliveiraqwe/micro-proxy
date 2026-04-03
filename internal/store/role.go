package store

import (
	"context"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
)

type RoleStore interface {
	GetRoleById(ctx context.Context, id int) (*domain.Role, error)
	GetRoleByName(ctx context.Context, name string) (*domain.Role, error)
}
