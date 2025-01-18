package translator

import (
	"strings"

	contract "github.com/khanzadimahdi/testproject/domain/translator"
)

type (
	locale       = string
	keyValues    = map[string]string
	translations = map[locale]keyValues
)

type Translator struct {
	translations  translations
	defaultLocale string
}

var _ contract.Translator = &Translator{}

func New(t translations, defaultLocale string) *Translator {
	return &Translator{
		translations:  t,
		defaultLocale: defaultLocale,
	}
}

func (t *Translator) Translate(key string, options ...func(*contract.Params)) string {
	params := contract.Params{
		Attributes: make(map[string]string),
		Locale:     t.defaultLocale,
	}

	for i := range options {
		if options[i] == nil {
			continue
		}

		options[i](&params)
	}

	translation, ok := t.translations[params.Locale][key]
	if !ok {
		return ""
	}

	if len(params.Attributes) == 0 {
		return translation
	}

	mappedAttributes := make([]string, len(params.Attributes)*2)

	var i int
	for k := range params.Attributes {
		mappedAttributes[i] = "{" + k + "}"
		mappedAttributes[i+1] = params.Attributes[k]
		i += 2
	}

	return strings.NewReplacer(mappedAttributes...).Replace(translation)
}
