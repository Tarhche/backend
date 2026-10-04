//go:build !microsandbox

package main

import "github.com/danceable/console"

// registerOptionalCommands registers nothing in a build without the
// microsandbox tag.
//
// serve-workload-microsandbox drives microsandbox through its SDK, which is a
// cgo library that cannot be linked statically, so only the
// workload-microsandbox image is built with the tag. Every other image, the
// tests and vet are built without it, stay static, and have no such command.
func registerOptionalCommands(*console.Console) {}
