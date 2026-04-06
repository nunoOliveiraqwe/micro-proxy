package store

import "github.com/nunoOliveiraqwe/torii/internal/domain"

type AcmeStore interface {
	GetConfiguration() (*domain.AcmeConfiguration, error)
	SaveConfiguration(conf *domain.AcmeConfiguration) error
	GetAccount(email string) (*domain.AcmeAccount, error)
	SaveAccount(account *domain.AcmeAccount) error
	GetCertificate(domainName string) (*domain.AcmeCertificate, error)
	SaveCertificate(cert *domain.AcmeCertificate) error
	ListCertificates() ([]*domain.AcmeCertificate, error)
	ResetAll() error
}
