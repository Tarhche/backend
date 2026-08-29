package protocol

import (
	"errors"
	"regexp"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

// idRegex is the shape a client-side request id must have.
var idRegex = regexp.MustCompile("^[a-zA-Z0-9-]+$")

// RequestLookup reports the server-side id a client-side id is registered
// under, which is how the validator tells a fresh request from one already
// waiting for its reply.
type RequestLookup interface {
	GetServerSideID(clientSideID string) (string, error)
}

// the fields a Rule can reject, as the client sees them.
const (
	RequestIDField = "request_id"
	SubjectField   = "subject"
)

// the translation keys a Rule can reject a field with.
const (
	RequiredFieldMessage        = "required_field"
	InvalidValueMessage         = "invalid_value"
	RequestAlreadyExistsMessage = "request_already_exists"
)

// Rule reports the field it rejects and the translation key of the reason, or
// an empty field when the request passes. An error means the Rule could not
// reach a verdict at all.
type Rule func(request *domain.Request) (field string, message string, err error)

// Validator applies its rules in order; the first Rule to reject a field
// owns that field's message. Rules run from missing-field checks to more
// specific ones, so a later Rule never overwrites a more basic complaint.
type Validator struct {
	translator translator.Translator
	rules      []Rule
}

func NewValidator(requests RequestLookup, subjects *Subjects, translator translator.Translator) *Validator {
	return &Validator{
		translator: translator,
		rules: []Rule{
			requestIDIsPresent,
			requestIDIsWellFormed,
			requestIDIsUnused(requests),
			subjectIsPresent,
			subjectIsConsumed(subjects),
		},
	}
}

func (v *Validator) Validate(request *domain.Request) (domain.ValidationErrors, error) {
	validationErrors := make(domain.ValidationErrors)

	for _, Rule := range v.rules {
		field, message, err := Rule(request)
		if err != nil {
			return nil, err
		}

		if len(field) == 0 {
			continue
		}

		if _, rejected := validationErrors[field]; rejected {
			continue
		}

		validationErrors[field] = v.translator.Translate(message)
	}

	return validationErrors, nil
}

func requestIDIsPresent(request *domain.Request) (string, string, error) {
	if len(request.ID) == 0 {
		return RequestIDField, RequiredFieldMessage, nil
	}

	return "", "", nil
}

// requestIDIsWellFormed keeps ids to the characters the message subjects carry.
func requestIDIsWellFormed(request *domain.Request) (string, string, error) {
	if len(request.ID) > 0 && !idRegex.MatchString(request.ID) {
		return RequestIDField, InvalidValueMessage, nil
	}

	return "", "", nil
}

// requestIDIsUnused rejects an id that is already waiting for a reply.
func requestIDIsUnused(requests RequestLookup) Rule {
	return func(request *domain.Request) (string, string, error) {
		if len(request.ID) == 0 {
			return "", "", nil
		}

		serverSideID, err := requests.GetServerSideID(request.ID)
		if err != nil && !errors.Is(err, domain.ErrNotExists) {
			return "", "", err
		}

		if len(serverSideID) > 0 {
			return RequestIDField, RequestAlreadyExistsMessage, nil
		}

		return "", "", nil
	}
}

func subjectIsPresent(request *domain.Request) (string, string, error) {
	if len(request.Subject) == 0 {
		return SubjectField, RequiredFieldMessage, nil
	}

	return "", "", nil
}

// subjectIsConsumed rejects a subject nothing listens on.
func subjectIsConsumed(subjects *Subjects) Rule {
	return func(request *domain.Request) (string, string, error) {
		if len(request.Subject) > 0 && !subjects.Has(request.Subject) {
			return SubjectField, InvalidValueMessage, nil
		}

		return "", "", nil
	}
}
