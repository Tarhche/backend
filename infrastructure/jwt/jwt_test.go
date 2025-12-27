package jwt

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
)

func TestJWT(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	if err != nil {
		t.Error("unexpected error", err)
	}

	// For demonstration purposes, in a real scenario, you would typically use the public key from the corresponding private key.
	// Here, we extract the public key from the private key just for demonstration purposes.
	publicKey := privateKey.Public()

	JWTToken := NewJWT(privateKey, publicKey)

	t.Run("Verifying a valid token should pass", func(t *testing.T) {
		t.Parallel()

		now := time.Now()

		// Generate a jwt token
		claims := jwt.MapClaims{
			"sub":  "1234567890",
			"name": "John Doe",
			"iat":  float64(now.Unix()),
			"exp":  float64(now.Add(time.Hour * 1).Unix()), // Token expires in 1 hour
		}

		tokenString, err := JWTToken.Generate(claims)
		assert.NoError(t, err, "unexpected error")

		// Verify the jwt token
		tokenClaims, err := JWTToken.Verify(tokenString)
		assert.NoError(t, err, "unexpected error")

		// Access the claims
		got, ok := tokenClaims.(jwt.MapClaims)
		assert.True(t, ok, "claims are not valid")
		assert.Equal(t, claims, got, "expected and given claims does not match")
	})

	t.Run("Verifying an invalid token should fail", func(t *testing.T) {
		t.Parallel()

		// Generate a jwt token
		claims := jwt.MapClaims{
			"sub":  "1234567890",
			"name": "John Doe",
			"iat":  time.Now().Unix(),
			"exp":  time.Now().Add(time.Hour * -1).Unix(), // Token expired
		}

		tokenString, err := JWTToken.Generate(claims)
		assert.NoError(t, err, "unexpected error")

		// Verify the jwt token
		tokenClaims, err := JWTToken.Verify(tokenString)
		assert.ErrorIs(t, err, jwt.ErrTokenExpired)
		assert.Empty(t, tokenClaims)
	})
}
