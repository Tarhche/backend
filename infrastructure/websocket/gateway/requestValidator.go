package gateway

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
	requestField   = "request"
	requestIDField = "request_id"
	streamIDField  = "stream_id"
	subjectField   = "subject"
)

// the translation keys a rule can reject a field with.
const (
	requiredFieldMessage        = "required_field"
	invalidValueMessage         = "invalid_value"
	requestAlreadyExistsMessage = "request_already_exists"
	tooManyRequestsMessage      = "too_many_requests"
	streamNotOpenMessage        = "stream_is_not_open"
)

// rule reports the field it rejects and the translation key of the reason, or
// an empty field when the request passes. An error means the rule could not
// reach a verdict at all.
type rule func(request *domain.Request) (field string, message string, err error)

// requestValidator applies its rules in order; the first rule to reject a field
// owns that field's message. Rules run from missing-field checks to more
// specific ones, so a later rule never overwrites a more basic complaint.
//
// A request naming a stream is held to a different set: it asks nothing new, so
// it needs no id of its own and does not count against what the client has in
// flight — but the stream it names has to be one this connection actually has
// open.
type requestValidator struct {
	translator translator.Translator
	rules      []rule
	streamRule []rule
}

func newRequestValidator(
	registry RequestRegistry,
	subjects *subjects,
	translator translator.Translator,
	maxInFlight int,
) *requestValidator {
	return &requestValidator{
		translator: translator,
		rules: []rule{
			requestIDIsPresent,
			requestIDIsWellFormed,
			requestIDIsUnused(registry),
			subjectIsPresent,
			subjectIsConsumed(subjects),
			inFlightRequestsAreUnderLimit(registry, maxInFlight),
		},
		streamRule: []rule{
			streamIDIsWellFormed,
			streamIsOpen(registry),
			subjectIsConsumed(subjects),
		},
	}
}

func (v *requestValidator) validate(request *domain.Request) (domain.ValidationErrors, error) {
	rules := v.rules
	if len(request.StreamID) > 0 {
		rules = v.streamRule
	}

	validationErrors := make(domain.ValidationErrors)

	for _, rule := range rules {
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
func requestIDIsUnused(registry RequestRegistry) rule {
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

// streamIDIsWellFormed holds a stream id to the same shape as a request id,
// because that is what it is: the id of the request that opened the stream.
func streamIDIsWellFormed(request *domain.Request) (string, string, error) {
	if !IDRegex.MatchString(request.StreamID) {
		return streamIDField, invalidValueMessage, nil
	}

	return "", "", nil
}

// streamIsOpen rejects a stream this connection never opened, or one that has
// already ended. A client can only ever talk to its own streams.
func streamIsOpen(registry RequestRegistry) rule {
	return func(request *domain.Request) (string, string, error) {
		if !IDRegex.MatchString(request.StreamID) {
			return "", "", nil
		}

		serverSideID, err := registry.GetServerSideID(request.StreamID)
		if errors.Is(err, domain.ErrNotExists) {
			return streamIDField, streamNotOpenMessage, nil
		} else if err != nil {
			return "", "", err
		}

		if len(serverSideID) == 0 {
			return streamIDField, streamNotOpenMessage, nil
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

// subjectIsConsumed rejects a subject nothing listens on. A stream request may
// carry no subject at all, which asks for the stream to end rather than to
// reach a handler.
func subjectIsConsumed(subjects *subjects) rule {
	return func(request *domain.Request) (string, string, error) {
		if len(request.Subject) > 0 && !subjects.has(request.Subject) {
			return subjectField, invalidValueMessage, nil
		}

		return "", "", nil
	}
}

// inFlightRequestsAreUnderLimit caps what one connection may have waiting at
// once. A registry entry only goes away when its reply arrives or is given up
// on, so without a ceiling a client that asks and never gets answered grows its
// registry for as long as it stays connected.
func inFlightRequestsAreUnderLimit(registry RequestRegistry, limit int) rule {
	return func(*domain.Request) (string, string, error) {
		if registry.Len() >= limit {
			return requestField, tooManyRequestsMessage, nil
		}

		return "", "", nil
	}
}
