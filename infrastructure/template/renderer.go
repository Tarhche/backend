package template

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

type Renderer struct {
	fileSystem fs.FS
	extension  string

	lock  sync.Mutex
	files []string
	cache map[string]*template.Template
}

var _ domain.Renderer = &Renderer{}

func NewRenderer(fileSystem fs.FS, extension string) *Renderer {
	return &Renderer{
		fileSystem: fileSystem,
		extension:  extension,
		cache:      make(map[string]*template.Template),
	}
}

func (r *Renderer) Render(writer io.Writer, templateName string, data any) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	if len(r.files) == 0 {
		files, err := walkDir(r.fileSystem, r.extension)
		if err != nil {
			return err
		}
		slices.Sort(files)

		r.files = files
	}

	index, ok := slices.BinarySearch(r.files, templateName+"."+r.extension)
	if !ok {
		return fmt.Errorf("template not found: %s", templateName)
	}

	if _, ok := r.cache[templateName]; !ok {
		l := len(r.files) - 1
		last := r.files[l]

		r.files[l] = r.files[index]
		t, err := template.New(path.Base(r.files[index])).ParseFS(r.fileSystem, r.files...)
		r.files[l] = last

		if err != nil {
			return err
		}
		r.cache[templateName] = t
	}

	return r.cache[templateName].Execute(writer, data)
}

func walkDir(fileSystem fs.FS, extension string) ([]string, error) {
	var files []string

	err := fs.WalkDir(fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, "."+extension) {
			files = append(files, path)
			return nil
		}

		return nil
	})

	return files, err
}
