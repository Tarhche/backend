package createfile

import "io"

const MaxFileSize int64 = 100 << 20 // 100MB

type validationErrors map[string]string

type Request struct {
	Name       string
	OwnerUUID  string
	FileReader io.Reader
	Size       int64
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.Name) == 0 {
		errors["name"] = "name is required"
	}

	if len(r.OwnerUUID) == 0 {
		errors["owner_uuid"] = "owner uuid is required"
	}

	if r.Size == 0 {
		errors["size"] = "file's size should be greater than zero"
	}

	if r.Size > MaxFileSize {
		errors["size"] = "file's size exceeds the limit"
	}

	return len(errors) == 0, errors
}
