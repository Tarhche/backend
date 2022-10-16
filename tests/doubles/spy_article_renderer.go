package doubles

import (
	"github.com/Tarhche/backend/domain/article"
	"io"
)

type SpyArticleRenderer struct {
	CallRenderCounter, CallRenderIndexCounter int
}

func (s *SpyArticleRenderer) Render(w io.Writer, article article.Entity) error {
	s.CallRenderCounter++
	return nil
}

func (s *SpyArticleRenderer) RenderIndex(w io.Writer, articles []article.Entity) error {
	s.CallRenderIndexCounter++
	return nil
}
