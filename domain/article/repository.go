package article

type Repository interface {
	Articles() ([]Entity, error)
	CreateArticle(*Entity) error
	Article(id string) (*Entity, error)
	UpdateArticle(*Entity) error
	DeleteArticle(string) error
}
