package runCode

import (
	"fmt"
	"log"
	"slices"

	"github.com/khanzadimahdi/testproject/domain"
)

const (
	codeRunnerImageUrl = "ghcr.io/tarhche/code-runner"
)

type Request struct {
	Code   string `json:"code"`
	Runner string `json:"runner"`
}

var supportedCodeRunners = []string{
	// Go
	"go-1.24",
	"go-1.23",

	// NodeJS
	"nodejs-23.11",
	"nodejs-22.14",
	"nodejs-20.19",

	// PHP
	"php-8.4",
	"php-8.3",

	// nats
	"nats-2.10.0",
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Code) == 0 {
		validationErrors["code"] = "required_field"
	}

	if len(r.Runner) == 0 {
		validationErrors["runner"] = "required_field"
	}

	if !slices.Contains(supportedCodeRunners, r.Runner) {
		validationErrors["runner"] = "invalid_value"
	}

	log.Printf("validation errors: %+v", validationErrors)

	return validationErrors
}

func (r *Request) Image() string {
	return fmt.Sprintf("%s:%s-latest", codeRunnerImageUrl, r.Runner)
}
