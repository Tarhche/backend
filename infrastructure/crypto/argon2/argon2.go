package argon2

import (
	"bytes"
	"context"

	"github.com/khanzadimahdi/testproject/domain/password"
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
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

	tracer oteltrace.Tracer
}

var _ password.Hasher = NewArgon2id(1, 2, 3, 4)

// NewArgon2id constructor function for argon2id.
func NewArgon2id(time, memory uint32, threads uint8, keyLen uint32) *argon2id {
	return &argon2id{
		time:    time,
		memory:  memory,
		threads: threads,
		keyLen:  keyLen,
		tracer:  otel.Tracer("argon2"),
	}
}

// Hash using the value and provided salt. Argon2 is deliberately expensive
// CPU-wise, so this is traced to keep it visible as a latency contributor on
// auth flows rather than an invisible chunk of "handler" time.
func (a *argon2id) Hash(ctx context.Context, value, salt []byte) []byte {
	_, span := a.tracer.Start(ctx, "argon2.hash")
	defer span.End()

	return argon2.IDKey(value, salt, a.time, a.memory, a.threads, a.keyLen)
}

// Equal reports whether a value and its hash match.
func (a *argon2id) Equal(ctx context.Context, value, hash, salt []byte) bool {
	return bytes.Equal(hash, a.Hash(ctx, value, salt))
}
