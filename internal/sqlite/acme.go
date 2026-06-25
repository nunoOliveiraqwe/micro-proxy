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

const acmeConfigColumns = `ID, EMAIL, DNS_PROVIDER, CA_DIR_URL, RENEWAL_CHECK_INTERVAL, ENABLED, DNS_PROVIDER_SERIALIZED_FIELDS, ACME_DOMAINS, DNS_RESOLVERS, AUTO_DISCOVER, CREATED_AT, UPDATED_AT`

func (s *AcmeStore) GetConfigurations() ([]*domain.AcmeConfiguration, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT `+acmeConfigColumns+` FROM acme_configuration ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.AcmeConfiguration
	for rows.Next() {
		conf, err := scanAcmeConfiguration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, conf)
	}
	return out, rows.Err()
}

func (s *AcmeStore) GetConfiguration(id int) (*domain.AcmeConfiguration, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `SELECT `+acmeConfigColumns+` FROM acme_configuration WHERE id = ?`, id)
	conf, err := scanAcmeConfiguration(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return conf, nil
}

func (s *AcmeStore) SaveConfiguration(conf *domain.AcmeConfiguration) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	domainsCSV := strings.Join(conf.Domains, ",")
	resolversCSV := strings.Join(conf.DNSResolvers, ",")
	intervalStr := conf.RenewalCheckInterval.String()

	if conf.ID == 0 {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO acme_configuration (EMAIL, DNS_PROVIDER, CA_DIR_URL, RENEWAL_CHECK_INTERVAL, ENABLED, DNS_PROVIDER_SERIALIZED_FIELDS, ACME_DOMAINS, DNS_RESOLVERS, AUTO_DISCOVER, UPDATED_AT)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			conf.Email,
			conf.DNSProvider,
			conf.CADirURL,
			intervalStr,
			conf.Enabled,
			conf.SerializedFields,
			domainsCSV,
			resolversCSV,
			conf.AutoDiscover,
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		conf.ID = int(id)
	} else {
		_, err := tx.ExecContext(ctx, `
			UPDATE acme_configuration SET
				email                          = ?,
				dns_provider                   = ?,
				ca_dir_url                     = ?,
				renewal_check_interval         = ?,
				enabled                        = ?,
				dns_provider_serialized_fields = ?,
				acme_domains                   = ?,
				dns_resolvers                  = ?,
				auto_discover                  = ?,
				updated_at                     = CURRENT_TIMESTAMP
			WHERE id = ?`,
			conf.Email,
			conf.DNSProvider,
			conf.CADirURL,
			intervalStr,
			conf.Enabled,
			conf.SerializedFields,
			domainsCSV,
			resolversCSV,
			conf.AutoDiscover,
			conf.ID,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *AcmeStore) DeleteConfiguration(id int) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM acme_configuration WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// scanAcmeConfiguration scans either a *sql.Row or *sql.Rows because both expose Scan.
func scanAcmeConfiguration(scanner interface {
	Scan(dest ...any) error
}) (*domain.AcmeConfiguration, error) {
	var conf domain.AcmeConfiguration
	var intervalStr, domainsStr, dnsResolversStr string
	err := scanner.Scan(
		&conf.ID,
		&conf.Email,
		&conf.DNSProvider,
		&conf.CADirURL,
		&intervalStr,
		&conf.Enabled,
		&conf.SerializedFields,
		&domainsStr,
		&dnsResolversStr,
		&conf.AutoDiscover,
		(*NullTime)(&conf.CreatedAt),
		(*NullTime)(&conf.UpdatedAt),
	)
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
	if dnsResolversStr != "" {
		conf.DNSResolvers = strings.Split(dnsResolversStr, ",")
	}
	return &conf, nil
}

// ---------------------------------------------------------------------------
// Account
// ---------------------------------------------------------------------------

func (s *AcmeStore) GetAccountFor(email string) (*domain.AcmeAccount, error) {
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
		INSERT INTO acme_account (EMAIL, PRIVATE_KEY, REGISTRATION)
		VALUES (?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET
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
