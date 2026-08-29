package websocket

import (
	"errors"
	"regexp"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

// IDRegex is the shape a client-side request id must have.
var IDRegex = regexp.MustCompile("^[a-zA-Z0-9-]+$")

// the fields a rule can reject, as the client sees them.
const (
	requestIDField = "request_id"
	subjectField   = "subject"
)

// the translation keys a rule can reject a field with.
const (
	requiredFieldMessage        = "required_field"
	invalidValueMessage         = "invalid_value"
	requestAlreadyExistsMessage = "request_already_exists"
)

// rule reports the field it rejects and the translation key of the reason, or
// an empty field when the request passes. An error means the rule could not
// reach a verdict at all.
type rule func(request *domain.Request) (field string, message string, err error)

// requestValidator applies its rules in order; the first rule to reject a field
// owns that field's message. Rules run from missing-field checks to more
// specific ones, so a later rule never overwrites a more basic complaint.
type requestValidator struct {
	translator translator.Translator
	rules      []rule
}

func newRequestValidator(registry domain.RequestRegistry, subjects *subjects, translator translator.Translator) *requestValidator {
	return &requestValidator{
		translator: translator,
		rules: []rule{
			requestIDIsPresent,
			requestIDIsWellFormed,
			requestIDIsUnused(registry),
			subjectIsPresent,
			subjectIsConsumed(subjects),
		},
	}
}

func (v *requestValidator) validate(request *domain.Request) (domain.ValidationErrors, error) {
	validationErrors := make(domain.ValidationErrors)

	for _, rule := range v.rules {
		field, message, err := rule(request)
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
		return requestIDField, requiredFieldMessage, nil
	}

	return "", "", nil
}

// requestIDIsWellFormed keeps ids to the characters the message subjects carry.
func requestIDIsWellFormed(request *domain.Request) (string, string, error) {
	if len(request.ID) > 0 && !IDRegex.MatchString(request.ID) {
		return requestIDField, invalidValueMessage, nil
	}

	return "", "", nil
}

// requestIDIsUnused rejects an id that is already waiting for a reply.
func requestIDIsUnused(registry domain.RequestRegistry) rule {
	return func(request *domain.Request) (string, string, error) {
		if len(request.ID) == 0 {
			return "", "", nil
		}

		serverSideID, err := registry.GetServerSideID(request.ID)
		if err != nil && !errors.Is(err, domain.ErrNotExists) {
			return "", "", err
		}

		if len(serverSideID) > 0 {
			return requestIDField, requestAlreadyExistsMessage, nil
		}

		return "", "", nil
	}
}

func subjectIsPresent(request *domain.Request) (string, string, error) {
	if len(request.Subject) == 0 {
		return subjectField, requiredFieldMessage, nil
	}

	return "", "", nil
}

// subjectIsConsumed rejects a subject nothing listens on.
func subjectIsConsumed(subjects *subjects) rule {
	return func(request *domain.Request) (string, string, error) {
		if len(request.Subject) > 0 && !subjects.has(request.Subject) {
			return subjectField, invalidValueMessage, nil
		}

		return "", "", nil
	}
}
