package author

type Author struct {
	UUID   string
	Name   string
	Avatar string
}

type Repository interface {
	GetOne(UUID string) (Author, error)
}
