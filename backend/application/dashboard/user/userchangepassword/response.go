package userchangepassword

type Response struct {
	ValidationErrors validationErrors `json:"errors,omitempty"`
}
