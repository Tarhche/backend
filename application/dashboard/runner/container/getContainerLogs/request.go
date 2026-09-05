package getContainerLogs

import (
	"time"
)

type Request struct {
	UUID string `json:"-"`

	After time.Time `json:"after"`
	Limit uint      `json:"limit"`
}
