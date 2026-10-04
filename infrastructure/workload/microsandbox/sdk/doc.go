// Package sdk drives microsandbox through its Go SDK, behind the port the run
// supervisor declares in application/workload/microsandbox/runs. It is the
// only package in the module that imports the SDK.
//
// The SDK is a cgo binding to a native library it embeds. That library needs
// glibc and cannot be linked statically, and with CGO_ENABLED=0 the SDK does
// not compile at all. So every file here that touches the SDK carries the
// microsandbox build tag, and only the workload-microsandbox image is built
// with it. What needs no SDK is left untagged: converting the workload's bytes
// and cores to microsandbox's MiB and whole vCPUs, and turning an exec
// session's stream into the port's Process. Untagged builds compile only that,
// so go test ./... covers it and nothing static ever links the SDK. go mod
// tidy reads every build tag, so go.mod keeps the SDK's requirement all the
// same.
//
// The SDK's version has to match the msb in the workload-microsandbox image:
// an SDK and an msb of different versions break the database they share under
// MSB_HOME.
//
// Microsandbox reads its own settings from the environment, as OpenTelemetry's
// SDK reads OTEL_*: MSB_HOME, and MSB_PATH and MSB_LIBKRUNFW_PATH, which point
// it at the image's msb and libkrunfw, so that it never fetches a runtime of
// its own. Nothing here reads them.
package sdk
