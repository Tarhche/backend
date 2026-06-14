package component

const ComponentTypeItem = "item"

type Item struct {
	// ContentUUID references the article by its correlation uuid (stable across translations).
	ContentUUID string
	ContentType string
}

func (c Item) Items() []Item {
	return []Item{c}
}

func (c Item) Type() string {
	return ComponentTypeItem
}
