package image

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	pathpkg "path"
	"strings"
	"syscall"
)

const (
	// compressionLevel keeps making a disk quick: zstd reads back as fast at
	// any level, and its higher ones take far longer to write.
	compressionLevel = "3"

	// squashMemory is what sqfstar may keep in its caches. Left to itself it
	// takes a quarter of the host's memory, which is the host's and not
	// vmhost's to give: vmhost's own container is held to far less, and
	// sqfstar runs inside it.
	squashMemory = "128M"
)

// squasher makes a filesystem at path out of the tarball write writes to it.
type squasher func(ctx context.Context, path string, write func(io.Writer) error) error

// squashWith makes filesystems with the sqfstar at binary: a squashfs,
// compressed with zstd, of a tarball read on its standard input.
//
// The root is root's, and so is any directory the tarball only implies, as
// they would be in a container: sqfstar would otherwise give them to whoever
// runs it.
func squashWith(binary string) squasher {
	return func(ctx context.Context, path string, write func(io.Writer) error) error {
		command := exec.CommandContext(ctx, binary,
			"-quiet",
			"-no-progress",
			"-mem", squashMemory,
			"-comp", "zstd",
			"-Xcompression-level", compressionLevel,
			"-root-uid", "0",
			"-root-gid", "0",
			"-root-mode", "0755",
			"-default-uid", "0",
			"-default-gid", "0",
			"-default-mode", "0755",
			path,
		)

		var output bytes.Buffer
		command.Stdout = &output
		command.Stderr = &output

		stdin, err := command.StdinPipe()
		if err != nil {
			return err
		}

		if err := command.Start(); err != nil {
			return err
		}

		// the tarball is written here, into sqfstar's standard input, and
		// closing it is how sqfstar is told it has all of it. A tarball cut
		// short is a failure whatever sqfstar makes of it.
		written := write(stdin)
		closed := stdin.Close()
		waited := command.Wait()

		if waited != nil {
			waited = fmt.Errorf("sqfstar failed: %w: %s", waited, strings.TrimSpace(output.String()))

			// sqfstar going away is why the tarball could not be written to
			// it, and says why itself.
			if errors.Is(written, syscall.EPIPE) {
				written = nil
			}
		}

		if written != nil {
			written = fmt.Errorf("the image's layers cannot be read: %w", written)
		}

		return errors.Join(written, waited, closed)
	}
}

// normalize copies a tarball, naming every entry the way sqfstar takes it:
// relative, and with nothing in it that is "." or "..". Layers are written by
// whatever built the image, and plenty of them name their entries "./bin/sh"
// or have one for "./" itself. The root is left out, since sqfstar makes it.
func normalize(in io.Reader, out io.Writer) error {
	source := tar.NewReader(in)
	target := tar.NewWriter(out)

	for {
		header, err := source.Next()
		if errors.Is(err, io.EOF) {
			return target.Close()
		}

		if err != nil {
			return err
		}

		header.Name = cleanEntry(header.Name)
		if len(header.Name) == 0 {
			continue
		}

		// a hard link names another entry of the tarball, which is named the
		// same way; a symlink's target is what the link says, and is left as
		// it is.
		if header.Typeflag == tar.TypeLink {
			header.Linkname = cleanEntry(header.Linkname)
		}

		if err := target.WriteHeader(header); err != nil {
			return err
		}

		if _, err := io.Copy(target, source); err != nil {
			return err
		}
	}
}

// cleanEntry is an entry's name relative to the root, or empty for the root
// itself. It is cleaned as though the root were the whole filesystem, so a
// name climbing out of the root lands inside it instead, as it would inside a
// container.
func cleanEntry(name string) string {
	return strings.TrimPrefix(pathpkg.Clean("/"+name), "/")
}
