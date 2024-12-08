package files

import (
	"time"
)

type FileBson struct {
	UUID      string    `bson:"_id,omitempty"`
	Name      string    `bson:"name"`
	Size      int64     `bson:"size"`
	OwnerUUID string    `bson:"owner_uuid"`
	MimeType  string    `bson:"mimetype"`
	CreatedAt time.Time `bson:"created_at,omitempty"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
