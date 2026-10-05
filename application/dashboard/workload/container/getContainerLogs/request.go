package getContainerLogs

import (
	"time"
)

type Request struct {
	VMUUID string `json:"-"`

	// ID is the container's id or its name.
	ID string `json:"-"`

	// Since leaves out what was written before it; zero is from the start.
	Since time.Time `json:"since"`

	// Tail keeps only the last lines; zero is as many as one answer carries.
	Tail uint `json:"tail"`

	// OwnerUUID narrows what is read to one person's own, and is empty for
	// anybody's.
	OwnerUUID string `json:"-"`
}
