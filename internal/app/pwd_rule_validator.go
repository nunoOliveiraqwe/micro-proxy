package app

import "unicode"

type PasswordValidator interface {
	IsValidPassword(password string) error
}

type DefaultPasswordValidator struct{}

func NewDefaultPasswordValidator() PasswordValidator {
	return &DefaultPasswordValidator{}
}

func (d *DefaultPasswordValidator) IsValidPassword(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	var hasUpper, hasNumber, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsNumber(ch):
			hasNumber = true
		case !unicode.IsLetter(ch) && !unicode.IsNumber(ch):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return ErrMissingUpper
	}
	if !hasNumber {
		return ErrMissingNumber
	}
	if !hasSpecial {
		return ErrMissingSpecial
	}

	return nil
}

var ErrPasswordTooShort = &PasswordValidationError{"Password must be at least 8 characters long"}
var ErrMissingUpper = &PasswordValidationError{"Password must contain at least one uppercase letter"}
var ErrMissingNumber = &PasswordValidationError{"Password must contain at least one number"}
var ErrMissingSpecial = &PasswordValidationError{"Password must contain at least one special character"}

type PasswordValidationError struct {
	Message string
}

func (e *PasswordValidationError) Error() string {
	return e.Message
}
