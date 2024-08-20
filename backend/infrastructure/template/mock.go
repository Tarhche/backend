package template

import (
	"io"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
)

type MockRenderer struct {
	mock.Mock
}

var _ domain.Renderer = &MockRenderer{}

func (r *MockRenderer) Render(writer io.Writer, templateName string, data any) error {
	args := r.Mock.Called(writer, templateName, data)

	return args.Error(0)
}
