package argon2

import (
	"bytes"

	"github.com/khanzadimahdi/testproject/domain/password"
	"golang.org/x/crypto/argon2"
)

type argon2id struct {
	// time represents the number of
	// passed over the specified memory.
	time uint32
	// cpu memory to be used.
	memory uint32
	// threads for parallelism aspect
	// of the algorithm.
	threads uint8
	// keyLen of the generate hash key.
	keyLen uint32
}

var _ password.Hasher = NewArgon2id(1, 2, 3, 4)

// NewArgon2id constructor function for argon2id.
func NewArgon2id(time, memory uint32, threads uint8, keyLen uint32) *argon2id {
	return &argon2id{
		time:    time,
		memory:  memory,
		threads: threads,
		keyLen:  keyLen,
	}
}

// Hash using the value and provided salt.
func (a *argon2id) Hash(value, salt []byte) []byte {
	return argon2.IDKey(value, salt, a.time, a.memory, a.threads, a.keyLen)
}

// Equal reports whether a value and its hash match.
func (a *argon2id) Equal(value, hash, salt []byte) bool {
	return bytes.Equal(hash, a.Hash(value, salt))
}
