package createStack

import (
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/dockerVM"
	"github.com/khanzadimahdi/testproject/domain"
)

const (
	// MaxCompose is the most YAML a stack may be given, in bytes.
	MaxCompose = 256 << 10

	// maxNameLength keeps a name to something a listing can show.
	maxNameLength = 100
)

// Request is a compose project to deploy for OwnerUUID, into the Docker VM VM
// chooses.
type Request struct {
	OwnerUUID string `json:"-"`

	Name string `json:"name"`

	// Compose is the YAML as it was written. What it calls its project is
	// ignored: the stack's slug is the project.
	Compose string `json:"compose"`

	VM dockerVM.Choice `json:"vm"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.OwnerUUID) == 0 {
		validationErrors["owner_uuid"] = "required_field"
	}

	switch name := strings.TrimSpace(r.Name); {
	case len(name) == 0:
		validationErrors["name"] = "required_field"
	case len(name) > maxNameLength:
		validationErrors["name"] = "invalid_name"
	}

	if code, ok := ValidateCompose(r.Compose); !ok {
		validationErrors["compose"] = code
	}

	if len(r.VM.UUID) > 0 && r.VM.New != nil {
		validationErrors["vm"] = "vm_or_new_vm"
	}

	return validationErrors
}

// ValidateCompose checks that compose is YAML a compose project can be made
// of: no larger than a stack may be given, and with a service at least.
// Anything finer is compose's own to refuse, which it does when the stack is
// deployed, in words that end up in the stack's output.
func ValidateCompose(compose string) (string, bool) {
	if len(strings.TrimSpace(compose)) == 0 {
		return "required_field", false
	}

	if len(compose) > MaxCompose {
		return "too_large", false
	}

	var project struct {
		Services map[string]any `yaml:"services"`
	}

	if err := yaml.Unmarshal([]byte(compose), &project); err != nil {
		return "invalid_value", false
	}

	if len(project.Services) == 0 {
		return "invalid_value", false
	}

	return "", true
}
