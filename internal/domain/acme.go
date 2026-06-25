package domain

import "time"

// AcmeConfiguration holds the user-managed ACME settings stored in the DB.
// There is at most one active configuration row (singleton, ID = 1).
type AcmeConfiguration struct {
	ID                   int
	Email                string
	DNSProvider          string
	SerializedFields     []byte
	CADirURL             string
	RenewalCheckInterval time.Duration // how often the renewal loop ticks
	Enabled              bool
	Domains              []string
	DNSResolvers         []string
	// AutoDiscover, when true, asks this configuration to also obtain
	// certificates for any host the proxy currently serves. At most one
	// configuration in the system may have this enabled.
	AutoDiscover bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (a *AcmeConfiguration) IsValid() bool {
	return a.Email != "" && a.DNSProvider != "" && len(a.SerializedFields) > 0 && a.CADirURL != ""
}

type AcmeAccount struct {
	ID           int
	Email        string
	PrivateKey   []byte
	Registration string
	CreatedAt    time.Time
}

type AcmeCertificate struct {
	ID                int
	Domain            string
	Certificate       []byte
	PrivateKey        []byte
	IssuerCertificate []byte
	ExpiresAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
