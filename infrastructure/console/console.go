package console

import "context"

type Console struct {
}

func NewConsole() *Console {
	return &Console{}
}

func (c *Console) Run(ctx context.Context) int {
	return 0
}
