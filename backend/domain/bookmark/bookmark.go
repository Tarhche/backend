package bookmark

import (
	"time"
)

const (
	ObjectTypeArticle = "article"
)

type Bookmark struct {
	UUID       string
	ObjectUUID string
	ObjectType string
	OwnerUUID  string
	CreatedAt  time.Time
}

type Repository interface {
	Save(*Bookmark) (string, error)
	Count(objectType string, objectUUID string) (uint, error)

	GetAllByOwnerUUID(ownerUUID string, offset uint, limit uint) ([]Bookmark, error)
	CountByOwnerUUID(ownerUUID string) (uint, error)
	GetByOwnerUUID(ownerUUID string, objectType string, objectUUID string) (Bookmark, error)
	DeleteByOwnerUUID(ownerUUID string, objectType string, objectUUID string) error
}
