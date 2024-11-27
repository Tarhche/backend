package validator

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/mock"
)

type MockValidator struct {
	mock.Mock
}

var _ domain.Validator = &validator{}

// Validate validates the given value and returns the validation error, if any.
func (m MockValidator) Validate(value any) domain.ValidationErrors {
	args := m.Mock.Called(value)

	validationErrors := args.Get(0)
	if validationErrors == nil {
		return nil
	}

	return validationErrors.(domain.ValidationErrors)
}
