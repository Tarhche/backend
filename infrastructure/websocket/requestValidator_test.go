package websocket

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// echoTranslator returns every message key unchanged.
func echoTranslator() *translator.TranslatorMock {
	translatorMock := &translator.TranslatorMock{}

	for _, key := range []string{requiredFieldMessage, invalidValueMessage, requestAlreadyExistsMessage} {
		translatorMock.On("Translate", key, mock.AnythingOfType("[]func(*translator.Params)")).Return(key).Maybe()
	}

	return translatorMock
}

func TestRequestValidator(t *testing.T) {
	t.Parallel()

	const consumedSubject = "runCode"

	testcases := []struct {
		name             string
		request          domain.Request
		registry         func(*MockRequestRegistry)
		validationErrors domain.ValidationErrors
		expectsError     bool
	}{
		{
			name:    "a well formed request on a consumed subject passes",
			request: domain.Request{ID: "req-1", Subject: consumedSubject},
			registry: func(r *MockRequestRegistry) {
				r.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
			},
			validationErrors: domain.ValidationErrors{},
		},
		{
			name:    "a missing id is reported as missing, not as malformed",
			request: domain.Request{Subject: consumedSubject},
			validationErrors: domain.ValidationErrors{
				requestIDField: requiredFieldMessage,
			},
		},
		{
			name:    "an id outside the allowed characters is rejected",
			request: domain.Request{ID: "req 1!", Subject: consumedSubject},
			registry: func(r *MockRequestRegistry) {
				r.On("GetServerSideID", "req 1!").Return("", domain.ErrNotExists)
			},
			validationErrors: domain.ValidationErrors{
				requestIDField: invalidValueMessage,
			},
		},
		{
			name:    "an id that is already waiting for a reply is rejected",
			request: domain.Request{ID: "req-1", Subject: consumedSubject},
			registry: func(r *MockRequestRegistry) {
				r.On("GetServerSideID", "req-1").Return("server-1", nil)
			},
			validationErrors: domain.ValidationErrors{
				requestIDField: requestAlreadyExistsMessage,
			},
		},
		{
			name:    "a missing subject is rejected",
			request: domain.Request{ID: "req-1"},
			registry: func(r *MockRequestRegistry) {
				r.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
			},
			validationErrors: domain.ValidationErrors{
				subjectField: requiredFieldMessage,
			},
		},
		{
			name:    "a subject nothing consumes is rejected",
			request: domain.Request{ID: "req-1", Subject: "nobody-listens-here"},
			registry: func(r *MockRequestRegistry) {
				r.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
			},
			validationErrors: domain.ValidationErrors{
				subjectField: invalidValueMessage,
			},
		},
		{
			name:    "every rejected field is reported, not just the first",
			request: domain.Request{},
			validationErrors: domain.ValidationErrors{
				requestIDField: requiredFieldMessage,
				subjectField:   requiredFieldMessage,
			},
		},
		{
			name:    "a registry that cannot answer fails the validation instead of rejecting the request",
			request: domain.Request{ID: "req-1", Subject: consumedSubject},
			registry: func(r *MockRequestRegistry) {
				r.On("GetServerSideID", "req-1").Return("", errors.New("registry is unreachable"))
			},
			expectsError: true,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()

			var registryMock MockRequestRegistry
			if testcase.registry != nil {
				testcase.registry(&registryMock)
			}
			defer registryMock.AssertExpectations(t)

			translatorMock := echoTranslator()
			defer translatorMock.AssertExpectations(t)

			subjects := newSubjects()
			subjects.add(consumedSubject)

			validationErrors, err := newRequestValidator(&registryMock, subjects, translatorMock).validate(&testcase.request)

			if testcase.expectsError {
				assert.Error(t, err)
				assert.Nil(t, validationErrors)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, testcase.validationErrors, validationErrors)
		})
	}
}
