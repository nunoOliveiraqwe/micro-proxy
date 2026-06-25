package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"

	"github.com/go-acme/lego/v4/registration"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"go.uber.org/zap"
)

type acmeUser struct {
	email        string
	registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *acmeUser) GetEmail() string                        { return u.email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

func (m *LegoAcmeManager) loadOrCreateAccount(acmeConf *domain.AcmeConfiguration) error {
	account, err := m.store.GetAccountFor(acmeConf.Email)
	if err != nil {
		return fmt.Errorf("could not load existing acme account: %w", err)
	}
	if account != nil {
		key, err := pemToECKey(account.PrivateKey)
		if err != nil {
			return fmt.Errorf("could not parse acme stored key: %w", err)
		}
		var reg registration.Resource
		if account.Registration != "" {
			if err := json.Unmarshal([]byte(account.Registration), &reg); err != nil {
				return fmt.Errorf("could not parse acme stored registration: %w", err)
			}
		}

		m.user = &acmeUser{
			email:        account.Email,
			registration: &reg,
			key:          key,
		}
		zap.S().Infof("loaded existing acme account for %s", account.Email)
		return nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	m.user = &acmeUser{email: acmeConf.Email, key: key}
	zap.S().Infof("generated new acme account key for %s", acmeConf.Email)
	return nil
}

func (m *LegoAcmeManager) registerIfNeeded(acmeConf *domain.AcmeConfiguration) error {
	if m.user.registration != nil && m.user.registration.URI != "" {
		return nil // already registered
	}

	reg, err := m.client.Registration.Register(registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		return fmt.Errorf("cannot register acme client: %w", err)
	}
	m.user.registration = reg

	if err := m.persistAccount(acmeConf); err != nil {
		return fmt.Errorf("cannot persist acme account: %w", err)
	}

	zap.S().Infof("registered new acme account for %s (URI: %s)", acmeConf.Email, reg.URI)
	return nil
}

func (m *LegoAcmeManager) persistAccount(acmeConf *domain.AcmeConfiguration) error {
	ecKey, ok := m.user.key.(*ecdsa.PrivateKey)
	if !ok {
		return fmt.Errorf("acme account key is not ECDSA")
	}
	keyPEM, err := ecKeyToPEM(ecKey)
	if err != nil {
		return err
	}
	regJSON, err := json.Marshal(m.user.registration)
	if err != nil {
		return fmt.Errorf("cannot marshal acme registration: %w", err)
	}
	return m.store.SaveAccount(&domain.AcmeAccount{
		Email:        acmeConf.Email,
		PrivateKey:   keyPEM,
		Registration: string(regJSON),
	})
}

func pemToECKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func ecKeyToPEM(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal ec key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}
