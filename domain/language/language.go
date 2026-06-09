package language

type Language struct {
	Code string
	Name string
}

type Repository interface {
	GetAll(offset uint, limit uint) ([]Language, error)
	GetByCodes(codes []string) ([]Language, error)
	GetOne(code string) (Language, error)
	Exists(code string) bool
	Save(*Language) (string, error)
	Delete(code string) error
	Count() (uint, error)
}
