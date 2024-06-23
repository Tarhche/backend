package register

import "regexp"

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
)

type validationErrors map[string]string

type Request struct {
	Identity string `json:"identity"`
}

func (r *Request) Validate() (bool, validationErrors) {
	if !emailRegex.MatchString(r.Identity) {
		return false, validationErrors{
			"identity": "identity is not a valid email address",
		}
	}

	return true, nil
}
