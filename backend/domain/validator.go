package domain

type ValidationErrors map[string]string

type Validator interface {
	Validate(value any) ValidationErrors
}

// Validatable is the interface indicating the type implementing it supports data validation.
type Validatable interface {
	Validate() ValidationErrors
}
