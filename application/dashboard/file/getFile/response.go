package getfile

import (
	"io"
	"time"
)

type Response struct {
	Name      string
	Size      int64
	OwnerUUID string
	MimeType  string
	CreatedAt time.Time

	Reader io.ReadSeekCloser
}
