package createfile

import (
	"io"

	"github.com/khanzadimahdi/testproject/domain"
)

const MaxFileSize int64 = 100 << 20 // 100MB

type Request struct {
	Name       string
	OwnerUUID  string
	FileReader io.Reader
	Size       int64
}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	if len(r.OwnerUUID) == 0 {
		validationErrors["owner_uuid"] = "required_field"
	}

	if r.Size == 0 {
		validationErrors["size"] = "greater_than_zero"
	}

	if r.Size > MaxFileSize {
		validationErrors["size"] = "exceeds_limit"
	}

	return validationErrors
}
