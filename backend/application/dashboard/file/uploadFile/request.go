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
	return true, nil
}
