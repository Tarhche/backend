package jwt

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ImpersonatorClaim carries who obtained a token to act as its subject. An
// ordinary token does not have it: it is what tells a shadow session apart
// from the session of the person it stands for.
const ImpersonatorClaim = "impersonator"

// PermissionsClaim carries what the subject of a token may do, as it was when
// the token was issued.
const PermissionsClaim = "permissions"

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

func (b builder) SetRoles(value []string) {
	b.Set("roles", value)
}

func (b builder) SetPermissions(value []string) {
	b.Set("permissions", value)
}

func (b builder) SetLanguage(code string) {
	b.Set("lang", code)
}

func (b builder) SetImpersonator(value string) {
	b.Set(ImpersonatorClaim, value)
}

func (c builder) Set(name string, value any) {
	c[name] = value
}

func (c builder) Build() jwt.MapClaims {
	return jwt.MapClaims(c)
}

// Impersonator reports who obtained a token to act as its subject, and is
// empty when nobody did, which is every ordinary session.
func Impersonator(claims jwt.Claims) string {
	mapClaims, ok := claims.(jwt.MapClaims)
	if !ok {
		return ""
	}

	impersonator, _ := mapClaims[ImpersonatorClaim].(string)

	return impersonator
}

// Permissions are what a token says its subject may do. They are what the
// roles held at the moment it was issued came to, so they are a token's own
// answer rather than the database's, and whoever has to refuse a request asks
// the database instead.
func Permissions(claims jwt.Claims) []string {
	mapClaims, ok := claims.(jwt.MapClaims)
	if !ok {
		return nil
	}

	switch permissions := mapClaims[PermissionsClaim].(type) {
	case []string:
		return permissions
	case []any:
		names := make([]string, 0, len(permissions))
		for _, permission := range permissions {
			if name, ok := permission.(string); ok {
				names = append(names, name)
			}
		}

		return names
	default:
		return nil
	}
}
