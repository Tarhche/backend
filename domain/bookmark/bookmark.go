package bookmark

import (
	"context"
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
	Save(ctx context.Context, b *Bookmark) (string, error)

	GetAllByOwnerUUID(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]Bookmark, error)
	CountByOwnerUUID(ctx context.Context, ownerUUID string) (uint, error)
	GetByOwnerUUID(ctx context.Context, ownerUUID string, objectType string, objectUUID string, languageCode string) (Bookmark, error)
	DeleteByOwnerUUID(ctx context.Context, ownerUUID string, objectType string, objectUUID string, languageCode string) error
}
