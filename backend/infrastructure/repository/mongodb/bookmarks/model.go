package articles

import (
	"time"
)

type BookmarkBson struct {
	UUID       string    `bson:"_id, omitempty"`
	ObjectUUID string    `bson:"object_uuid,omitempty"`
	ObjectType string    `bson:"object_type,omitempty"`
	OwnerUUID  string    `bson:"owner_uuid,omitempty"`
	CreatedAt  time.Time `bson:"created_at,omitempty"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
