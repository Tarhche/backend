//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	// boot is how long a VM is given to come up, its image pulled included.
	boot = 5 * time.Minute

	// gib is a gibibyte: sizes are bytes, end to end.
	gib = 1 << 30
)

// serve is a shell line that serves body over HTTP on port, in the
// background, for as long as the VM runs. The default image has perl and no
// python, so perl it is.
func serve(port uint, body string) string {
	return fmt.Sprintf(`setsid nohup perl -MIO::Socket::INET -e '$s=IO::Socket::INET->new(LocalPort=>%d,Listen=>16,ReuseAddr=>1) or die $!; while($c=$s->accept){ while(<$c>){ last if /^\r?$/ } print $c "HTTP/1.0 200 OK\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\n%s\n"; close $c }' >/dev/null 2>&1 &`, port, body)
}

// TestMachineVM walks a machine VM through its life, the way somebody using
// the dashboard would: made, opened in a terminal, its port served through
// the ingress, snapshotted and restored both ways, given another port, and
// deleted.
func TestMachineVM(t *testing.T) {
	marker := "e2e-marker-" + random(6)

	var v vmView
	timings.step(t, "create", func(t *testing.T) {
		user.call(t, http.MethodPost, "/api/dashboard/workload/vms", map[string]any{
			"name":             "e2e machine",
			"kind":             "machine",
			"resources":        resources{CPUs: 2, Memory: 1 * gib, Disk: 4 * gib},
			"ports":            []uint{8000},
			"network":          network{Ingress: "allow", Egress: "allow"},
			"persistent_disk":  true,
			"lifetime_seconds": 3600,
		}, http.StatusCreated, &v)

		if v.Kind != "machine" || len(v.Slug) == 0 || len(v.Image) == 0 {
			t.Fatalf("the vm was made as %+v", v)
		}

		if len(v.URLs) != 1 || !strings.Contains(v.URLs[0].URL, portHost(v.Slug, 8000)) {
			t.Fatalf("the vm's port is served at %+v, wanted %s", v.URLs, portHost(v.Slug, 8000))
		}
	})

	timings.step(t, "running", func(t *testing.T) {
		v = waitForVM(t, v.UUID, "running", boot)
	})

	var term *terminal
	timings.step(t, "terminal echo", func(t *testing.T) {
		term = openTerminal(t, v.UUID)
		term.resize(t, 40, 120)

		if printed := term.run(t, "echo e2e-$((6*7))", time.Minute); !strings.Contains(printed, "e2e-42") {
			t.Fatalf("the terminal printed %q", printed)
		}
	})

	timings.step(t, "terminal refused to anybody else", func(t *testing.T) {
		// whatever the account may do to anybody's VMs, a shell in one is its
		// owner's alone, and anybody else is told there is no such VM.
		conn, status, err := dialTerminal(t, admin, v.UUID)
		if err == nil {
			_ = conn.Close()

			t.Fatal("somebody other than its owner opened a terminal in the vm")
		}

		if status != http.StatusNotFound {
			t.Fatalf("somebody other than its owner is refused a terminal with %d: %v", status, err)
		}
	})

	timings.step(t, "logs", func(t *testing.T) {
		var logs struct {
			Items []struct {
				At     time.Time `json:"at"`
				Source string    `json:"source"`
				Line   string    `json:"line"`
			} `json:"items"`
			Truncated bool `json:"truncated"`
		}
		user.call(t, http.MethodGet, vmPath(v.UUID, "/logs?tail=50"), nil, http.StatusOK, &logs)

		t.Logf("%d log lines; the last: %+v", len(logs.Items), logs.Items[max(0, len(logs.Items)-3):])
	})

	timings.step(t, "stats", func(t *testing.T) {
		eventually(t, "the vm's stats", 2*time.Minute, func() (bool, string) {
			v = getVM(t, v.UUID)
			if v.Stats == nil {
				return false, "none yet"
			}

			return v.Stats.MemoryLimit > 0 && !v.Stats.SampledAt.IsZero(), fmt.Sprintf("%+v", *v.Stats)
		})
	})

	timings.step(t, "port through the ingress", func(t *testing.T) {
		term.run(t, serve(8000, marker), time.Minute)

		waitForPort(t, v.Slug, 8000, marker, time.Minute)
	})

	// what the disk holds when the snapshot is taken, which a stop and a
	// start keep, since the disk is persistent, and both restores have to
	// bring back.
	term.run(t, "echo "+marker+" > /root/e2e-marker && sync", time.Minute)

	timings.step(t, "stop and start", func(t *testing.T) {
		user.call(t, http.MethodPost, vmPath(v.UUID, "/stop"), nil, http.StatusAccepted, nil)
		waitForVM(t, v.UUID, "stopped", 2*time.Minute)

		user.call(t, http.MethodPost, vmPath(v.UUID, "/start"), nil, http.StatusAccepted, nil)
		v = waitForVM(t, v.UUID, "running", boot)

		term = openTerminal(t, v.UUID)
		if printed := term.run(t, "cat /root/e2e-marker", time.Minute); !strings.Contains(printed, marker) {
			t.Fatalf("after a stop and a start, the persistent disk holds %q", printed)
		}
	})

	timings.step(t, "restart", func(t *testing.T) {
		user.call(t, http.MethodPost, vmPath(v.UUID, "/restart"), nil, http.StatusAccepted, nil)

		// a restart ends where it began, so the shell it had is what shows
		// it happened: the terminal open on it is closed.
		eventually(t, "the terminal of vm "+v.UUID, 2*time.Minute, func() (bool, string) {
			select {
			case <-term.closed:
				return true, "closed"
			default:
				return false, "open"
			}
		})

		v = waitForVM(t, v.UUID, "running", boot)
		term = openTerminal(t, v.UUID)
		term.run(t, "true", time.Minute)
	})

	var snapshot snapshotView
	timings.step(t, "snapshot", func(t *testing.T) {
		user.call(t, http.MethodPost, "/api/dashboard/workload/vms/"+v.UUID+"/snapshots", map[string]any{
			"name": "e2e machine snapshot",
		}, http.StatusCreated, &snapshot)

		eventually(t, "snapshot "+snapshot.UUID, 10*time.Minute, func() (bool, string) {
			snapshot = snapshotView{UUID: snapshot.UUID}
			user.call(t, http.MethodGet, "/api/dashboard/my/workload/snapshots/"+snapshot.UUID, nil, http.StatusOK, &snapshot)
			if snapshot.State == "failed" {
				t.Fatalf("the snapshot failed: %s", snapshot.Reason)
			}

			return snapshot.State == "ready", snapshot.State
		})

		if snapshot.Size <= 0 || len(snapshot.Engine) == 0 || snapshot.Kind != "machine" {
			t.Fatalf("the snapshot is %+v", snapshot)
		}

		t.Logf("the snapshot is %d bytes, from %s", snapshot.Size, snapshot.Engine)
	})

	// taking a snapshot may stop the VM for a moment: it is running again
	// before anything else is asked of it.
	v = waitForVM(t, v.UUID, "running", boot)

	timings.step(t, "restore as a new vm", func(t *testing.T) {
		var restored vmView
		user.call(t, http.MethodPost, "/api/dashboard/workload/vms", map[string]any{
			"name":             "e2e machine restored",
			"kind":             "machine",
			"resources":        resources{CPUs: 1, Memory: 1 * gib, Disk: 4 * gib},
			"ports":            []uint{},
			"network":          network{Ingress: "deny", Egress: "deny"},
			"persistent_disk":  true,
			"lifetime_seconds": 3600,
			"snapshot_uuid":    snapshot.UUID,
		}, http.StatusCreated, &restored)

		restored = waitForVM(t, restored.UUID, "running", boot)

		if printed := openTerminal(t, restored.UUID).run(t, "cat /root/e2e-marker", time.Minute); !strings.Contains(printed, marker) {
			t.Fatalf("the vm restored from the snapshot holds %q", printed)
		}

		user.call(t, http.MethodDelete, vmPath(restored.UUID), nil, http.StatusAccepted, nil)
		waitForVMGone(t, restored.UUID, 3*time.Minute)
	})

	timings.step(t, "restore onto the vm", func(t *testing.T) {
		term.run(t, "rm /root/e2e-marker && echo changed > /root/e2e-after && sync", time.Minute)

		user.call(t, http.MethodPost, vmPath(v.UUID, "/restore"), map[string]any{
			"snapshot_uuid": snapshot.UUID,
		}, http.StatusAccepted, nil)

		// it is restoring before it is running again, which is the only way
		// to tell the restore from the VM it already was.
		eventually(t, "vm "+v.UUID, time.Minute, func() (bool, string) {
			v = getVM(t, v.UUID)

			return v.State != "running", v.String()
		})

		v = waitForVM(t, v.UUID, "running", boot)

		term = openTerminal(t, v.UUID)
		printed := term.run(t, "cat /root/e2e-marker; ls /root", time.Minute)

		if !strings.Contains(printed, marker) || strings.Contains(printed, "e2e-after") {
			t.Fatalf("the vm restored onto holds %q", printed)
		}
	})

	timings.step(t, "update ports", func(t *testing.T) {
		user.call(t, http.MethodPatch, vmPath(v.UUID), map[string]any{
			"ports": []uint{8000, 8080},
		}, http.StatusOK, &v)

		if !slices.Equal(v.Ports, []uint{8000, 8080}) {
			t.Fatalf("the vm's ports are %v", v.Ports)
		}

		// a VM's ports are fixed when it boots, so the change restarts it.
		eventually(t, "vm "+v.UUID, time.Minute, func() (bool, string) {
			v = getVM(t, v.UUID)

			return v.State != "running", v.String()
		})

		v = waitForVM(t, v.UUID, "running", boot)

		term = openTerminal(t, v.UUID)
		term.run(t, serve(8000, marker+"-8000"), time.Minute)
		term.run(t, serve(8080, marker+"-8080"), time.Minute)

		waitForPort(t, v.Slug, 8080, marker+"-8080", time.Minute)
		waitForPort(t, v.Slug, 8000, marker+"-8000", time.Minute)
	})

	timings.step(t, "delete", func(t *testing.T) {
		user.call(t, http.MethodDelete, vmPath(v.UUID), nil, http.StatusAccepted, nil)
		waitForVMGone(t, v.UUID, 3*time.Minute)

		// snapshots outlive their VMs, until somebody deletes them.
		user.call(t, http.MethodGet, "/api/dashboard/my/workload/snapshots/"+snapshot.UUID, nil, http.StatusOK, &snapshot)
		user.call(t, http.MethodDelete, "/api/dashboard/my/workload/snapshots/"+snapshot.UUID, nil, http.StatusNoContent, nil)

		if status, _ := user.status(t, http.MethodGet, "/api/dashboard/my/workload/snapshots/"+snapshot.UUID, nil); status != http.StatusNotFound {
			t.Fatalf("the deleted snapshot answers %d", status)
		}
	})
}
