package ecdsa

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

var (
	ErrPrivateKeyParseFailure = errors.New("failed to parse PEM block containing the private key")
	ErrPublicKeyParseFailure  = errors.New("failed to parse PEM block containing the public key")
	ErrInvalidKey             = errors.New("invalid key")
)

// Generate ECDSA P-521 private key
func Generate() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
}

func ParsePrivateKey(key []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(key)
	if block == nil {
		return nil, ErrPrivateKeyParseFailure
	}

	ecdsaPrivateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return ecdsaPrivateKey, nil
}

func ParsePublicKey(key []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(key)
	if block == nil {
		return nil, ErrPublicKeyParseFailure
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	ecdsaPublicKey, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, ErrInvalidKey
	}

	return ecdsaPublicKey, nil
}
