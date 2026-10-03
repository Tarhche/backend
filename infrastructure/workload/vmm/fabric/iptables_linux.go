//go:build linux

package fabric

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// iptables is the firewall of the namespace vmhost runs in, through the
// iptables-save and iptables-restore of vmhost's image. Whichever of
// iptables' backends they are, legacy or nf_tables, the fabric's rules are
// all written with it, and read back the way they were written.
type iptables struct {
	saveBinary    string
	restoreBinary string
}

// iptablesTables finds the iptables tools.
func iptablesTables() (tables, error) {
	save, err := exec.LookPath("iptables-save")
	if err != nil {
		return nil, fmt.Errorf("machines' networks cannot be kept apart without iptables-save: %w", err)
	}

	restore, err := exec.LookPath("iptables-restore")
	if err != nil {
		return nil, fmt.Errorf("machines' networks cannot be kept apart without iptables-restore: %w", err)
	}

	return iptables{saveBinary: save, restoreBinary: restore}, nil
}

func (i iptables) save(ctx context.Context) (string, error) {
	var stdout, stderr bytes.Buffer

	command := exec.CommandContext(ctx, i.saveBinary)
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		return "", fmt.Errorf("%s failed: %w: %s", i.saveBinary, err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}

// restore writes the input in one transaction a table, leaving every chain
// it does not declare as it is (--noflush).
func (i iptables) restore(ctx context.Context, input string) error {
	command := exec.CommandContext(ctx, i.restoreBinary, "--noflush")
	command.Stdin = strings.NewReader(input)

	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%s failed: %w: %s", i.restoreBinary, err, strings.TrimSpace(string(output)))
	}

	return nil
}
