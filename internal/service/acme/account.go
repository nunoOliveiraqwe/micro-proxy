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

func (m *LegoAcmeManager) loadOrCreateAccount() error {
	existing, err := m.store.GetAccount(m.conf.Email)
	if err != nil {
		return fmt.Errorf("store lookup: %w", err)
	}
	if existing != nil {
		key, err := pemToECKey(existing.PrivateKey)
		if err != nil {
			return fmt.Errorf("parse stored key: %w", err)
		}
		var reg registration.Resource
		if existing.Registration != "" {
			if err := json.Unmarshal([]byte(existing.Registration), &reg); err != nil {
				return fmt.Errorf("parse stored registration: %w", err)
			}
		}

		m.user = &acmeUser{
			email:        existing.Email,
			registration: &reg,
			key:          key,
		}
		zap.S().Infof("acme: loaded existing account for %s", existing.Email)
		return nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	m.user = &acmeUser{email: m.conf.Email, key: key}
	zap.S().Infof("acme: generated new account key for %s", m.conf.Email)
	return nil
}

func (m *LegoAcmeManager) registerIfNeeded() error {
	if m.user.registration != nil && m.user.registration.URI != "" {
		return nil // already registered
	}

	reg, err := m.client.Registration.Register(registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	m.user.registration = reg

	if err := m.persistAccount(); err != nil {
		return fmt.Errorf("persist account: %w", err)
	}
	zap.S().Infof("acme: registered new account for %s (URI: %s)", m.conf.Email, reg.URI)
	return nil
}

func (m *LegoAcmeManager) persistAccount() error {
	ecKey, ok := m.user.key.(*ecdsa.PrivateKey)
	if !ok {
		return fmt.Errorf("account key is not ECDSA")
	}
	keyPEM, err := ecKeyToPEM(ecKey)
	if err != nil {
		return err
	}
	regJSON, err := json.Marshal(m.user.registration)
	if err != nil {
		return fmt.Errorf("marshal registration: %w", err)
	}
	return m.store.SaveAccount(&domain.AcmeAccount{
		Email:        m.conf.Email,
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
