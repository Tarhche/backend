package component

type Item struct {
	UUID string
	Type string
}

func (c Item) Items() []Item {
	return []Item{c}
}
