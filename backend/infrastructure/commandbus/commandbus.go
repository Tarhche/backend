package commandbus

import (
	"reflect"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

type HandlerFunc func(command any)

func (h HandlerFunc) Handle(command any) {
	h(command)
}

type bus struct {
	lock     sync.Mutex
	handlers map[reflect.Type]domain.CommandHandler
}

var _ domain.CommandBus = New()

func New() *bus {
	return &bus{
		handlers: make(map[reflect.Type]domain.CommandHandler),
	}
}

func (b *bus) Register(command any, handler domain.CommandHandler) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.handlers[reflect.TypeOf(command)] = handler
}

func (b *bus) Execute(command any) {
	b.lock.Lock()
	defer b.lock.Unlock()

	handler, ok := b.handlers[reflect.TypeOf(command)]
	if !ok {
		return
	}

	handler.Handle(command)
}
