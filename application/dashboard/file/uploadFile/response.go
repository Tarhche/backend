package createfile

type UploadFileResponse struct {
	ValidationErrors validationErrors `json:"errors,omitempty"`
	UUID             string           `json:"uuid,omitempty"`
}
