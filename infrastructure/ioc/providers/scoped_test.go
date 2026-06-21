package providers

import (
	"context"
	"testing"

	"github.com/danceable/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/localize"
	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
)

// validatable is a test type whose validation always fails with a known key,
// used to assert that the scoped validator localizes messages.
type validatable struct{}

var _ domain.Validatable = validatable{}

func (validatable) Validate() domain.ValidationErrors {
	return domain.ValidationErrors{"name": "required_field"}
}

func TestScopedProviders_resolveLanguageAwareServices(t *testing.T) {
	manager := provider.Default
	manager.Register(NewScopedTranslationProvider())
	manager.Register(NewScopedValidationProvider())

	testCases := map[string]struct {
		languageCode      string
		wantTranslation   string
		wantValidationMsg string
	}{
		"english": {
			languageCode:      "en",
			wantTranslation:   "this field is required",
			wantValidationMsg: "this field is required",
		},
		"persian": {
			languageCode:      "fa",
			wantTranslation:   "این فیلد اجباری است",
			wantValidationMsg: "این فیلد اجباری است",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			scope, err := manager.Scope(context.Background(), provider.WithValue(localize.LanguageCode, testCase.languageCode))
			require.NoError(t, err)
			defer scope.Terminate(context.Background())

			var translator translatorContract.Translator
			require.NoError(t, scope.Container().Resolve(&translator))
			assert.Equal(t, testCase.wantTranslation, translator.Translate("required_field"))

			var validator domain.Validator
			require.NoError(t, scope.Container().Resolve(&validator))
			assert.Equal(t, testCase.wantValidationMsg, validator.Validate(validatable{})["name"])
		})
	}
}
