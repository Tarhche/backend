//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/khanzadimahdi/testproject/resources/translation"
)

// setting as a number, for the workload's bounds the stack runs with.
func numberSetting(t testing.TB, name string, fallback uint) uint {
	t.Helper()

	n, err := strconv.ParseUint(setting(name, strconv.FormatUint(uint64(fallback), 10)), 10, 32)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}

	return uint(n)
}

// TestQuota asks for one VM more than its owner's quota allows, and is refused
// in the reader's language, field by field, with nothing made.
//
// The quota held to is vCPUs: whatever the run's account holds already is
// topped up to the quota with small VMs, each within the most one VM may have,
// so that the VM asked for next, with a single vCPU, is the one that goes
// past it. The bounds are the control plane's defaults unless E2E_USER_CPUS
// and E2E_MAX_CPUS say what it was given instead.
func TestQuota(t *testing.T) {
	quota := numberSetting(t, "E2E_USER_CPUS", 8)
	most := numberSetting(t, "E2E_MAX_CPUS", 4)

	asked := func(name string, cpus uint) map[string]any {
		return map[string]any{
			"name":             name,
			"kind":             "machine",
			"resources":        resources{CPUs: cpus, Memory: 256 << 20, Disk: 1 * gib},
			"ports":            []uint{},
			"network":          network{Ingress: "deny", Egress: "deny"},
			"persistent_disk":  false,
			"lifetime_seconds": 900,
		}
	}

	// the fillers go when the test ends, rather than when the step that made
	// them does: the refusal needs them there.
	var fillers []string
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())

		for _, uuid := range fillers {
			status, body, err := user.request(ctx, http.MethodDelete, vmPath(uuid), nil)
			if err == nil && status != http.StatusAccepted && status != http.StatusNotFound {
				err = fmt.Errorf("%d %s", status, body)
			}

			if err != nil {
				t.Errorf("deleting a filler: %v", err)
			}
		}
	})

	timings.step(t, "fill the quota", func(t *testing.T) {
		var vms page[vmView]
		user.call(t, http.MethodGet, "/api/dashboard/my/workload/vms", nil, http.StatusOK, &vms)

		var held uint
		for _, v := range vms.Items {
			if v.State != "deleting" {
				held += v.Resources.CPUs
			}
		}

		for held < quota {
			cpus := min(most, quota-held)

			var filler vmView
			user.call(t, http.MethodPost, "/api/dashboard/workload/vms", asked("e2e quota filler", cpus), http.StatusCreated, &filler)

			fillers = append(fillers, filler.UUID)
			held += cpus
		}

		t.Logf("the account holds %d of its %d vCPUs", held, quota)
	})

	timings.step(t, "refused past it", func(t *testing.T) {
		for _, language := range []string{translation.EN, translation.FA} {
			token, err := user.token(t.Context())
			if err != nil {
				t.Fatal(err)
			}

			status, body, err := user.requestWith(t.Context(), http.MethodPost, "/api/dashboard/workload/vms", token, language, asked("e2e one too many", 1))
			if err != nil {
				t.Fatal(err)
			}

			var refusal struct {
				Errors map[string]string `json:"errors"`
			}
			if status != http.StatusBadRequest || json.Unmarshal(body, &refusal) != nil {
				t.Fatalf("a VM past the quota is answered %d: %s", status, body)
			}

			want := translation.Translations[language]["quota_exceeded"]
			if got := refusal.Errors["resources.cpus"]; got != want || len(want) == 0 {
				t.Fatalf("in %s, a VM past the quota is refused with %q, wanted %q: %s", language, got, want, body)
			}
		}

		// nothing was made of either.
		var vms page[vmView]
		user.call(t, http.MethodGet, "/api/dashboard/my/workload/vms", nil, http.StatusOK, &vms)
		for _, v := range vms.Items {
			if v.Name == "e2e one too many" {
				t.Fatalf("a VM refused for the quota was made: %+v", v)
			}
		}
	})
}
