package argon2

import (
	"crypto/rand"
	"testing"
)

func TestArgon2(t *testing.T) {
	var keyLen uint32 = 256

	argon2id := NewArgon2id(1, 64*1024, 32, keyLen)

	value := make([]byte, 100)
	rand.Read(value)

	t.Run("given value and its hash should match", func(t *testing.T) {
		salt := []byte{2, 4, 6, 8, 10}
		hash := argon2id.Hash(value, salt)

		if int(keyLen) != len(hash) {
			t.Errorf("expected hash length %d, but got %d", keyLen, len(hash))
		}

		if equal := argon2id.Equal(value, hash, salt); !equal {
			t.Error("value and it's hash doesn't match")
		}
	})

	t.Run("not identical value should not match", func(t *testing.T) {
		salt := []byte{2, 4, 6, 8, 10}
		hash := argon2id.Hash(value, salt)

		if int(keyLen) != len(hash) {
			t.Errorf("expected hash length %d, but got %d", keyLen, len(hash))
		}

		nonIdenticalValue := append(value, 0)

		if equal := argon2id.Equal(nonIdenticalValue, hash, salt); equal {
			t.Error("hash should not match a non-identical value")
		}
	})
}

func BenchmarkArgon2idHash(b *testing.B) {
	hasher := NewArgon2id(3, 64*1024, 2, 64)

	salt := make([]byte, 16)
	rand.Read(salt)

	value := make([]byte, 64)
	rand.Read(value)

	b.ResetTimer()
	for range b.N {
		hasher.Hash(value, salt)
	}
}
