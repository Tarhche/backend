package runCode

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		want    domain.ValidationErrors
	}{
		{
			name: "valid request with go-1.24",
			request: Request{
				Code:   "package main\nfunc main() {}",
				Runner: "go-1.24",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with go-1.23",
			request: Request{
				Code:   "package main\nfunc main() {}",
				Runner: "go-1.23",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with nodejs-23.11",
			request: Request{
				Code:   "console.log('hello')",
				Runner: "nodejs-23.11",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with nodejs-22.14",
			request: Request{
				Code:   "console.log('hello')",
				Runner: "nodejs-22.14",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with nodejs-20.19",
			request: Request{
				Code:   "console.log('hello')",
				Runner: "nodejs-20.19",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with php-8.4",
			request: Request{
				Code:   "<?php echo 'hello'; ?>",
				Runner: "php-8.4",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with php-8.3",
			request: Request{
				Code:   "<?php echo 'hello'; ?>",
				Runner: "php-8.3",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with nats-2.10.0",
			request: Request{
				Code:   "some nats code",
				Runner: "nats-2.10.0",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty code",
			request: Request{
				Code:   "",
				Runner: "go-1.24",
			},
			want: domain.ValidationErrors{
				"code": "required_field",
			},
		},
		{
			name: "invalid request with empty runner",
			request: Request{
				Code:   "package main\nfunc main() {}",
				Runner: "",
			},
			want: domain.ValidationErrors{
				"runner": "invalid_value",
			},
		},
		{
			name: "invalid request with unsupported runner",
			request: Request{
				Code:   "package main\nfunc main() {}",
				Runner: "python-3.11",
			},
			want: domain.ValidationErrors{
				"runner": "invalid_value",
			},
		},
		{
			name: "invalid request with both empty",
			request: Request{
				Code:   "",
				Runner: "",
			},
			want: domain.ValidationErrors{
				"code":   "required_field",
				"runner": "invalid_value",
			},
		},
		{
			name: "invalid request with empty code and unsupported runner",
			request: Request{
				Code:   "",
				Runner: "python-3.11",
			},
			want: domain.ValidationErrors{
				"code":   "required_field",
				"runner": "invalid_value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.request.Validate()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRequest_Image(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		want    string
	}{
		{
			name: "returns correct image URL for go-1.24",
			request: Request{
				Runner: "go-1.24",
			},
			want: "ghcr.io/tarhche/code-runner:go-1.24-latest",
		},
		{
			name: "returns correct image URL for nodejs-23.11",
			request: Request{
				Runner: "nodejs-23.11",
			},
			want: "ghcr.io/tarhche/code-runner:nodejs-23.11-latest",
		},
		{
			name: "returns correct image URL for php-8.4",
			request: Request{
				Runner: "php-8.4",
			},
			want: "ghcr.io/tarhche/code-runner:php-8.4-latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.request.Image()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRequest_ResourceLimits(t *testing.T) {
	t.Parallel()

	defaults := taskKind.Limits{CPU: 2, Memory: 200 << 20, Disk: 100 << 20}

	// a Go snippet is built from source before it runs, standard library
	// and all: its compiler is killed in the default's memory, and a build of
	// one that imports net/http does not fit in the default's disk.
	goes := taskKind.Limits{CPU: 2, Memory: 512 << 20, Disk: 512 << 20}

	// every runner there is, so that one added is one somebody has decided
	// what to give.
	want := map[string]taskKind.Limits{
		"go-1.24":      goes,
		"go-1.23":      goes,
		"nodejs-23.11": defaults,
		"nodejs-22.14": defaults,
		"nodejs-20.19": defaults,
		"php-8.4":      defaults,
		"php-8.3":      defaults,
		"nats-2.10.0":  defaults,
	}

	assert.ElementsMatch(t, supportedCodeRunners, slices.Collect(maps.Keys(want)))

	for runner, limits := range want {
		t.Run(runner, func(t *testing.T) {
			t.Parallel()

			request := Request{Runner: runner}
			assert.Equal(t, limits, request.ResourceLimits())
		})
	}
}

func TestKeepable(t *testing.T) {
	t.Parallel()

	keepable := func(t *testing.T, r Request) bool {
		t.Helper()

		payload, err := json.Marshal(r)
		require.NoError(t, err)

		return Keepable(payload)
	}

	t.Run("a snippet that prints something prints the same thing", func(t *testing.T) {
		t.Parallel()

		assert.True(t, keepable(t, Request{ID: "request-id", Code: "print", Runner: "go-1.24"}))
	})

	t.Run("one that serves a port is a task to be reached", func(t *testing.T) {
		t.Parallel()

		assert.False(t, keepable(t, Request{ID: "request-id", Code: "serve", Runner: "go-1.24", Ports: []port.Port{8080}}))
	})

	t.Run("and so is one somebody is given a way into", func(t *testing.T) {
		t.Parallel()

		assert.False(t, keepable(t, Request{ID: "request-id", Code: "sleep", Runner: "go-1.24", Terminal: true}))
	})
}
