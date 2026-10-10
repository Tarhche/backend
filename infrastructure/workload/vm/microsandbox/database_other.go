//go:build !linux

package microsandbox

import "io"

// holdDatabase holds nothing but on Linux, the only host microsandbox's VMs
// run on here.
func holdDatabase(string) (io.Closer, error) {
	return nothingHeld{}, nil
}

type nothingHeld struct{}

func (nothingHeld) Close() error {
	return nil
}
