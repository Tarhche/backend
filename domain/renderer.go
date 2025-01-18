package domain

import "io"

type Renderer interface {
	Render(writer io.Writer, templateName string, data any) error
}
