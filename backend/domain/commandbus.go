package domain

// CommandHandler provides command handler logic
type CommandHandler interface {
	Handle(command any)
}

type CommandBus interface {
	Register(command any, handler CommandHandler)
	Execute(command any)
}
