package createfile

import (
	"io"
	"path/filepath"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
)

const MaxFileSize int64 = 100 << 20 // 100MB

type Request struct {
	Name       string
	OwnerUUID  string
	FileReader io.Reader
	Size       int64
	MimeType   string
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

func (r *Request) StoredName() (string, error) {
	var filename string

	extension := filepath.Ext(r.Name)

	uuid, err := uuid.NewV7()
	if err != nil {
		return filename, err
	}
	filename = uuid.String() + extension

	return filename, nil
}
