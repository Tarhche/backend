package microsandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestMainCommandOf(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		spec vm.Spec
		want mainCommand
	}{
		{
			name: "a command alone is handed to the image's entrypoint, as the code runner's is",
			spec: vm.Spec{Command: []string{"-c", "print(1)"}},
			want: mainCommand{cmd: []string{"-c", "print(1)"}},
		},
		{
			name: "an entrypoint alone replaces the image's, and its CMD with it",
			spec: vm.Spec{Entrypoint: []string{"/bin/sh"}},
			want: mainCommand{entrypoint: []string{"/bin/sh"}, cmd: []string{}},
		},
		{
			name: "both replace both",
			spec: vm.Spec{Entrypoint: []string{"python"}, Command: []string{"-c", "print(1)"}},
			want: mainCommand{entrypoint: []string{"python"}, cmd: []string{"-c", "print(1)"}},
		},
		{
			name: "neither keeps the image's",
			spec: vm.Spec{},
			want: mainCommand{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, mainCommandOf(tt.spec))
		})
	}
}

func TestEnv(t *testing.T) {
	t.Parallel()

	assert.Nil(t, envOf(nil))
	assert.Equal(t, map[string]string{"A": "1=2", "B": "", "C": "3"}, envOf([]string{"A=1=2", "B", "C=1", "C=3", "=x"}))

	t.Run("a command runs with the instance's environment and its own over it", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t,
			map[string]string{"A": "1", "B": "command"},
			execEnv([]string{"A=1", "B=instance"}, vm.ExecOptions{Env: []string{"B=command"}}),
		)
	})

	t.Run("a terminal says what it is unless the command does", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, map[string]string{"TERM": "xterm-256color"}, execEnv(nil, vm.ExecOptions{TTY: true}))
		assert.Equal(t, map[string]string{"TERM": "dumb"}, execEnv(nil, vm.ExecOptions{TTY: true, Env: []string{"TERM=dumb"}}))
		assert.Nil(t, execEnv(nil, vm.ExecOptions{}))
	})
}

func TestSpawnExitCode(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 127, spawnExitCode("not_found"))
	assert.Equal(t, 126, spawnExitCode("permission_denied"))
	assert.Equal(t, 126, spawnExitCode("not_executable"))
	assert.Equal(t, 126, spawnExitCode(""))
}
