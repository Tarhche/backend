// Package initrd builds the initramfs every microVM boots from.
//
// It holds the agent as /init and the directories it mounts over, and nothing
// else: the task's own root comes from its disks, so the one initramfs serves
// every machine whatever it runs. The kernel unpacks it into the machine's
// memory before anything runs there, which is why the agent is a small binary
// of its own.
package initrd

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// the directories the agent mounts over, which the kernel expects to find.
var directories = []string{"dev", "mnt", "proc", "run", "sys", "tmp"}

// Ensure builds the initramfs for the agent at agentPath into dir, unless one
// for the same agent is already there, and says where it is. It is named by
// what it holds, so every orchestrator on a host shares it and a new agent
// never overwrites one a machine is still booting from.
func Ensure(dir string, agentPath string) (string, error) {
	agent, err := os.Open(agentPath)
	if err != nil {
		return "", fmt.Errorf("the agent a machine boots with is not at %s: %w", agentPath, err)
	}
	defer agent.Close()

	info, err := agent.Stat()
	if err != nil {
		return "", err
	}

	digest := sha256.New()
	if _, err := io.Copy(digest, agent); err != nil {
		return "", err
	}

	path := filepath.Join(dir, "initrd-"+hex.EncodeToString(digest.Sum(nil))[:16]+".cpio.gz")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	if _, err := agent.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	// built beside where it goes and moved there whole, so nothing ever boots
	// from half of one.
	temporary, err := os.CreateTemp(dir, ".initrd-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary.Name())

	if err := Build(temporary, agent, info.Size()); err != nil {
		temporary.Close()

		return "", err
	}

	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()

		return "", err
	}

	if err := temporary.Close(); err != nil {
		return "", err
	}

	if err := os.Rename(temporary.Name(), path); err != nil {
		return "", err
	}

	return path, nil
}

// Build writes a compressed initramfs holding init, size bytes long, as /init.
func Build(w io.Writer, init io.Reader, size int64) error {
	compressed := gzip.NewWriter(w)
	archive := &writer{w: compressed}

	for _, dir := range directories {
		if err := archive.directory(dir); err != nil {
			return err
		}
	}

	if err := archive.file("init", 0o755, init, size); err != nil {
		return err
	}

	if err := archive.close(); err != nil {
		return err
	}

	return compressed.Close()
}

// writer writes a cpio archive in the "newc" format, which is the one the
// kernel reads: each entry is a header of hexadecimal fields, its name, and
// its content, each padded to four bytes.
type writer struct {
	w     io.Writer
	inode uint32
}

const (
	modeDirectory = 0o040000
	modeRegular   = 0o100000

	trailer = "TRAILER!!!"
)

func (a *writer) directory(name string) error {
	return a.entry(name, modeDirectory|0o755, 2, nil, 0)
}

func (a *writer) file(name string, perm uint32, content io.Reader, size int64) error {
	return a.entry(name, modeRegular|perm, 1, content, size)
}

func (a *writer) close() error {
	return a.entry(trailer, 0, 1, nil, 0)
}

func (a *writer) entry(name string, mode uint32, links uint32, content io.Reader, size int64) error {
	if size > 0xffffffff {
		return errors.New("an initramfs entry cannot be larger than 4 GiB")
	}

	a.inode++

	header := fmt.Sprintf("070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
		a.inode, mode, 0, 0, links, time.Unix(0, 0).Unix(), size, 0, 0, 0, 0, len(name)+1, 0)

	if _, err := io.WriteString(a.w, header+name+"\x00"); err != nil {
		return err
	}

	if err := a.pad(len(header) + len(name) + 1); err != nil {
		return err
	}

	if content == nil {
		return nil
	}

	written, err := io.CopyN(a.w, content, size)
	if err != nil {
		return err
	}

	return a.pad(int(written))
}

// pad brings what was written up to a multiple of four bytes.
func (a *writer) pad(written int) error {
	if remainder := written % 4; remainder != 0 {
		_, err := a.w.Write(make([]byte, 4-remainder))

		return err
	}

	return nil
}
