package jwt

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type builder jwt.MapClaims

func NewClaimsBuilder() builder {
	return make(builder)
}

func (b builder) SetIssuer(value string) {
	b.Set("iss", value)
}

func (b builder) SetSubject(value string) {
	b.Set("sub", value)
}

func (b builder) SetAudience(value []string) {
	b.Set("aud", value)
}

func (b builder) SetExpirationTime(value time.Time) {
	b.Set("exp", value.Unix())
}

func (b builder) SetNotBefore(value time.Time) {
	b.Set("nbf", value.Unix())
}

func (b builder) SetIssuedAt(value time.Time) {
	b.Set("iat", value.Unix())
}

func (b builder) SetID(value string) {
	b.Set("jti", value)
}

func (c builder) Set(name string, value any) {
	c[name] = value
}

func (c builder) Build() jwt.MapClaims {
	return jwt.MapClaims(c)
}
