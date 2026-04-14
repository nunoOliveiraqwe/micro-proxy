package sqlite

import (
	"context"
	"crypto/rand"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"go.uber.org/zap"
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
			first_time_setup_complete,
			api_key_hmac_secret
		FROM system_configuration
		WHERE id = 1`,
	).Scan(
		&config.ID,
		&config.IsFirstTimeSetupConcluded,
		&config.ApiKeyHmacSecret,
	)
	if err != nil {
		return nil, err
	}
	if len(config.ApiKeyHmacSecret) == 0 {
		zap.S().Info("API key HMAC secret not found, generating new one")
		if err := s.createHMacSecret(&config); err != nil {
			return nil, err
		}
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

// createHMacSecret generates a 32-byte random secret and persists it.
// Uses its own transaction so the write is committed independently.
// Called only once — on first access when the column is still NULL.
func (s *SystemConfigStore) createHMacSecret(config *domain.SystemConfiguration) error {
	if config == nil {
		zap.S().Errorf("System configuration is nil, cannot create HMAC secret")
		return nil
	}
	if len(config.ApiKeyHmacSecret) > 0 {
		zap.S().Errorf("API key HMAC secret already exists, skipping generation")
		return nil
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		zap.S().Errorf("Failed to generate API key HMAC secret: %v", err)
		return err
	}

	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE system_configuration SET
			api_key_hmac_secret = ?
		WHERE id = 1 AND api_key_hmac_secret IS NULL`,
		secret,
	)
	if err != nil {
		zap.S().Errorf("Failed to persist HMAC secret: %v", err)
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	config.ApiKeyHmacSecret = secret
	return nil
}
