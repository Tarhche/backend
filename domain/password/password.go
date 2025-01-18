package password

type Hash struct {
	Value []byte
	Salt  []byte
}

type Hasher interface {
	Hash(value, salt []byte) []byte
	Equal(value, hash, salt []byte) bool
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
