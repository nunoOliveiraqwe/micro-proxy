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
	CreatedAt            time.Time
	UpdatedAt            time.Time
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
