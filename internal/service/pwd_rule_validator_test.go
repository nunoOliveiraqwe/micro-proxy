package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultPasswordValidator_IsValidPassword(t *testing.T) {
	v := &DefaultPasswordValidator{}

	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{"valid password", "Secret1!", nil},
		{"too short", "Ab1!", ErrPasswordTooShort},
		{"missing uppercase", "secret1!", ErrMissingUpper},
		{"missing number", "Secret!!", ErrMissingNumber},
		{"missing special", "Secret11", ErrMissingSpecial},
		{"exactly 8 chars valid", "Abcdef1!", nil},
		{"empty password", "", ErrPasswordTooShort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.IsValidPassword(tt.password)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tt.wantErr, err)
			}
		})
	}
}
