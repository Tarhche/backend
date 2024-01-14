package ecdsa

import (
	"os"
	"testing"
)

func TestECDSA(t *testing.T) {
	// to generate testdata using bash:
	// - private key: openssl ecparam -name secp521r1 -genkey -noout -out key.pem
	// - public key: openssl ec -in key.pem -pubout -out key.pem.pub

	privKeyData, err := os.ReadFile("testdata/key.pem")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	pubKeyData, err := os.ReadFile("testdata/key.pem.pub")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	privKey, err := ParsePrivateKey(privKeyData)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	pubKey, err := ParsePublicKey(pubKeyData)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if privKey == nil {
		t.Error("empty private key")
	}

	if pubKey == nil {
		t.Error("empty public key")
	}

	if !privKey.PublicKey.Equal(pubKey) {
		t.Errorf("private and it's public key doesn't match")
	}
}
