package component

const ComponentTypeFeatured = "featured"

type Featured struct {
	Main  Item
	Aside []Item
}

func (c Featured) Items() []Item {
	items := make([]Item, 0, len(c.Aside)+1)
	items = append(items, c.Aside...)
	items = append(items, c.Main)

	return items
}

func (c Featured) Type() string {
	return ComponentTypeFeatured
}
