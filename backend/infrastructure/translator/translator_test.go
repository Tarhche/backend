package translator

import (
	"fmt"
	"testing"
	"time"

	contract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/stretchr/testify/assert"
)

func TestTranslator(t *testing.T) {
	t.Run("translate", func(t *testing.T) {
		defaultLocale := "de"
		translations := map[string]map[string]string{
			defaultLocale: {
				"germany": "deutschland",
				"now":     "jetzt",
			},
		}

		translator := New(translations, defaultLocale)

		assert.Equal(t, "deutschland", translator.Translate("germany"))
		assert.Equal(t, "jetzt", translator.Translate("now"))
	})

	t.Run("with locale", func(t *testing.T) {
		localeDE := "de"
		localeFA := "fa"

		translations := map[string]map[string]string{
			localeDE: {
				"germany": "deutschland",
				"now":     "jetzt",
			},
			localeFA: {
				"germany": "آلمان",
				"now":     "الان",
			},
		}

		translator := New(translations, localeDE)

		assert.Equal(t, "deutschland", translator.Translate("germany"))
		assert.Equal(t, "jetzt", translator.Translate("now"))

		assert.Equal(t, "آلمان", translator.Translate("germany", contract.WithLocale(localeFA)))
		assert.Equal(t, "الان", translator.Translate("now", contract.WithLocale(localeFA)))
	})

	t.Run("with attributes", func(t *testing.T) {
		localeDE := "de"
		localeFA := "fa"

		translations := map[string]map[string]string{
			localeFA: {
				"{field} is required": "{field} اجباری است",
				"time is {time}":      "زمان برابر با {time} است",
			},
		}

		translator := New(translations, localeDE)

		fieldName := "password"
		date, err := time.Parse(time.RFC3339, "2024-09-29T15:56:25Z")
		assert.NoError(t, err)

		assert.Equal(
			t,
			fmt.Sprintf("%s اجباری است", fieldName),
			translator.Translate(
				"{field} is required",
				contract.WithLocale(localeFA),
				contract.WithAttribute("field", fieldName),
			),
		)

		assert.Equal(
			t,
			fmt.Sprintf("زمان برابر با %s است", date.Format(time.RFC3339)),
			translator.Translate(
				"time is {time}",
				contract.WithLocale(localeFA),
				contract.WithAttribute("time", date.Format(time.RFC3339)),
			),
		)
	})

	t.Run("returns empty string when a translation not exists", func(t *testing.T) {
		localeDE := "de"
		localeFA := "fa"

		translations := map[string]map[string]string{
			localeFA: {
				"{field} is required": "{field} اجباری است",
				"time is {time}":      "زمان برابر با {time} است",
			},
		}

		translator := New(translations, localeFA)

		fieldName := "password"
		date, err := time.Parse(time.RFC3339, "2024-09-29T15:56:25Z")
		assert.NoError(t, err)

		assert.Equal(
			t,
			"",
			translator.Translate(
				"{field} is required",
				contract.WithLocale(localeDE),
				contract.WithAttribute("field", fieldName),
			),
		)

		assert.Equal(
			t,
			"",
			translator.Translate(
				"time is {time}",
				contract.WithLocale(localeDE),
				contract.WithAttribute("time", date.Format(time.RFC3339)),
			),
		)
	})
}
