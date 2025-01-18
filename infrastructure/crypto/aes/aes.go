package aes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"github.com/khanzadimahdi/testproject/domain/password"
)

type aesGCM struct {
	key []byte
}

var _ password.EncryptDecrypter = NewAESGCM(nil)

func NewAESGCM(key []byte) *aesGCM {
	return &aesGCM{key: key}
}

func (s *aesGCM) Encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, data, nil), nil
}

func (s *aesGCM) Decrypt(cipherData []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(cipherData) < nonceSize {
		return nil, errors.New("cipher data too short")
	}

	nonce, ciphertext := cipherData[:nonceSize], cipherData[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
