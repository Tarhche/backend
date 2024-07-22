package comment

import (
	"github.com/khanzadimahdi/testproject/domain/author"
	"time"
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
}
