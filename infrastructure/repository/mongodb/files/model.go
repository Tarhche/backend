package files

import (
	"time"
)

type FileBson struct {
	UUID       string    `bson:"_id,omitempty"`
	Name       string    `bson:"name"`
	StoredName string    `bson:"stored_name"`
	Size       int64     `bson:"size"`
	OwnerUUID  string    `bson:"owner_uuid"`
	MimeType   string    `bson:"mimetype"`
	CreatedAt  time.Time `bson:"created_at,omitempty"`
}
