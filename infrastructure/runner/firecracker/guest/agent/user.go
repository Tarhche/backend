//go:build linux

package agent

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// defaultPath is the PATH a task that names none runs with, as a container
// would.
const defaultPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// user is who a process runs as, as the task's root says it.
type user struct {
	uid    uint32
	gid    uint32
	groups []uint32
	home   string
}

// resolveUser reads who spec names out of the task's root: a name or uid,
// optionally with a group or gid after a colon, as an image says it. A name
// has to be in the root's passwd; a uid need not be.
func resolveUser(root string, spec string) (user, error) {
	name, group, _ := strings.Cut(spec, ":")

	resolved := user{home: "/"}

	passwd := readColonFile(filepath.Join(root, "etc/passwd"), 7)

	if len(name) == 0 {
		name = "0"
	}

	entry, found := findEntry(passwd, name, 0, 2)
	uid, isNumber := parseID(name)

	switch {
	case found:
		resolved.uid, _ = parseID(entry[2])
		resolved.gid, _ = parseID(entry[3])
		resolved.home = entry[5]
		name = entry[0]
	case isNumber:
		resolved.uid = uid
	default:
		return user{}, fmt.Errorf("no user %q in the image", name)
	}

	groups := readColonFile(filepath.Join(root, "etc/group"), 4)

	if len(group) > 0 {
		entry, found := findEntry(groups, group, 0, 2)
		gid, isNumber := parseID(group)

		switch {
		case found:
			resolved.gid, _ = parseID(entry[2])
		case isNumber:
			resolved.gid = gid
		default:
			return user{}, fmt.Errorf("no group %q in the image", group)
		}
	}

	// the groups the user is a member of, besides its own, as a login would
	// give them.
	for _, entry := range groups {
		for _, member := range strings.Split(entry[3], ",") {
			if member == name {
				if gid, ok := parseID(entry[2]); ok && gid != resolved.gid {
					resolved.groups = append(resolved.groups, gid)
				}
			}
		}
	}

	if len(resolved.home) == 0 {
		resolved.home = "/"
	}

	return resolved, nil
}

// findEntry finds the entry whose name, or whose id, is key.
func findEntry(entries [][]string, key string, nameField int, idField int) ([]string, bool) {
	for _, entry := range entries {
		if entry[nameField] == key {
			return entry, true
		}
	}

	for _, entry := range entries {
		if entry[idField] == key {
			return entry, true
		}
	}

	return nil, false
}

// readColonFile reads a file of colon-separated entries, keeping those with
// at least fields fields. A root without the file has no entries.
func readColonFile(path string, fields int) [][]string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var entries [][]string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		entry := strings.Split(line, ":")
		if len(entry) >= fields {
			entries = append(entries, entry)
		}
	}

	return entries
}

func parseID(value string) (uint32, bool) {
	id, err := strconv.ParseUint(value, 10, 32)

	return uint32(id), err == nil
}

// lookPath finds a command the way a shell in the task's root would: a name
// with a slash is taken as it is, and any other is looked for along PATH.
//
// Entries are only checked to be there, not followed: an image's symlinks
// point at places inside its own root, which is only where they point once
// the command runs there.
func lookPath(root string, command string, path string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("there is no command to run")
	}

	if strings.Contains(command, "/") {
		return command, nil
	}

	if len(path) == 0 {
		path = defaultPath
	}

	for _, dir := range filepath.SplitList(path) {
		if !filepath.IsAbs(dir) {
			continue
		}

		candidate := filepath.Join(dir, command)

		info, err := os.Lstat(filepath.Join(root, candidate))
		if err != nil || info.IsDir() {
			continue
		}

		return candidate, nil
	}

	return "", fmt.Errorf("%q is not on the image's PATH", command)
}

// envValue is the value of one variable of an environment.
func envValue(env []string, key string) (string, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		if name, value, found := strings.Cut(env[i], "="); found && name == key {
			return value, true
		}
	}

	return "", false
}

// withDefaults gives an environment what every process is started with
// unless it says otherwise.
func withDefaults(env []string, home string) []string {
	result := append([]string(nil), env...)

	if _, found := envValue(result, "PATH"); !found {
		result = append(result, "PATH="+defaultPath)
	}

	if _, found := envValue(result, "HOME"); !found {
		result = append(result, "HOME="+home)
	}

	return result
}
