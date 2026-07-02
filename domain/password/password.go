package password

import "context"

type Hash struct {
	Value []byte
	Salt  []byte
}

type Hasher interface {
	Hash(ctx context.Context, value, salt []byte) []byte
	Equal(ctx context.Context, value, hash, salt []byte) bool
}

type Encrypter interface {
	Encrypt([]byte) ([]byte, error)
}

type Decrypter interface {
	Decrypt([]byte) ([]byte, error)
}

type EncryptDecrypter interface {
	Encrypter
	Decrypter
}
