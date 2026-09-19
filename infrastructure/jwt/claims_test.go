package jwt

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestBuilder(t *testing.T) {
	builder := NewClaimsBuilder()

	exp := time.Now().Add(1 * time.Hour)
	nbf := time.Now()
	iat := time.Now()

	expectedClaims := jwt.MapClaims{
		"iss":  "test-issuer",
		"sub":  "test-subject",
		"aud":  []string{"test-audience-1", "test-audience-2"},
		"exp":  exp.Unix(),
		"nbf":  nbf.Unix(),
		"iat":  iat.Unix(),
		"jti":  "test-id",
		"lang": "EN",
	}

	builder.SetIssuer(expectedClaims["iss"].(string))
	builder.SetSubject(expectedClaims["sub"].(string))
	builder.SetAudience(expectedClaims["aud"].([]string))
	builder.SetExpirationTime(exp)
	builder.SetNotBefore(nbf)
	builder.SetIssuedAt(iat)
	builder.SetID(expectedClaims["jti"].(string))
	builder.SetLanguage(expectedClaims["lang"].(string))

	claims := builder.Build()

	want, err := json.Marshal(expectedClaims)
	if err != nil {
		t.Fatalf("unexpected error: %v", want)
	}

	got, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", got)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("console error output mismatch (-want +got):\n%s", diff)
	}
}

func TestPermissions(t *testing.T) {
	t.Parallel()

	t.Run("what a token says its holder may do comes back", func(t *testing.T) {
		claims := NewClaimsBuilder()
		claims.SetPermissions([]string{"articles.index", "users.show"})

		assert.Equal(t, []string{"articles.index", "users.show"}, Permissions(claims.Build()))
	})

	t.Run("it survives the journey through json", func(t *testing.T) {
		claims := jwt.MapClaims{"permissions": []any{"articles.index", 42, "users.show"}}

		assert.Equal(t, []string{"articles.index", "users.show"}, Permissions(claims))
	})

	t.Run("a token that says nothing says nothing", func(t *testing.T) {
		assert.Nil(t, Permissions(NewClaimsBuilder().Build()))
		assert.Nil(t, Permissions(jwt.MapClaims{"permissions": "articles.index"}))
	})
}
