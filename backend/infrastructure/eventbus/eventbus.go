package eventbus

import (
	"reflect"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

type HandlerFunc func(event any)

func (h HandlerFunc) Handle(event any) {
	h(event)
}

type bus struct {
	lock     sync.Mutex
	handlers map[reflect.Type][]domain.EventHandler
}

var _ domain.EventBus = New()

func New() *bus {
	return &bus{
		handlers: make(map[reflect.Type][]domain.EventHandler),
	}
}

func (b *bus) Subscribe(event any, handler domain.EventHandler) {
	t := reflect.TypeOf(event)

	b.lock.Lock()
	defer b.lock.Unlock()

	b.handlers[t] = append(b.handlers[t], handler)
}

func (b *bus) Publish(event any) {
	t := reflect.TypeOf(event)

	b.lock.Lock()
	defer b.lock.Unlock()

	for i := range b.handlers[t] {
		b.handlers[t][i].Handle(event)
	}
}
