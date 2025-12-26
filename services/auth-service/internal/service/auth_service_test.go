package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Aadithya-J/code_nest/proto"
)

func TestAuthService_GenerateToken(t *testing.T) {
	service := &AuthService{}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	service.privateKey = privateKey

	token, err := service.GenerateToken("user-123", "test@example.com")

	require.NoError(t, err)
	assert.NotEmpty(t, token)

	parsed, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Method)
		}
		return &privateKey.PublicKey, nil
	})

	require.NoError(t, err)
	require.True(t, parsed.Valid)

	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, "user-123", claims["sub"])
}

func TestAuthService_ValidateToken(t *testing.T) {
	service := &AuthService{}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	service.privateKey = privateKey

	validToken, err := service.GenerateToken("user-123", "test@example.com")
	require.NoError(t, err)

	tests := []struct {
		name    string
		token   string
		wantVal bool
		wantUID string
	}{
		{
			name:    "valid token",
			token:   validToken,
			wantVal: true,
			wantUID: "user-123",
		},
		{
			name:    "empty token",
			token:   "",
			wantVal: false,
		},
		{
			name:    "invalid token",
			token:   "invalid.jwt.token",
			wantVal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &proto.ValidateTokenRequest{Token: tt.token}
			resp, err := service.ValidateToken(context.Background(), req)

			require.NoError(t, err)
			assert.Equal(t, tt.wantVal, resp.Valid)
			if tt.wantVal {
				assert.Equal(t, tt.wantUID, resp.UserId)
			}
		})
	}
}
