package runCode

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
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
