package sqlite

import (
	"context"
	"fmt"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/store"
)

// Ensure service implements interface.
var _ store.RoleStore = (*RoleStore)(nil)

type RoleStore struct {
	db *DB
}

func NewRoleStore(db *DB) store.RoleStore {
	return &RoleStore{db: db}
}

func (s *RoleStore) GetRoleById(ctx context.Context, id int) (*domain.Role, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var role domain.Role
	err = tx.QueryRowContext(ctx, `
		SELECT id, name
		FROM role
		WHERE id = ?`,
		id,
	).Scan(&role.ID, &role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get role by id %d: %w", id, err)
	}

	return &role, nil
}

func (s *RoleStore) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var role domain.Role
	err = tx.QueryRowContext(ctx, `
		SELECT id, name
		FROM role
		WHERE name = ?`,
		name,
	).Scan(&role.ID, &role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get role by name %s: %w", name, err)
	}

	return &role, nil
}
