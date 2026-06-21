package localize

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
)

// LanguageCode is the name under which the resolved request language code is
// seeded into a request scope.
const LanguageCode = "language_code"

type languageKey struct{}

// FromContext returns the resolved request language stored in ctx, or an empty
// string when none was set.
func FromContext(ctx context.Context) string {
	code, _ := ctx.Value(languageKey{}).(string)

	return code
}

// ToContext returns a copy of ctx carrying the given resolved language code.
// Handlers read it back with FromContext.
func ToContext(ctx context.Context, code string) context.Context {
	return context.WithValue(ctx, languageKey{}, code)
}

// Localizer resolves the language that applies to a request.
type Localizer struct {
	resolver resolver.Resolver
}

func New(languageResolver resolver.Resolver) *Localizer {
	return &Localizer{
		resolver: languageResolver,
	}
}

// Resolve determines the request language:
//  1. requestedCode, when non-empty (an explicitly requested language code);
//  2. otherwise the authenticated user's language, if any;
//  3. otherwise the website's default language.
func (l *Localizer) Resolve(ctx context.Context, requestedCode string) string {
	if len(requestedCode) > 0 {
		return requestedCode
	}

	if u := auth.FromContext(ctx); u != nil && len(u.LanguageCode) > 0 {
		return u.LanguageCode
	}

	code, _ := l.resolver.DefaultCode()

	return code
}
