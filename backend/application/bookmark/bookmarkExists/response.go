package bookmarkExists

type Response struct {
	ValidationErrors validationErrors `json:"errors,omitempty"`

	Exist bool `json:"exist"`
}
