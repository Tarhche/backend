package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkloadPermission(t *testing.T) {
	testCases := []struct {
		permission string
		want       string
		renamed    bool
	}{
		{permission: "runner.tasks.index", want: "workload.tasks.index", renamed: true},
		{permission: "runner.stacks.manage", want: "workload.stacks.manage", renamed: true},
		{permission: "self.runner.tasks.attach", want: "self.workload.tasks.attach", renamed: true},
		{permission: "articles.index", want: "articles.index"},
		{permission: "self.articles.index", want: "self.articles.index"},
		{permission: "workload.tasks.index", want: "workload.tasks.index"},
		{permission: "runners.tasks.index", want: "runners.tasks.index"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.permission, func(t *testing.T) {
			got, renamed := workloadPermission(testCase.permission)

			assert.Equal(t, testCase.want, got)
			assert.Equal(t, testCase.renamed, renamed)
		})
	}
}

func TestOrchestratorNodeName(t *testing.T) {
	testCases := []struct {
		name    string
		want    string
		renamed bool
	}{
		{name: "runner-worker-01", want: "workload-orchestrator-01", renamed: true},
		{name: "runner-orchestrator-03", want: "workload-orchestrator-03", renamed: true},
		{name: "workload-orchestrator-02", want: "workload-orchestrator-02"},
		{name: "0bf9dcf45b57", want: "0bf9dcf45b57"},
		{name: "runner-controlplane", want: "runner-controlplane"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, renamed := orchestratorNodeName(testCase.name)

			assert.Equal(t, testCase.want, got)
			assert.Equal(t, testCase.renamed, renamed)
		})
	}
}
