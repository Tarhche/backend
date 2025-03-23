package view

import (
	"embed"
)

// Files is a shared embed.FS instance for view files
//
//go:embed *
var Files embed.FS
