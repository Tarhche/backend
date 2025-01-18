package comment

import (
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/domain/author"
)

const (
	ObjectTypeArticle = "article"
)

type Comment struct {
	UUID       string
	Body       string
	Author     author.Author
	ParentUUID string
	ObjectUUID string
	ObjectType string
	ApprovedAt time.Time
	CreatedAt  time.Time
}

type Repository interface {
	GetAll(offset uint, limit uint) ([]Comment, error)
	GetOne(UUID string) (Comment, error)
	Count() (uint, error)
	Save(*Comment) (string, error)
	Delete(UUID string) error

	GetApprovedByObjectUUID(objectType string, UUID string, offset uint, limit uint) ([]Comment, error)
	CountApprovedByObjectUUID(objectType string, UUID string) (uint, error)

	GetAllByAuthorUUID(authorUUID string, offset uint, limit uint) ([]Comment, error)
	GetOneByAuthorUUID(UUID string, authorUUID string) (Comment, error)
	CountByAuthorUUID(authorUUID string) (uint, error)
	DeleteByAuthorUUID(UUID string, authorUUID string) error
}

var (
	ErrUpdatingAnApprovedCommentNotAllowed = errors.New("updating an approved comment is not allowed")
)
