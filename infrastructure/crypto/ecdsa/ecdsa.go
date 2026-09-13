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

// EncodePrivateKey writes a private key in the PEM form ParsePrivateKey reads,
// which is the one `openssl ecparam -genkey` produces.
func EncodePrivateKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}

// EncodePublicKey writes a public key in the PEM form ParsePublicKey reads,
// which is the one `openssl ec -pubout` produces.
func EncodePublicKey(key *ecdsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
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
