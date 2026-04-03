package sqlite

import (
	"context"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/store"
)

// Ensure service implements interface.
var _ store.SystemConfigStore = (*SystemConfigStore)(nil)

type SystemConfigStore struct {
	db *DB
}

func NewSystemConfigStore(db *DB) store.SystemConfigStore {
	return &SystemConfigStore{db: db}
}

func (s *SystemConfigStore) GetSystemConfiguration() (*domain.SystemConfiguration, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var config domain.SystemConfiguration
	err = tx.QueryRowContext(ctx, `
		SELECT
			id,
			first_time_setup_complete
		FROM system_configuration
		WHERE id = 1`,
	).Scan(
		&config.ID,
		&config.IsFirstTimeSetupConcluded,
	)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func (s *SystemConfigStore) UpdateSystemConfiguration(config *domain.SystemConfiguration) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := updateSystemConfiguration(ctx, tx, config); err != nil {
		return err
	}
	return tx.Commit()
}

func updateSystemConfiguration(ctx context.Context, tx *Tx, config *domain.SystemConfiguration) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE system_configuration SET
			first_time_setup_complete = ?
		WHERE id = 1`,
		config.IsFirstTimeSetupConcluded,
	)
	return err
}
