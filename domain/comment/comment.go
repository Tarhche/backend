package comment

import (
	"context"
	"errors"
	"time"
)

const (
	ObjectTypeArticle = "article"
)

type Comment struct {
	UUID         string
	Body         string
	AuthorUUID   string
	ParentUUID   string
	ObjectUUID   string
	ObjectType   string
	LanguageCode string
	ApprovedAt   time.Time
	CreatedAt    time.Time
}

type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Comment, error)
	GetOne(ctx context.Context, UUID string) (Comment, error)
	Count(ctx context.Context) (uint, error)
	Save(ctx context.Context, c *Comment) (string, error)
	Delete(ctx context.Context, UUID string) error

	GetApprovedByObjectUUID(ctx context.Context, objectType string, UUID string, languageCode string, offset uint, limit uint) ([]Comment, error)
	CountApprovedByObjectUUID(ctx context.Context, objectType string, UUID string, languageCode string) (uint, error)

	GetAllByAuthorUUID(ctx context.Context, authorUUID string, offset uint, limit uint) ([]Comment, error)
	GetOneByAuthorUUID(ctx context.Context, UUID string, authorUUID string) (Comment, error)
	CountByAuthorUUID(ctx context.Context, authorUUID string) (uint, error)
	DeleteByAuthorUUID(ctx context.Context, UUID string, authorUUID string) error
}

var (
	ErrUpdatingAnApprovedCommentNotAllowed = errors.New("updating an approved comment is not allowed")
)
