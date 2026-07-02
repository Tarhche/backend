package jwt

import (
	"context"
	"crypto"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var (
	ErrInvalidAlgorithm        = errors.New("invalid algorithm")
	ErrInvalidToken            = errors.New("invalid token")
	ErrInvalidClaims           = errors.New("invalid claims")
	ErrUnexpectedSigningMethod = errors.New("unexpected signing method")
)

type JWT struct {
	privateKey crypto.PrivateKey
	publicKey  crypto.PublicKey
	tracer     oteltrace.Tracer
}

func NewJWT(privateKey crypto.PrivateKey, publicKey crypto.PublicKey) *JWT {
	return &JWT{
		privateKey: privateKey,
		publicKey:  publicKey,
		tracer:     otel.Tracer("jwt"),
	}
}

func (t *JWT) Generate(ctx context.Context, claims jwt.Claims) (string, error) {
	_, span := t.tracer.Start(ctx, "jwt.generate")
	defer span.End()

	token := jwt.NewWithClaims(jwt.SigningMethodES512, claims)

	tokenString, err := token.SignedString(t.privateKey)

	return tokenString, trace.RecordError(span, err)
}

func (t *JWT) Verify(ctx context.Context, tokenString string) (jwt.Claims, error) {
	_, span := t.tracer.Start(ctx, "jwt.verify")
	defer span.End()

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		// Check if the signing method is the expected ECDSA P-521 with SHA-512
		if _, ok := token.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("%w: %v", ErrUnexpectedSigningMethod, token.Header["alg"])
		}
		return t.publicKey, nil
	})

	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	if !token.Valid {
		return nil, trace.RecordError(span, ErrInvalidToken)
	}

	return token.Claims, nil
}
