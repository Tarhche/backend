package firecracker

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The files that say which users, groups and subordinate ids a host gives out.
var (
	userFiles        = []string{"/etc/passwd", "/etc/group"}
	subordinateFiles = []string{"/etc/subuid", "/etc/subgid"}
)

// takenIDs is whatever already holds one of the count ids from first, which
// machines' users and groups are numbered from: a user or a group with one of
// them, or a range of subordinate ids that takes one in, which a user's
// containers map their own users onto. Whoever holds one runs as a machine's
// user, and could read the machine's disks.
//
// It reads the files where it runs. On a host that is the host's own account,
// which vmhost's preflight on the host takes too; inside vmhost's container it
// is only the image's.
func takenIDs(first int, count int, users []string, subordinates []string) []string {
	last := first + count - 1

	var taken []string

	for _, path := range users {
		forEachLine(path, func(fields []string) {
			if len(fields) < 3 {
				return
			}

			if id, err := strconv.Atoi(fields[2]); err == nil && id >= first && id <= last {
				taken = append(taken, fmt.Sprintf("%s in %s (%d)", fields[0], path, id))
			}
		})
	}

	for _, path := range subordinates {
		forEachLine(path, func(fields []string) {
			if len(fields) < 3 {
				return
			}

			start, startErr := strconv.Atoi(fields[1])
			size, sizeErr := strconv.Atoi(fields[2])

			if startErr == nil && sizeErr == nil && size > 0 && start <= last && start+size-1 >= first {
				taken = append(taken, fmt.Sprintf("the range %s holds in %s (%d-%d)", fields[0], path, start, start+size-1))
			}
		})
	}

	return taken
}

// forEachLine hands every line of a file of colon-separated fields to each, a
// file that is not there having none.
func forEachLine(path string, each func(fields []string)) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		each(strings.Split(line, ":"))
	}
}
