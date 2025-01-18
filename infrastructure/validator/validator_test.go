package validator

import (
	"reflect"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type user struct {
	email    string
	password string
}

var _ domain.Validatable = &user{}

func (u *user) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(u.email) == 0 {
		validationErrors["email"] = "email_is_mandatory"
	}

	if len(u.password) == 0 {
		validationErrors["password"] = "password_is_mandatory"
	}

	if len(validationErrors) > 0 {
		return validationErrors
	}

	return nil
}

var optionsType = reflect.TypeOf([]func(*translatorContract.Params){}).Name()

func TestValidator(t *testing.T) {
	t.Parallel()

	t.Run("validating of a validatable passes", func(t *testing.T) {
		t.Parallel()

		var (
			translatorMock translator.TranslatorMock
		)

		translatorMock.On(
			"Translate",
			"email_is_mandatory",
			mock.AnythingOfType(optionsType),
		).Once().Return("email is mandatory")

		translatorMock.On(
			"Translate",
			"password_is_mandatory",
			mock.AnythingOfType(optionsType),
		).Once().Return("password is mandatory")
		defer translatorMock.AssertExpectations(t)

		validator := New(&translatorMock)

		u1 := user{
			email:    "johnDoe@test.com",
			password: "test",
		}
		assert.Nil(t, validator.Validate(&u1))

		u2 := user{}
		validationErrors := domain.ValidationErrors{
			"email":    "email is mandatory",
			"password": "password is mandatory",
		}
		assert.Equal(t, validationErrors, validator.Validate(&u2))
	})

	t.Run("validating of a collection/list of validatables", func(t *testing.T) {
		t.Parallel()

		t.Run("slice", func(t *testing.T) {
			var (
				translatorMock translator.TranslatorMock
			)

			translatorMock.On(
				"Translate",
				"password_is_mandatory",
				mock.AnythingOfType(optionsType),
			).Once().Return("password is mandatory")
			defer translatorMock.AssertExpectations(t)

			validator := New(&translatorMock)

			u1 := []*user{
				{
					email:    "johnDoe@test.com",
					password: "test",
				},
			}
			assert.Nil(t, validator.Validate(&u1))

			u2 := []*user{
				{
					email: "johnDoe@test.com",
				},
			}
			validationErrors := domain.ValidationErrors{
				"password": "password is mandatory",
			}
			assert.Equal(t, validationErrors, validator.Validate(&u2))
		})

		t.Run("map", func(t *testing.T) {
			var (
				translatorMock translator.TranslatorMock
			)

			translatorMock.On(
				"Translate",
				"password_is_mandatory",
				mock.AnythingOfType(optionsType),
			).Once().Return("password is mandatory")
			defer translatorMock.AssertExpectations(t)

			validator := New(&translatorMock)

			u1 := map[string]*user{
				"friend": {
					email:    "johnDoe@test.com",
					password: "test",
				},
			}
			assert.Nil(t, validator.Validate(&u1))

			u2 := map[string]*user{
				"colleague": {
					email: "johnDoe@test.com",
				},
			}
			validationErrors := domain.ValidationErrors{
				"password": "password is mandatory",
			}
			assert.Equal(t, validationErrors, validator.Validate(&u2))
		})
	})
}
