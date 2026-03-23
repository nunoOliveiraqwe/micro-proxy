package auth

import (
	"crypto/rand"

	"go.uber.org/zap"
)

type Encoder interface {
	Encrypt(salt []byte, pwd string) (string, error)
	Matches(pwd string, hashedPwd string) error
	GenerateSecureSalt() ([]byte, error)
}

type Argon2PasswordEncoder struct {
	argon2Hasher *Argon2Hasher
}

func (a *Argon2PasswordEncoder) GenerateSecureSalt() ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		zap.S().Errorf("Failed to generate salt: %v", err)
		return nil, err
	}
	return salt, nil
}

func (a *Argon2PasswordEncoder) Encrypt(salt []byte, pwd string) (string, error) {
	return a.argon2Hasher.Hash(pwd, salt)
}

func (a *Argon2PasswordEncoder) Matches(pwd string, hashedPwd string) error {
	return a.argon2Hasher.CompareHashAndPassword(hashedPwd, pwd)
}

func NewDefaultEncoder() Encoder {
	return &Argon2PasswordEncoder{
		argon2Hasher: NewArgon2Hasher(19, 64*1024, 3, 4, 32),
	}
}
