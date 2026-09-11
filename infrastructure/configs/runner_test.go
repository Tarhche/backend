package configs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunnerWorker_IngressAddresses(t *testing.T) {
	tests := []struct {
		name  string
		given string
		want  []string
	}{
		{name: "one ingress", given: "runner-ingress:81", want: []string{"runner-ingress:81"}},
		{name: "several", given: "a:81,b:81,c:81", want: []string{"a:81", "b:81", "c:81"}},
		{name: "spaces around them are not part of them", given: " a:81 , b:81 ", want: []string{"a:81", "b:81"}},
		{name: "an empty one is not an ingress", given: "a:81,,b:81", want: []string{"a:81", "b:81"}},
		{name: "none at all", given: "", want: []string{}},
		{name: "nothing but separators", given: " , ", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := RunnerWorker{TunnelAddresses: tt.given}

			assert.Equal(t, tt.want, c.IngressAddresses())
		})
	}
}
