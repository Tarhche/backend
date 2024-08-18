package comments

import (
	"time"
)

type CommentBson struct {
	UUID       string    `bson:"_id,omitempty"`
	Body       string    `bson:"body"`
	AuthorUUID string    `bson:"author_uuid"`
	ParentUUID string    `bson:"parent_uuid"`
	ObjectUUID string    `bson:"object_uuid,omitempty"`
	ObjectType string    `bson:"object_type,omitempty"`
	ApprovedAt time.Time `bson:"approved_at"`
	CreatedAt  time.Time `bson:"created_at,omitempty"`
	UpdatedAt  time.Time `bson:"updated_at,omitempty"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
