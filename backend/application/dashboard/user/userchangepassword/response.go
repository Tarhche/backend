package userchangepassword

type ChangePasswordResponse struct {
	ValidationErrors validationErrors `json:"errors,omitempty"`
}
