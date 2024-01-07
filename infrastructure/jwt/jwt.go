package jwt

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidAlgorithm        = errors.New("invalid algorithm")
	ErrInvalidToken            = errors.New("invalid token")
	ErrInvalidClaims           = errors.New("invalid claims")
	ErrUnexpectedSigningMethod = errors.New("unexpected signing method")
)

type JWT struct {
	privateKey crypto.PrivateKey
	publicKey  crypto.PublicKey
}

func NewJWT(privateKey crypto.PrivateKey, publicKey crypto.PublicKey) *JWT {
	return &JWT{
		privateKey: privateKey,
		publicKey:  publicKey,
	}
}

func (t *JWT) Generate(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodES512, claims)

	return token.SignedString(t.privateKey)
}

func (t *JWT) Verify(tokenString string) (jwt.Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Check if the signing method is the expected ECDSA P-521 with SHA-512
		if _, ok := token.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("%w: %v", ErrUnexpectedSigningMethod, token.Header["alg"])
		}
		return t.publicKey, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return token.Claims, nil
}
