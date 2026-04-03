package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nunoOliveiraqwe/torii/internal/auth"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// hashPassword is a test helper that hashes a cleartext password using
// the same encoder the application uses.
func hashPassword(t *testing.T, cleartext string) string {
	t.Helper()
	enc := auth.NewDefaultEncoder()
	salt, err := enc.GenerateSecureSalt()
	if err != nil {
		t.Fatalf("generate salt: %v", err)
	}
	h, err := enc.Encrypt(salt, cleartext)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return h
}

func TestHandleLogin_Success(t *testing.T) {
	f := newTestFixture(t)
	hashed := hashPassword(t, "Secret1!")

	f.userStore.On("GetUserByUsername", mock.Anything, "admin").
		Return(&domain.User{ID: 1, Username: "admin", Password: hashed}, nil)

	body := LoginRequest{Username: "admin", Password: "Secret1!"}
	req := newJSONRequest(t, http.MethodPost, "/api/v1/auth/login", body)

	handler := handleLogin(f.svc)
	rec := serveWithSession(f, handler, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	f.userStore.AssertExpectations(t)
}

func TestHandleLogin_InvalidJSON(t *testing.T) {
	f := newTestFixture(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	handler := handleLogin(f.svc)
	rec := serveWithSession(f, handler, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleLogin_UserNotFound(t *testing.T) {
	f := newTestFixture(t)

	f.userStore.On("GetUserByUsername", mock.Anything, "nonexistent").
		Return(nil, errors.New("user not found"))

	body := LoginRequest{Username: "nonexistent", Password: "Secret1!"}
	req := newJSONRequest(t, http.MethodPost, "/api/v1/auth/login", body)

	handler := handleLogin(f.svc)
	rec := serveWithSession(f, handler, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	f.userStore.AssertExpectations(t)
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	f := newTestFixture(t)
	hashed := hashPassword(t, "Secret1!")

	f.userStore.On("GetUserByUsername", mock.Anything, "admin").
		Return(&domain.User{ID: 1, Username: "admin", Password: hashed}, nil)

	body := LoginRequest{Username: "admin", Password: "WrongPassword1!"}
	req := newJSONRequest(t, http.MethodPost, "/api/v1/auth/login", body)

	handler := handleLogin(f.svc)
	rec := serveWithSession(f, handler, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	f.userStore.AssertExpectations(t)
}

func TestHandleLogin_EmptyBody(t *testing.T) {
	f := newTestFixture(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", http.NoBody)
	handler := handleLogin(f.svc)
	rec := serveWithSession(f, handler, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
