package changepassword

type Response struct {
	ValidationErrors validationErrors `json:"errors,omitempty"`
}
