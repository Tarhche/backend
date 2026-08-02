package bookmark

import (
	"context"
	"time"
)

const (
	ObjectTypeArticle = "article"
	ObjectTypeNote    = "note"
)

// IsValidObjectType reports whether s names something that can be bookmarked.
func IsValidObjectType(s string) bool {
	return s == ObjectTypeArticle || s == ObjectTypeNote
}

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
	Save(ctx context.Context, b *Bookmark) (string, error)

	GetAllByOwnerUUID(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]Bookmark, error)
	CountByOwnerUUID(ctx context.Context, ownerUUID string) (uint, error)
	GetByOwnerUUID(ctx context.Context, ownerUUID string, objectType string, objectUUID string, languageCode string) (Bookmark, error)
	DeleteByOwnerUUID(ctx context.Context, ownerUUID string, objectType string, objectUUID string, languageCode string) error
}
