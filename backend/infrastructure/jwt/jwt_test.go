package jwt

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"golang.org/x/exp/maps"
)

func TestJWT(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	if err != nil {
		t.Error("unexpected error", err)
	}

	// For demonstration purposes, in a real scenario, you would typically use the public key from the corresponding private key.
	// Here, we extract the public key from the private key just for demonstration purposes.
	publicKey := privateKey.Public()

	// Generate a JWT token
	claims := jwt.MapClaims{
		"sub":  "1234567890",
		"name": "John Doe",
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(time.Hour * 1).Unix(), // Token expires in 1 hour
	}

	JWTToken := NewJWT(privateKey, publicKey)

	tokenString, err := JWTToken.Generate(claims)
	if err != nil {
		t.Error("unexpected error", err)
	}

	// Verify the JWT token
	tokenClaims, err := JWTToken.Verify(tokenString)
	if err != nil {
		t.Error("unexpected error", err)
	}

	// Access the claims
	got, ok := tokenClaims.(jwt.MapClaims)
	if !ok {
		t.Error("claims are not valid")
	}

	if maps.Equal(claims, got) {
		t.Error("expected and given claims does not match")
	}
}
