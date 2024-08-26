package getUserBookmarks

type validationErrors map[string]string

type Request struct {
	OwnerUUID string `json:"-"`
	Page      uint
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.OwnerUUID) == 0 {
		errors["owner_uuid"] = "owner uuid is required"
	}

	return len(errors) == 0, errors
}
