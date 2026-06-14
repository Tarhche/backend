package bookmark

import (
	"time"
)

const (
	ObjectTypeArticle = "article"
)

type Bookmark struct {
	UUID         string
	Title        string
	ObjectUUID   string
	ObjectType   string
	LanguageCode string
	OwnerUUID    string
	CreatedAt    time.Time
}

type Repository interface {
	Save(*Bookmark) (string, error)

	GetAllByOwnerUUID(ownerUUID string, offset uint, limit uint) ([]Bookmark, error)
	CountByOwnerUUID(ownerUUID string) (uint, error)
	GetByOwnerUUID(ownerUUID string, objectType string, objectUUID string, languageCode string) (Bookmark, error)
	DeleteByOwnerUUID(ownerUUID string, objectType string, objectUUID string, languageCode string) error
}
