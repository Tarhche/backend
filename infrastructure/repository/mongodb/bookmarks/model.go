package articles

import (
	"time"
)

type BookmarkBson struct {
	UUID       string    `bson:"_id, omitempty"`
	Title      string    `bson:"title,omitempty"`
	ObjectUUID string    `bson:"object_uuid,omitempty"`
	ObjectType string    `bson:"object_type,omitempty"`
	OwnerUUID  string    `bson:"owner_uuid,omitempty"`
	CreatedAt  time.Time `bson:"created_at,omitempty"`
}
