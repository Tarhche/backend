package createrole

type validationErrors map[string]string

type Request struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	UserUUIDs   []string `json:"user_uuids"`
}

func (r *Request) Validate() (bool, validationErrors) {
	return true, nil
}
