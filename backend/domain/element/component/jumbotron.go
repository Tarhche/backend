package component

type Jumbotron struct {
	Item
}

func (c Jumbotron) Items() []Item {
	return []Item{c.Item}
}
