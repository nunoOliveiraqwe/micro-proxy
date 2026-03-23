package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/domain"
	"github.com/nunoOliveiraqwe/micro-proxy/internal/store"
)

// Ensure service implements interface.
var _ store.UserStore = (*UserStore)(nil)

type UserStore struct {
	db *DB
}

func NewUserStore(db *DB) store.UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) GetUserById(ctx context.Context, id int) (*domain.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	filter := domain.UserFilter{
		ID: &id,
	}

	a, err := findUserWithFilter(ctx, tx, &filter)
	if err != nil {
		return nil, err
	}
	if len(a) > 1 {
		return nil, fmt.Errorf("found more than one user with id %d", id)
	}
	if len(a) == 0 {
		return nil, nil
	}
	return a[0], nil
}

func (s *UserStore) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	filter := domain.UserFilter{
		Username: &username,
	}

	a, err := findUserWithFilter(ctx, tx, &filter)
	if err != nil {
		return nil, err
	}
	if len(a) > 1 {
		return nil, fmt.Errorf("found more than one user with username %s", username)
	}
	if len(a) == 0 {
		return nil, fmt.Errorf("no user found for username %s", username)
	}
	return a[0], nil
}

func (s *UserStore) GetRolesForUser(ctx context.Context, username string) ([]domain.Role, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT r.id, r.name
		FROM role r
		INNER JOIN user_role ur ON ur.role_id = r.id
		INNER JOIN users u ON u.id = ur.user_id
		WHERE u.username = ?`,
		username,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := make([]domain.Role, 0)
	for rows.Next() {
		var r domain.Role
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return roles, nil
}

func (s *UserStore) InsertUser(ctx context.Context, user *domain.User) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertUser(ctx, tx, user); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *UserStore) UpdateUser(ctx context.Context, user *domain.User) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := updateUser(ctx, tx, user); err != nil {
		return err
	}
	return tx.Commit()
}

func findUserWithFilter(ctx context.Context, tx *Tx, filter *domain.UserFilter) (_ []*domain.User, err error) {
	// Build WHERE clause.
	where, args := []string{"1 = 1"}, []interface{}{}
	if v := filter.ID; v != nil {
		where, args = append(where, "id = ?"), append(args, *v)
	}
	if v := filter.Username; v != nil {
		where, args = append(where, "username = ?"), append(args, *v)
	}

	// Execute query to fetch user rows.
	rows, err := tx.QueryContext(ctx, `
		SELECT 
		    id,
		    username,
		    password,
		    is_first_time_login,
		    active,
		    created_at,
		    updated_at 
		FROM user WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC
		`+FormatLimitOffset(filter.Limit, filter.Offset),
		args...,
	)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]*domain.User, 0)

	for rows.Next() {
		var c domain.User
		if err := rows.Scan(
			&c.ID,
			&c.Username,
			&c.Password,
			&c.IsFirstTimeLogin,
			&c.Active,
			&c.CreatedAt,
			&c.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func updateUser(ctx context.Context, tx *Tx, user *domain.User) error {
	// Execute  query.
	_, err := tx.ExecContext(ctx, `
		UPDATE user SET username = ?,password=?,active=?,is_first_time_login=?,updated_at=? WHERE Id = ?`,
		user.Username,
		user.Password,
		user.Active,
		user.IsFirstTimeLogin,
		time.Now().UTC().Truncate(time.Second),
		user.ID,
	)
	if err != nil {
		return err
	}

	return nil
}

func insertUser(ctx context.Context, tx *Tx, user *domain.User) error {
	// Execute  query.
	_, err := tx.ExecContext(ctx, `
		INSERT INTO user (id, username, password, active,is_first_time_login, created_at,updated_at) VALUES (?,?,?,?,?,?,?)`,
		user.ID,
		user.Username,
		user.Password,
		user.Active,
		user.IsFirstTimeLogin,
		time.Now().UTC().Truncate(time.Second),
		time.Now().UTC().Truncate(time.Second),
	)
	if err != nil {
		return err
	}

	return nil
}
