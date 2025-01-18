package aes

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestAESGCM(t *testing.T) {
	data := make([]byte, 5*1024)
	rand.Read(data)

	key := make([]byte, 32)
	rand.Read(key)

	encryptDecrypter := NewAESGCM(key)

	t.Run("encryption and decryption", func(t *testing.T) {
		encryptedData, err := encryptDecrypter.Encrypt(data)
		if err != nil {
			t.Error("unexpected error", err)
		}

		decryptedData, err := encryptDecrypter.Decrypt(encryptedData)
		if err != nil {
			t.Error("unexpected error", err)
		}

		if !bytes.Equal(data, decryptedData) {
			t.Error("data and it's decrypted value are not equal")
		}
	})

	t.Run("decryption with a non-identical key should fail", func(t *testing.T) {
		wrongKey := make([]byte, 32)
		rand.Read(wrongKey)

		encryptedData, err := encryptDecrypter.Encrypt(data)
		if err != nil {
			t.Error("unexpected error", err)
		}

		if _, err := NewAESGCM(wrongKey).Decrypt(encryptedData); err == nil {
			t.Error("decryption using a wrong key should not be possible")
		}
	})

	t.Run("decryption of an interupted cipherdata should fail", func(t *testing.T) {
		encryptedData, err := encryptDecrypter.Encrypt(data)
		if err != nil {
			t.Error("unexpected error", err)
		}

		oneCharLost := encryptedData[:len(encryptedData)-1]
		if _, err := encryptDecrypter.Decrypt(oneCharLost); err == nil {
			t.Error("decryption of an interupted cipherdata should fail")
		}

	})
}
