package impersonateuser

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	// UserUUID is who the caller asks to be seen as.
	UserUUID string `json:"uuid"`

	// ImpersonatorUUID is who is asking. It is not asked for: it is who the
	// request's own token is for, filled in by the handler.
	ImpersonatorUUID string `json:"-"`

	// CallerIsAlreadyImpersonating says the caller is themselves being seen as
	// somebody else. It is filled in by the handler; standing in one person's
	// shoes is not permission to step into a third's.
	CallerIsAlreadyImpersonating bool `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UserUUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	return validationErrors
}
