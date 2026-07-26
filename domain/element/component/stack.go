package component

const ComponentTypeStack = "stack"

// Stack renders its items as a stacked list (e.g. the articles of a series).
type Stack struct {
	// HighlightCurrent tells the renderer to highlight the item the visitor is currently on.
	HighlightCurrent bool
	// VisibleNeighbors is how many items are shown before and after the current one.
	VisibleNeighbors uint
	ItemsList        []Item
}

func (c Stack) Items() []Item {
	return c.ItemsList
}

func (c Stack) Type() string {
	return ComponentTypeStack
}
