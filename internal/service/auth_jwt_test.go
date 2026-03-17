package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthServiceParseTokenReturnsClaims(t *testing.T) {
	store := newAuthTestStore()
	service := NewAuthService(store, "test-secret-123456789012345678901234567890", 24)

	result, err := service.SignUp(context.Background(), "user@example.com", "password123", "Oak")
	require.NoError(t, err)

	claims, err := service.ParseToken(result.Token)
	require.NoError(t, err)
	assert.Equal(t, result.User.ID, claims.UserID)
	assert.Equal(t, result.User.Email, claims.Email)
	assert.True(t, claims.ExpiresAt.After(claims.IssuedAt))
}

func TestAuthServiceParseTokenRejectsTamperedToken(t *testing.T) {
	store := newAuthTestStore()
	service := NewAuthService(store, "test-secret-123456789012345678901234567890", 24)

	result, err := service.SignUp(context.Background(), "user@example.com", "password123", "Oak")
	require.NoError(t, err)

	parts := strings.Split(result.Token, ".")
	require.Len(t, parts, 3)
	tampered := parts[0] + "." + parts[1] + ".invalid"

	_, err = service.ParseToken(tampered)
	require.Error(t, err)
}

func TestAuthServiceParseTokenRejectsExpiredToken(t *testing.T) {
	store := newAuthTestStore()
	service := NewAuthService(store, "test-secret-123456789012345678901234567890", -1)

	result, err := service.SignUp(context.Background(), "user@example.com", "password123", "Oak")
	require.NoError(t, err)

	_, err = service.ParseToken(result.Token)
	require.Error(t, err)
}
