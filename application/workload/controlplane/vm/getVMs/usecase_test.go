package getVMs

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	// 25 of the owner's, every fifth a Docker VM, and one of somebody else's.
	var stored []vm.VM
	for i := range 25 {
		kind := vm.KindMachine
		if i%5 == 0 {
			kind = vm.KindDocker
		}

		stored = append(stored, vm.VM{UUID: fmt.Sprintf("%02d", i), OwnerUUID: "owner", Kind: kind})
	}

	stored = append(stored, vm.VM{UUID: "99", OwnerUUID: "other", Kind: vm.KindDocker})

	w := vmtest.New(vmtest.WithVMs(stored...))
	useCase := NewUseCase(w.VMs, w.Runs)

	for name, tt := range map[string]struct {
		request    Request
		count      int
		first      string
		totalPages uint
		page       uint
	}{
		"anybody's, the first page": {
			request:    Request{},
			count:      20,
			first:      "99",
			totalPages: 2,
			page:       1,
		},
		"page zero is the first page": {
			request:    Request{Page: 0},
			count:      20,
			totalPages: 2,
			first:      "99",
			page:       1,
		},
		"one person's, the second page": {
			request:    Request{OwnerUUID: "owner", Page: 2},
			count:      5,
			first:      "04",
			totalPages: 2,
			page:       2,
		},
		"one person's docker vms": {
			request:    Request{OwnerUUID: "owner", Kind: vm.KindDocker},
			count:      5,
			first:      "20",
			totalPages: 1,
			page:       1,
		},
		"anybody's docker vms": {
			request:    Request{Kind: vm.KindDocker},
			count:      6,
			first:      "99",
			totalPages: 1,
			page:       1,
		},
		"past the last page": {
			request:    Request{Kind: vm.KindDocker, Page: 3},
			count:      0,
			totalPages: 1,
			page:       3,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := useCase.Execute(context.Background(), &tt.request)
			require.NoError(t, err)

			assert.Len(t, response.Items, tt.count)
			assert.Equal(t, tt.totalPages, response.Pagination.TotalPages)
			assert.Equal(t, tt.page, response.Pagination.CurrentPage)

			if tt.count > 0 {
				assert.Equal(t, tt.first, response.Items[0].UUID)
			}
		})
	}
}

// TestUseCase_Execute_runs lists the code runner's runs among anybody's VMs:
// newest first with the rest, a page at a time, and never in somebody's own
// listing or in a listing of Docker VMs.
func TestUseCase_Execute_runs(t *testing.T) {
	t.Parallel()

	made := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	// 25 VMs, a minute apart, every fifth a Docker VM, all of the owner's.
	var vms []vm.VM
	for i := range 25 {
		v := vmtest.Running(fmt.Sprintf("vm-%02d", i), "owner")
		v.CreatedAt = made.Add(time.Duration(i) * time.Minute)

		if i%5 == 0 {
			v.Kind = vm.KindDocker
		}

		vms = append(vms, v)
	}

	// three runs: one older than most VMs, one between them, and one newer
	// than all of them. A task of anybody else's is no run.
	runAt := func(uuid string, after time.Duration) task.Task {
		run := vmtest.Run(uuid)
		run.CreatedAt = made.Add(after)

		return run
	}

	runs := []task.Task{
		runAt("run-1", 2*time.Minute+30*time.Second),
		runAt("run-2", 12*time.Minute+30*time.Second),
		runAt("run-3", 30*time.Minute),
	}

	notARun := runAt("run-4", 40*time.Minute)
	notARun.OwnerUUID = "owner"

	w := vmtest.New(vmtest.WithVMs(vms...), vmtest.WithTasks(append(runs, notARun)...))
	useCase := NewUseCase(w.VMs, w.Runs)

	// every page of a listing, in order.
	listing := func(t *testing.T, request Request) ([]string, uint) {
		t.Helper()

		var (
			uuids []string
			pages uint
		)

		for page := uint(1); page == 1 || page <= pages; page++ {
			request.Page = page

			response, err := useCase.Execute(context.Background(), &request)
			require.NoError(t, err)
			require.LessOrEqual(t, len(response.Items), int(Limit))

			pages = response.Pagination.TotalPages
			assert.Equal(t, page, response.Pagination.CurrentPage)

			for _, item := range response.Items {
				uuids = append(uuids, item.UUID)
			}
		}

		return uuids, pages
	}

	// what a listing of the given kind would be, newest first.
	newestFirst := func(kind vm.Kind, withRuns bool) []string {
		type made struct {
			uuid string
			at   time.Time
		}

		var all []made
		for _, v := range vms {
			if len(kind) == 0 || v.Kind == kind {
				all = append(all, made{v.UUID, v.CreatedAt})
			}
		}

		if withRuns {
			for _, run := range runs {
				all = append(all, made{run.UUID, run.CreatedAt})
			}
		}

		slices.SortFunc(all, func(a, b made) int { return b.at.Compare(a.at) })

		uuids := make([]string, len(all))
		for i := range all {
			uuids[i] = all[i].uuid
		}

		return uuids
	}

	for name, tt := range map[string]struct {
		request Request
		want    []string
		pages   uint
	}{
		"anybody's have the runs among them, newest first": {
			request: Request{},
			want:    newestFirst("", true),
			pages:   2,
		},
		"anybody's machines have them too": {
			request: Request{Kind: vm.KindMachine},
			want:    newestFirst(vm.KindMachine, true),
			pages:   2,
		},
		"anybody's Docker VMs never have one": {
			request: Request{Kind: vm.KindDocker},
			want:    newestFirst(vm.KindDocker, false),
			pages:   1,
		},
		"somebody's own never have one": {
			request: Request{OwnerUUID: "owner"},
			want:    newestFirst("", false),
			pages:   2,
		},
		"nor somebody's own machines": {
			request: Request{OwnerUUID: "owner", Kind: vm.KindMachine},
			want:    newestFirst(vm.KindMachine, false),
			pages:   1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			uuids, pages := listing(t, tt.request)

			assert.Equal(t, tt.want, uuids)
			assert.Equal(t, tt.pages, pages)
		})
	}

	t.Run("the first page starts with the newest run and splits where the merge says", func(t *testing.T) {
		t.Parallel()

		response, err := useCase.Execute(context.Background(), &Request{Page: 1})
		require.NoError(t, err)
		require.Len(t, response.Items, 20)

		// 28 in all: the newest run, the 12 VMs made after the second run,
		// the second run, and the next 6 VMs.
		assert.Equal(t, "run-3", response.Items[0].UUID)
		assert.Equal(t, "vm-24", response.Items[1].UUID)
		assert.Equal(t, "run-2", response.Items[13].UUID)
		assert.Equal(t, "vm-07", response.Items[19].UUID)
		assert.Equal(t, uint(2), response.Pagination.TotalPages)

		second, err := useCase.Execute(context.Background(), &Request{Page: 2})
		require.NoError(t, err)
		require.Len(t, second.Items, 8)
		assert.Equal(t, "vm-06", second.Items[0].UUID)
		assert.Equal(t, "run-1", second.Items[4].UUID)
	})

	t.Run("a run is the guest's machine, kept by the code runner", func(t *testing.T) {
		t.Parallel()

		response, err := useCase.Execute(context.Background(), &Request{})
		require.NoError(t, err)

		run := response.Items[0]
		assert.Equal(t, "run-3", run.UUID)
		assert.Equal(t, task.GuestOwnerUUID, run.OwnerUUID)
		assert.Equal(t, string(vm.KindMachine), run.Kind)
		assert.Equal(t, vm.ManagedByCodeRunner, run.ManagedBy)
		assert.Equal(t, "running", run.State)

		// and a VM somebody asked for is kept by nobody but the workload.
		assert.Empty(t, response.Items[1].ManagedBy)
	})
}
