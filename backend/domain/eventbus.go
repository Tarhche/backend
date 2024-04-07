package domain

// EventHandler provides event handler logic
type EventHandler interface {
	Handle(event any)
}

type EventBus interface {
	Subscribe(event any, handler EventHandler)
	Publish(event any)
}
