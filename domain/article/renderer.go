package article

import "io"

type Renderer interface {
	Render(io.Writer, Entity) error
	RenderIndex(io.Writer, []Entity) error
}
