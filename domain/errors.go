package domain

import "errors"

var (
	ErrNotExists     = errors.New("not exists")
	ErrAlreadyExists = errors.New("already exists")
)
