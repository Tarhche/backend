package validator

import (
	"reflect"

	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
)

type validator struct {
	translator translatorContract.Translator
}

var _ domain.Validator = &validator{}

func New(translator translatorContract.Translator) *validator {
	return &validator{
		translator: translator,
	}
}

// Validate validates the given value and returns the validation error, if any.
func (v validator) Validate(value any) domain.ValidationErrors {
	validationErrors := validate(value)

	for field := range validationErrors {
		validationErrors[field] = v.translator.Translate(
			validationErrors[field],
			translatorContract.WithAttribute("field", field),
		)
	}

	return validationErrors
}

func validate(value any, rules ...domain.Validator) domain.ValidationErrors {
	for _, rule := range rules {
		if validationErrors := rule.Validate(value); len(validationErrors) > 0 {
			return validationErrors
		}
	}

	rv := reflect.ValueOf(value)
	if (rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface) && rv.IsNil() {
		return nil
	}

	if v, ok := value.(domain.Validatable); ok {
		return v.Validate()
	}

	switch rv.Kind() {
	case reflect.Map:
		return validateMap(rv)
	case reflect.Slice, reflect.Array:
		return validateSlice(rv)
	case reflect.Ptr, reflect.Interface:
		return validate(rv.Elem().Interface())
	}

	return nil
}

// validateMap validates a map of validatable elements
func validateMap(rv reflect.Value) domain.ValidationErrors {
	var (
		validationMessage domain.ValidationErrors
	)

	for _, key := range rv.MapKeys() {
		if mv := rv.MapIndex(key).Interface(); mv != nil {
			if validationMessage = validate(mv); len(validationMessage) > 0 {
				break
			}
		}
	}

	return validationMessage
}

// validateSlice validates a slice/array of validatable elements
func validateSlice(rv reflect.Value) domain.ValidationErrors {
	var (
		validationMessage domain.ValidationErrors
	)

	l := rv.Len()
	for i := 0; i < l; i++ {
		if ev := rv.Index(i).Interface(); ev != nil {
			if validationMessage = validate(ev); len(validationMessage) > 0 {
				break
			}
		}
	}

	return validationMessage
}
