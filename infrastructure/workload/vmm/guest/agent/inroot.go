package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The agent reaches into the task's root from outside it, to mount what the
// task expects to find there and to bind its own files over the root's. A
// symbolic link in an image points where it does inside the image; followed
// from outside, an absolute one points into the agent's own root instead, so
// a mount made through one would land on the agent's files rather than the
// task's. So a path is followed into the root one name at a time, and a link
// anywhere on the way is refused rather than followed.
//
// Nothing in the machine has run yet when the agent does this, except what a
// writable root kept on its scratch disk from an earlier boot: refusing what
// it does not expect costs the task a mount, never the agent its own root.

// directoryInRoot is the directory at path inside root. With create, a
// directory that is not there is made, and so is every one above it.
func directoryInRoot(root string, path string, create bool) (string, error) {
	names, err := namesOf(path)
	if err != nil {
		return "", err
	}

	current := root
	for _, name := range names {
		current = filepath.Join(current, name)

		info, err := os.Lstat(current)

		switch {
		case err == nil && info.IsDir():
			continue
		case err == nil:
			return "", fmt.Errorf("%s inside the task's root is not a directory", strings.TrimPrefix(current, root))
		case errors.Is(err, os.ErrNotExist) && create:
			if err := os.Mkdir(current, 0o755); err != nil {
				return "", err
			}
		default:
			return "", err
		}
	}

	return current, nil
}

// fileInRoot is the regular file at path inside root. With create, one that
// is not there is made, and anything else in its place — a link above all,
// which could point anywhere — is replaced by an empty one.
func fileInRoot(root string, path string, create bool) (string, error) {
	dir, err := directoryInRoot(root, filepath.Dir(path), create)
	if err != nil {
		return "", err
	}

	file := filepath.Join(dir, filepath.Base(path))

	info, err := os.Lstat(file)

	switch {
	case err == nil && info.Mode().IsRegular():
		return file, nil
	case err == nil && !create:
		return "", fmt.Errorf("%s inside the task's root is not a regular file", path)
	case err == nil:
		if err := os.Remove(file); err != nil {
			return "", err
		}
	case !errors.Is(err, os.ErrNotExist) || !create:
		return "", err
	}

	made, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}

	return file, made.Close()
}

// namesOf is the names an absolute path inside the root is made of. Nothing
// may climb out of the root on the way.
func namesOf(path string) ([]string, error) {
	trimmed := strings.Trim(path, "/")
	if len(trimmed) == 0 {
		return nil, nil
	}

	names := strings.Split(trimmed, "/")
	for _, name := range names {
		if len(name) == 0 || name == "." || name == ".." {
			return nil, fmt.Errorf("%q is not a path inside the task's root", path)
		}
	}

	return names, nil
}
