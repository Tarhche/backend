//go:build linux

package firecracker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	systemd "github.com/coreos/go-systemd/v22/dbus"
	"github.com/godbus/dbus/v5"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// the errors systemd answers with that are not failures here.
const (
	errNoSuchUnit = "org.freedesktop.systemd1.NoSuchUnit"
	errUnitExists = "org.freedesktop.systemd1.UnitExists"
)

// units starts machines' firecrackers as transient units of the host's
// systemd, over D-Bus (unitProperties says what they are). A unit is the
// host's, not vmhost's: it runs on while vmhost's container is redeployed, and
// is found again by its name.
type units struct {
	slice     string
	namespace string

	lock sync.Mutex
	conn *systemd.Conn
}

var _ launcher = (*units)(nil)

// newUnits connects to the host's systemd, which machines' units are asked of.
func newUnits(slice string, namespace string) (*units, error) {
	u := &units{slice: slice, namespace: namespace}

	if _, err := u.bus(); err != nil {
		return nil, err
	}

	return u, nil
}

// bus is the connection to the host's systemd, made again if it was lost, as it
// is when the host's D-Bus is restarted.
//
// The connection is the hypervisor's, kept for as long as it is, rather than
// any one request's: one made with a request's context ends with the request.
func (u *units) bus() (*systemd.Conn, error) {
	u.lock.Lock()
	defer u.lock.Unlock()

	if u.conn != nil && u.conn.Connected() {
		return u.conn, nil
	}

	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
	}

	conn, err := systemd.NewWithContext(context.Background())
	if err != nil {
		return nil, fmt.Errorf("%w: the host's systemd cannot be reached over D-Bus: %v", vm.ErrUnavailable, err)
	}

	u.conn = conn

	return conn, nil
}

func (u *units) start(ctx context.Context, l launch) (process, error) {
	bus, err := u.bus()
	if err != nil {
		return process{}, err
	}

	name := unitName(l.id)
	properties := unitProperties(l, u.slice, u.namespace)

	// systemd says how the start went once it has gone: done, once the
	// firecracker has been started. The answer is buffered, since systemd's
	// answers are handed over in turn and one nobody waits for any more would
	// hold up every other.
	result := make(chan string, 1)

	_, err = bus.StartTransientUnitContext(ctx, name, "fail", properties, result)
	if errorName(err) == errUnitExists {
		// a unit of an earlier boot of the machine that ended, and has not
		// been let go of yet.
		if err := u.release(ctx, bus, name); err != nil {
			return process{}, err
		}

		_, err = bus.StartTransientUnitContext(ctx, name, "fail", properties, result)
	}

	if err != nil {
		return process{}, fmt.Errorf("the machine's unit %s was refused: %w", name, err)
	}

	if err := await(ctx, result, startTimeout); err != nil {
		return process{}, fmt.Errorf("the machine's unit %s did not start: %w; the host's journal says why", name, err)
	}

	ended := func() error {
		p, err := u.process(ctx, bus, name)
		if err != nil {
			return err
		}

		if !p.running {
			return fmt.Errorf("the machine's unit %s ended before its firecracker was ready", name)
		}

		return nil
	}

	if err := awaitAPI(ctx, l.apiSocket, ended); err != nil {
		return process{}, err
	}

	return u.process(ctx, bus, name)
}

func (u *units) find(ctx context.Context) (map[string]process, error) {
	bus, err := u.bus()
	if err != nil {
		return nil, err
	}

	listed, err := bus.ListUnitsByPatternsContext(ctx, nil, []string{unitPattern})
	if err != nil {
		return nil, fmt.Errorf("the host's systemd did not say which machines' units it holds: %w", err)
	}

	found := make(map[string]process)

	for _, unit := range listed {
		id, ok := machineOfUnit(unit.Name)
		if !ok || unit.LoadState != "loaded" {
			continue
		}

		p, err := u.process(ctx, bus, unit.Name)
		if err != nil {
			return nil, err
		}

		found[id] = p
	}

	return found, nil
}

func (u *units) stop(ctx context.Context, id string) error {
	bus, err := u.bus()
	if err != nil {
		return err
	}

	name := unitName(id)
	result := make(chan string, 1)

	_, err = bus.StopUnitContext(ctx, name, "replace", result)
	if errorName(err) == errNoSuchUnit {
		return nil
	}

	if err != nil {
		return fmt.Errorf("the machine's unit %s could not be stopped: %w", name, err)
	}

	// stopping takes as long as the firecracker takes to go once it is told
	// to, and as long again once it is killed.
	if err := await(ctx, result, 2*stopTimeout); err != nil {
		return fmt.Errorf("the machine's unit %s did not stop: %w", name, err)
	}

	// a unit whose firecracker failed is let go of once it is reset, which
	// systemd does on its own for these units too; asking again is harmless.
	_ = bus.ResetFailedUnitContext(ctx, name)

	return nil
}

func (u *units) close() error {
	u.lock.Lock()
	defer u.lock.Unlock()

	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
	}

	return nil
}

// process is a machine's firecracker as its unit says: its main process, while
// it has one, and its cgroup.
func (u *units) process(ctx context.Context, bus *systemd.Conn, name string) (process, error) {
	properties, err := bus.GetUnitTypePropertiesContext(ctx, name, "Service")
	if err != nil {
		return process{}, fmt.Errorf("the host's systemd did not say how the machine's unit %s is: %w", name, err)
	}

	pid, _ := properties["MainPID"].(uint32)
	if pid == 0 {
		return process{}, nil
	}

	controlGroup, _ := properties["ControlGroup"].(string)

	return process{running: true, pid: int(pid), unit: name, cgroup: unitCgroup(controlGroup)}, nil
}

// release waits for a unit that ended to be let go of, which systemd does on
// its own; one that failed is reset first, which is what lets it go.
func (u *units) release(ctx context.Context, bus *systemd.Conn, name string) error {
	_ = bus.ResetFailedUnitContext(ctx, name)

	deadline := time.Now().Add(stopTimeout)

	for {
		listed, err := bus.ListUnitsByNamesContext(ctx, []string{name})
		if err != nil {
			return err
		}

		if len(listed) == 0 || listed[0].LoadState != "loaded" {
			return nil
		}

		if state := listed[0].ActiveState; state != "inactive" && state != "failed" {
			return fmt.Errorf("%w: the machine's unit %s is %s", vm.ErrConflict, name, state)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("the machine's unit %s ended, and was not let go of within %s", name, stopTimeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval * 5):
		}
	}
}

// await waits for a job systemd was given to end, and says how it went.
func await(ctx context.Context, result <-chan string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case outcome := <-result:
		if outcome != "done" {
			return fmt.Errorf("systemd says it %s", outcome)
		}

		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return fmt.Errorf("systemd did not say it was done within %s", timeout)
	}
}

// errorName is the name of the D-Bus error err is, if it is one.
func errorName(err error) string {
	var value dbus.Error
	if errors.As(err, &value) {
		return value.Name
	}

	var pointer *dbus.Error
	if errors.As(err, &pointer) && pointer != nil {
		return pointer.Name
	}

	return ""
}
