//go:build microsandbox

// Package sdk drives microsandbox through its Go SDK, behind the port the run
// supervisor declares in application/workload/microsandbox/runs. It is the
// only package in the module that imports the SDK.
//
// The SDK is a cgo binding to a native library it embeds. That library needs
// glibc and cannot be linked statically, and with CGO_ENABLED=0 the SDK does
// not compile at all. So every file here carries the microsandbox build tag,
// and only the workload-microsandbox image is built with it. Every other
// image, go test ./... and go vet stay static and never compile the SDK.
//
// The blank import pins the SDK's version in go.mod. go mod tidy reads every
// build tag, so it keeps the requirement even though untagged builds never see
// this file. The version has to match the msb in the workload-microsandbox
// image: an SDK and an msb of different versions break the database they
// share under MSB_HOME.
package sdk

import (
	// microsandbox's Go SDK, held in go.mod until the adapter here imports
	// it for itself.
	_ "github.com/superradcompany/microsandbox/sdk/go"
)
