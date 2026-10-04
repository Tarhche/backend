package layout

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Install copies the file at source into dir, named name-<digest> after what
// it holds, unless one holding the same is there already, and says where it
// is.
//
// It is how vmhost puts what machines run — the firecracker binary, the kernel
// — where the host can run it from: a machine's unit is the host's, and runs
// what is in the data directory rather than what is in vmhost's image. Named by
// what it holds, a vmhost that is upgraded puts the new one beside the old,
// and never replaces one a running machine was started from.
func Install(dir string, source string, name string, mode os.FileMode) (string, error) {
	in, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("%s, which machines need, is not at %s: %w", name, source, err)
	}
	defer in.Close()

	digest := sha256.New()
	if _, err := io.Copy(digest, in); err != nil {
		return "", err
	}

	path := filepath.Join(dir, name+"-"+hex.EncodeToString(digest.Sum(nil))[:16])
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		return path, os.Chmod(path, mode)
	}

	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	// copied beside where it goes and moved there whole, so nothing ever
	// runs half of one.
	temporary, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary.Name())

	if _, err := io.Copy(temporary, in); err != nil {
		temporary.Close()

		return "", err
	}

	if err := temporary.Chmod(mode); err != nil {
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
