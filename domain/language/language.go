package language

import "context"

type Language struct {
	Code string
	Name string
}

type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Language, error)
	GetByCodes(ctx context.Context, codes []string) ([]Language, error)
	GetOne(ctx context.Context, code string) (Language, error)
	Exists(ctx context.Context, code string) bool
	Save(ctx context.Context, l *Language) (string, error)
	Delete(ctx context.Context, code string) error
	Count(ctx context.Context) (uint, error)
}
