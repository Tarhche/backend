package migrations

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/permission"
)

func TestReplacedTaskPermissions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		permissions []string
		want        []string
		replaced    bool
	}{
		{
			name:        "a role that could list tasks can list what replaced them",
			permissions: []string{"articles.index", "workload.tasks.index"},
			want:        []string{"articles.index", "workload.vms.index", "workload.snapshots.index", "workload.containers.index"},
			replaced:    true,
		},
		{
			name:        "managing a task is managing, and updating, what replaced it",
			permissions: []string{"self.workload.tasks.manage"},
			want:        []string{"self.workload.vms.manage", "self.workload.vms.update", "self.workload.snapshots.update", "self.workload.containers.manage"},
			replaced:    true,
		},
		{
			name:        "a terminal in a task is a terminal in a vm",
			permissions: []string{"workload.tasks.attach", "self.workload.tasks.attach"},
			want:        []string{"workload.vms.attach", "self.workload.vms.attach"},
			replaced:    true,
		},
		{
			name:        "what a role already holds is not granted twice",
			permissions: []string{"workload.vms.logs", "workload.tasks.logs"},
			want:        []string{"workload.vms.logs", "workload.containers.logs"},
			replaced:    true,
		},
		{
			name:        "the stack permissions keep their names",
			permissions: []string{"workload.stacks.manage", "self.workload.stacks.index", "workload.tasks.show"},
			want:        []string{"workload.stacks.manage", "self.workload.stacks.index", "workload.vms.show", "workload.snapshots.show", "workload.containers.show"},
			replaced:    true,
		},
		{
			name:        "a task permission nothing replaces still goes",
			permissions: []string{"comments.index", "workload.tasks.kill"},
			want:        []string{"comments.index"},
			replaced:    true,
		},
		{
			name:        "a role with no task permissions is left as it is",
			permissions: []string{"articles.index", "workload.stacks.index"},
			want:        []string{"articles.index", "workload.stacks.index"},
			replaced:    false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, replaced := replacedTaskPermissions(slices.Clone(testCase.permissions))

			assert.Equal(t, testCase.want, got)
			assert.Equal(t, testCase.replaced, replaced)
		})
	}
}

// TestTaskPermissionReplacementsExist holds the replacements, which are
// written out as they were, to the permissions the application knows now: a
// role granted one that is not there would hold a permission nothing asks for.
func TestTaskPermissionReplacementsExist(t *testing.T) {
	t.Parallel()

	known := []string{
		permission.WorkloadVMsIndex, permission.WorkloadVMsCreate, permission.WorkloadVMsShow, permission.WorkloadVMsUpdate,
		permission.WorkloadVMsDelete, permission.WorkloadVMsManage, permission.WorkloadVMsLogs, permission.WorkloadVMsAttach,
		permission.WorkloadSnapshotsIndex, permission.WorkloadSnapshotsCreate, permission.WorkloadSnapshotsShow,
		permission.WorkloadSnapshotsUpdate, permission.WorkloadSnapshotsDelete,
		permission.WorkloadContainersIndex, permission.WorkloadContainersCreate, permission.WorkloadContainersShow,
		permission.WorkloadContainersDelete, permission.WorkloadContainersManage, permission.WorkloadContainersLogs,
		permission.SelfWorkloadVMsIndex, permission.SelfWorkloadVMsShow, permission.SelfWorkloadVMsUpdate,
		permission.SelfWorkloadVMsDelete, permission.SelfWorkloadVMsManage, permission.SelfWorkloadVMsLogs, permission.SelfWorkloadVMsAttach,
		permission.SelfWorkloadSnapshotsIndex, permission.SelfWorkloadSnapshotsShow, permission.SelfWorkloadSnapshotsUpdate,
		permission.SelfWorkloadSnapshotsDelete,
		permission.SelfWorkloadContainersIndex, permission.SelfWorkloadContainersShow, permission.SelfWorkloadContainersDelete,
		permission.SelfWorkloadContainersManage, permission.SelfWorkloadContainersLogs,
	}

	granted := make(map[string]bool)
	for old, replacements := range taskPermissionReplacements {
		assert.True(t, isTaskPermission(old), old)

		for _, replacement := range replacements {
			assert.Contains(t, known, replacement, "%s is replaced by %s", old, replacement)
			granted[replacement] = true
		}
	}

	// and every one of them is given to somebody who held what it replaced.
	for _, name := range known {
		assert.True(t, granted[name], "%s is granted to nobody", name)
	}
}
