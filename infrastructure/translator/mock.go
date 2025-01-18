package translator

import (
	contract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/stretchr/testify/mock"
)

type TranslatorMock struct {
	mock.Mock
}

func (t *TranslatorMock) Translate(key string, options ...func(*contract.Params)) string {
	args := t.Mock.Called(key, options)

	return args.String(0)
}
