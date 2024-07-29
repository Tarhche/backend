package articles

import (
	"time"
)

type ArticleBson struct {
	UUID        string    `bson:"_id,omitempty"`
	Cover       string    `bson:"cover,omitempty"`
	Video       string    `bson:"video,omitempty"`
	Title       string    `bson:"title,omitempty"`
	Excerpt     string    `bson:"excerpt,omitempty"`
	Body        string    `bson:"body,omitempty"`
	PublishedAt time.Time `bson:"published_at,omitempty"`
	AuthorUUID  string    `bson:"author_uuid,omitempty"`
	Tags        []string  `bson:"tags,omitempty"`
	ViewCount   uint      `bson:"view_count,omitempty"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
