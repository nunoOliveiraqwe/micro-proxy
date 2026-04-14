package domain

const systemConfigID = 1 // there is only one system configuration

type SystemConfiguration struct {
	ID                        int
	IsFirstTimeSetupConcluded bool
	ApiKeyHmacSecret          []byte // HMAC-SHA256 secret for API key hashing; auto-generated on first use
}
