package comments

import (
	"time"
)

type CommentBson struct {
	UUID       string    `bson:"_id,omitempty"`
	Body       string    `bson:"body,omitempty"`
	AuthorUUID string    `bson:"author_uuid,omitempty"`
	ParentUUID string    `bson:"parent_uuid,omitempty"`
	ObjectUUID string    `bson:"object_uuid,omitempty"`
	ObjectType string    `bson:"object_type,omitempty"`
	ApprovedAt time.Time `bson:"approved_at,omitempty"`
	CreatedAt  time.Time `bson:"created_at,omitempty"`
	UpdatedAt  time.Time `bson:"updated_at,omitempty"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
