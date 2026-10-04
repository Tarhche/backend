package client_test

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package if anything the tests started is still running
// once they are done: a followed log, an exec's stream or a connection kept
// for the next request that outlives its test is one that would outlive a
// task in an orchestrator.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
