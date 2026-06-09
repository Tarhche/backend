package resolver

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/language"
)

// Resolver resolves a language code to a language.
//
// DefaultCode returns the site's default language code (from config). Callers
// that accept an empty code are responsible for substituting it with
// DefaultCode before calling Resolve, so the substitution stays explicit.
//
// Read (GET) requests use Resolve, which fetches a language without verifying
// it against the existing ones. Write (POST/PATCH/PUT) requests use Verify to
// make sure a language actually exists before persisting it.
type Resolver interface {
	DefaultCode() (string, error)
	Resolve(requestedCode string) (language.Language, error)
	Verify(requestedCode string) bool
}

type LanguageResolver struct {
	languageRepository language.Repository
	configRepository   config.Repository
}

var _ Resolver = &LanguageResolver{}

func New(languageRepository language.Repository, configRepository config.Repository) *LanguageResolver {
	return &LanguageResolver{
		languageRepository: languageRepository,
		configRepository:   configRepository,
	}
}

// DefaultCode returns the site's default language code loaded from config (DB),
func (r *LanguageResolver) DefaultCode() (string, error) {
	c, err := r.configRepository.GetLatestRevision()
	if err == nil && len(c.DefaultLanguageCode) > 0 {
		return c.DefaultLanguageCode, nil
	}

	// fallback to first language in the respository
	// incase config is not set or default code is empty
	languages, err := r.languageRepository.GetAll(0, 1)
	if err == nil && len(languages) > 0 {
		return languages[0].Code, nil
	}

	return "", domain.ErrNotExists
}

// Resolve fetches the language for the given code.
func (r *LanguageResolver) Resolve(requestedCode string) (language.Language, error) {
	return r.languageRepository.GetOne(requestedCode)
}

// Verify reports whether a requested language code is acceptable.
func (r *LanguageResolver) Verify(requestedCode string) bool {
	return r.languageRepository.Exists(requestedCode)
}
