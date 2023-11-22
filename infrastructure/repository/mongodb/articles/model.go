package articles

import (
	"time"
)

type ArticleBson struct {
	UUID        string    `bson:"_id,omitempty"`
	Cover       string    `bson:"cover,omitempty"`
	Title       string    `bson:"title,omitempty"`
	Body        string    `bson:"body,omitempty"`
	PublishedAt time.Time `bson:"published_at,omitempty"`
	AuthorUUID  string    `bson:"author_uuid,omitempty"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
