package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTheBlogsSpecificationCanBeGenerated runs the go:generate directive above
// main, with its output sent somewhere of the test's own, and holds it to
// working.
//
// swag finds a type an annotation names by its package's name. Two packages of
// the same name in what it scans, with the type in both, make the annotation
// mean whichever it happens to read: a presenter package of the control plane's
// beside the dashboard's was enough to stop the blog's specification from
// being generated at all, and nothing else would have said so until somebody
// next ran make generate.
func TestTheBlogsSpecificationCanBeGenerated(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("generating the specification reads the whole module")
	}

	command := generateDirective(t)

	output := slices.Index(command, "--output")
	require.Positive(t, output, "the directive says where the specification goes")
	require.Less(t, output+1, len(command))

	command[output+1] = t.TempDir()

	generate := exec.CommandContext(t.Context(), command[0], command[1:]...)
	said, err := generate.CombinedOutput()
	require.NoError(t, err, "%s", tail(string(said)))

	for _, written := range []string{"docs.go", "swagger.json", "swagger.yaml"} {
		assert.FileExists(t, filepath.Join(command[output+1], written))
	}
}

// generateDirective is the command main's go:generate directive runs.
func generateDirective(t *testing.T) []string {
	t.Helper()

	source, err := os.Open("main.go")
	require.NoError(t, err)
	defer source.Close()

	scanner := bufio.NewScanner(source)
	for scanner.Scan() {
		if directive, found := strings.CutPrefix(scanner.Text(), "//go:generate "); found {
			return strings.Fields(directive)
		}
	}

	require.NoError(t, scanner.Err())
	require.FailNow(t, "main.go has no go:generate directive")

	return nil
}

// tail is the end of what a command said, which is where it says what went
// wrong.
func tail(said string) string {
	lines := strings.Split(strings.TrimSpace(said), "\n")

	return strings.Join(lines[max(len(lines)-5, 0):], "\n")
}

// TestTheBuildContextLeavesOutTheLocalStacksData holds every image's build to
// sending none of what the local stack keeps under ./tmp: its databases' files,
// owned by the containers that write them, are unreadable to whoever builds on
// Linux, and a build that has to read them fails.
func TestTheBuildContextLeavesOutTheLocalStacksData(t *testing.T) {
	t.Parallel()

	ignore, err := os.ReadFile(".dockerignore")
	require.NoError(t, err, "the build context is narrowed by .dockerignore")

	var patterns []string
	for line := range strings.Lines(string(ignore)) {
		if pattern := strings.TrimSpace(line); len(pattern) > 0 && !strings.HasPrefix(pattern, "#") {
			patterns = append(patterns, strings.TrimPrefix(pattern, "/"))
		}
	}

	assert.Contains(t, patterns, "tmp")
}
