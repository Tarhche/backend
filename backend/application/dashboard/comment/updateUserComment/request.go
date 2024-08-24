package updateUserComment

type validationErrors map[string]string

type Request struct {
	UUID     string `json:"uuid"`
	Body     string `json:"body"`
	UserUUID string `json:"-"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.UUID) == 0 {
		errors["uuid"] = "uuid is required"
	}

	if len(r.Body) == 0 {
		errors["body"] = "body is required"
	}

	if len(r.UserUUID) == 0 {
		errors["user_uuid"] = "user's uuid is required"
	}

	return len(errors) == 0, errors
}
