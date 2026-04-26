package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/store"
)

var _ store.AcmeStore = (*AcmeStore)(nil)

type AcmeStore struct {
	db *DB
}

func NewAcmeStore(db *DB) store.AcmeStore {
	return &AcmeStore{db: db}
}

func (s *AcmeStore) GetConfiguration() (*domain.AcmeConfiguration, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var conf domain.AcmeConfiguration
	var intervalStr string
	var domainsStr string
	err = tx.QueryRowContext(ctx, `
		SELECT ID, EMAIL, DNS_PROVIDER, CA_DIR_URL, RENEWAL_CHECK_INTERVAL, ENABLED, DNS_PROVIDER_SERIALIZED_FIELDS, ACME_DOMAINS, CREATED_AT, UPDATED_AT
		FROM acme_configuration
		WHERE id = 1`,
	).Scan(
		&conf.ID,
		&conf.Email,
		&conf.DNSProvider,
		&conf.CADirURL,
		&intervalStr,
		&conf.Enabled,
		&conf.SerializedFields,
		&domainsStr,
		(*NullTime)(&conf.CreatedAt),
		(*NullTime)(&conf.UpdatedAt),
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dur, err := time.ParseDuration(intervalStr)
	if err != nil {
		dur = 12 * time.Hour
	}
	conf.RenewalCheckInterval = dur
	if domainsStr != "" {
		conf.Domains = strings.Split(domainsStr, ",")
	}
	return &conf, nil
}

func (s *AcmeStore) SaveConfiguration(conf *domain.AcmeConfiguration) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO acme_configuration (ID, EMAIL, DNS_PROVIDER, CA_DIR_URL, RENEWAL_CHECK_INTERVAL, ENABLED, DNS_PROVIDER_SERIALIZED_FIELDS, ACME_DOMAINS, UPDATED_AT)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			email                  = excluded.email,
			dns_provider           = excluded.dns_provider,
			ca_dir_url             = excluded.ca_dir_url,
			renewal_check_interval = excluded.renewal_check_interval,
			enabled                = excluded.enabled,
			dns_provider_serialized_fields            = excluded.dns_provider_serialized_fields,
			acme_domains           = excluded.acme_domains,
			updated_at             = CURRENT_TIMESTAMP`,
		conf.Email,
		conf.DNSProvider,
		conf.CADirURL,
		conf.RenewalCheckInterval.String(),
		conf.Enabled,
		conf.SerializedFields,
		strings.Join(conf.Domains, ","),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Account
// ---------------------------------------------------------------------------

func (s *AcmeStore) GetAccount(email string) (*domain.AcmeAccount, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var account domain.AcmeAccount
	err = tx.QueryRowContext(ctx, `
		SELECT ID, EMAIL, PRIVATE_KEY, REGISTRATION, CREATED_AT
		FROM ACME_ACCOUNT
		WHERE EMAIL = ?`, email,
	).Scan(
		&account.ID,
		&account.Email,
		&account.PrivateKey,
		&account.Registration,
		(*NullTime)(&account.CreatedAt),
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *AcmeStore) SaveAccount(account *domain.AcmeAccount) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO acme_account (ID, EMAIL, PRIVATE_KEY, REGISTRATION)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			email        = excluded.email,
			private_key  = excluded.private_key,
			registration = excluded.registration`,
		account.Email,
		account.PrivateKey,
		account.Registration,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AcmeStore) GetCertificate(domainName string) (*domain.AcmeCertificate, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var cert domain.AcmeCertificate
	err = tx.QueryRowContext(ctx, `
		SELECT id, domain, certificate, private_key, issuer_certificate, expires_at, created_at, updated_at
		FROM acme_certificate
		WHERE domain = ?`, domainName,
	).Scan(
		&cert.ID,
		&cert.Domain,
		&cert.Certificate,
		&cert.PrivateKey,
		&cert.IssuerCertificate,
		(*NullTime)(&cert.ExpiresAt),
		(*NullTime)(&cert.CreatedAt),
		(*NullTime)(&cert.UpdatedAt),
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

func (s *AcmeStore) SaveCertificate(cert *domain.AcmeCertificate) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO acme_certificate (domain, certificate, private_key, issuer_certificate, expires_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			certificate        = excluded.certificate,
			private_key        = excluded.private_key,
			issuer_certificate = excluded.issuer_certificate,
			expires_at         = excluded.expires_at,
			updated_at         = excluded.updated_at`,
		cert.Domain,
		cert.Certificate,
		cert.PrivateKey,
		cert.IssuerCertificate,
		cert.ExpiresAt.UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AcmeStore) ListCertificates() ([]*domain.AcmeCertificate, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, domain, certificate, private_key, issuer_certificate, expires_at, created_at, updated_at
		FROM acme_certificate`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []*domain.AcmeCertificate
	for rows.Next() {
		var cert domain.AcmeCertificate
		if err := rows.Scan(
			&cert.ID,
			&cert.Domain,
			&cert.Certificate,
			&cert.PrivateKey,
			&cert.IssuerCertificate,
			(*NullTime)(&cert.ExpiresAt),
			(*NullTime)(&cert.CreatedAt),
			(*NullTime)(&cert.UpdatedAt),
		); err != nil {
			return nil, err
		}
		certs = append(certs, &cert)
	}
	return certs, rows.Err()
}

func (s *AcmeStore) ResetAll() error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{"acme_certificate", "acme_account", "acme_configuration"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("failed to clear %s: %w", table, err)
		}
	}
	return tx.Commit()
}
