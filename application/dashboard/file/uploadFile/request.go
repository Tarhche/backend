package createfile

import "io"

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
