package store

import "github.com/nunoOliveiraqwe/torii/internal/domain"

type AcmeStore interface {
	GetConfigurations() ([]*domain.AcmeConfiguration, error)
	GetConfiguration(id int) (*domain.AcmeConfiguration, error)
	SaveConfiguration(conf *domain.AcmeConfiguration) error
	DeleteConfiguration(id int) error

	GetAccountFor(email string) (*domain.AcmeAccount, error)
	SaveAccount(account *domain.AcmeAccount) error

	GetCertificate(domainName string) (*domain.AcmeCertificate, error)
	SaveCertificate(cert *domain.AcmeCertificate) error
	ListCertificates() ([]*domain.AcmeCertificate, error)
	ResetAll() error
}
