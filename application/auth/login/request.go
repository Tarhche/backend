package login

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	Identity string `json:"identity,omitempty"`
	Password string `json:"password,omitempty"`

	// Provider names somebody else's login page -- google, github, linkedin --
	// and Code is what that page handed the browser on its way back. A request
	// that carries them is answered by asking the provider who it was, and
	// nothing else about the request is read.
	Provider string `json:"provider,omitempty"`
	Code     string `json:"code,omitempty"`
}

var _ domain.Validatable = &Request{}

// SignsInWithProvider reports which of the two ways of proving who you are this
// request is using.
func (r *Request) SignsInWithProvider() bool {
	return len(r.Provider) > 0
}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if r.SignsInWithProvider() {
		if len(r.Code) == 0 {
			validationErrors["code"] = "required_field"
		}

		return validationErrors
	}

	if len(r.Identity) == 0 {
		validationErrors["identity"] = "required_field"
	}

	if len(r.Password) == 0 {
		validationErrors["password"] = "required_field"
	}

	return validationErrors
}
