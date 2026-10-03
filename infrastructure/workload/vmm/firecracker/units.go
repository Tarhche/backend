package firecracker

import (
	"math"
	"path/filepath"
	"strconv"
	"strings"

	systemd "github.com/coreos/go-systemd/v22/dbus"
	"github.com/godbus/dbus/v5"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

// A machine's unit is named after the machine, which is how it is found again:
// workload-vm-<id>.service.
const (
	unitPrefix = "workload-vm-"
	unitSuffix = ".service"

	// unitPattern matches every machine's unit, and nothing else.
	unitPattern = unitPrefix + "*" + unitSuffix
)

// cgroupRoot is where the host's cgroup hierarchy is, which a unit's own cgroup
// is under.
const cgroupRoot = "/sys/fs/cgroup"

// setpriv is the host's util-linux setpriv, which a machine's unit starts its
// firecracker through, as the machine's user.
//
// systemd cannot be told to: User= and Group= take a user or group the host's
// user database knows, and refuse the unit (217/USER) for a number it does not,
// which a machine's user is — machines' users are numbers, so that nothing is
// installed on the host for them. setpriv takes numbers as they are, gives the
// process the groups the devices it opens are open to, and lets go of every
// capability before it runs firecracker in its own place, so the unit's main
// process is the machine's firecracker as ever.
const setpriv = "/usr/bin/setpriv"

// The capabilities setpriv needs to become a machine's user and let go of
// every capability on the way, and the only ones a machine's unit is ever
// given: what runs as root in it, for as long as it does, can do nothing else.
const (
	capSetgid  = 6
	capSetuid  = 7
	capSetpcap = 8

	setprivCapabilities uint64 = 1<<capSetgid | 1<<capSetuid | 1<<capSetpcap
)

// unitName is the host unit a machine's firecracker runs as.
func unitName(id string) string {
	return unitPrefix + id + unitSuffix
}

// machineOfUnit is the machine a unit runs, if it runs one.
func machineOfUnit(unit string) (string, bool) {
	id, found := strings.CutPrefix(unit, unitPrefix)
	if !found {
		return "", false
	}

	id, found = strings.CutSuffix(id, unitSuffix)

	return id, found && vm.IsID(id)
}

// unitProperties is the transient unit a machine's firecracker runs as.
//
// It runs the firecracker linked beside the machine's directory, in that
// directory, as the machine's own user and group, with the groups the devices
// it opens are open to and no capability at all: setpriv makes it so, the
// only thing in the unit that runs as root, and only with the capabilities it
// needs for that. It is the host's process, in the slice every machine shares,
// and joins the network namespace the machines' taps are in when it starts,
// which it then holds itself.
//
// systemd holds it to the machine's size: its guest's memory and the VMM's
// overhead (MemoryMax=), and the share of its CPUs it may keep busy
// (CPUQuota=); and it accounts for everything else it uses, which is where a
// VM's stats come from. It sees the host read-only but for its own directory,
// with a /tmp of its own and no homes, and can never gain a privilege. What it
// and the guest's kernel say is appended to the machine's console log, which
// systemd opens for it, so its user cannot reach the log itself.
//
// Stopping it ends its firecracker at once, and anything left in its cgroup
// with it, since asking the guest to turn itself off is done before, through
// its agent. A unit that has ended, however it ended, is let go of at once, so
// that the machine's name can be booted again: that it ended is known from the
// machine's directory, which is left behind.
//
// A machine that runs as vmhost itself, which is for development, runs as
// root, as vmhost does.
func unitProperties(l launch, slice string, namespace string) []systemd.Property {
	command := []string{l.binary, "--api-sock", layout.APISocketName, "--id", l.id}

	if l.uid > 0 {
		groups := "--clear-groups"
		if len(l.groups) > 0 {
			numbers := make([]string, 0, len(l.groups))
			for _, group := range l.groups {
				numbers = append(numbers, strconv.FormatUint(uint64(group), 10))
			}

			groups = "--groups=" + strings.Join(numbers, ",")
		}

		user := strconv.Itoa(l.uid)

		command = append([]string{setpriv, "--reuid=" + user, "--regid=" + user, groups, "--inh-caps=-all", "--bounding-set=-all", "--"}, command...)
	}

	properties := []systemd.Property{
		systemd.PropDescription("workload vm " + l.id),
		systemd.PropType("exec"),
		systemd.PropExecStart(command, false),
		property("WorkingDirectory", l.dir),
		systemd.PropSlice(slice),
		property("CollectMode", "inactive-or-failed"),
		property("KillMode", "mixed"),
		property("TimeoutStopUSec", uint64(stopTimeout.Microseconds())),
		property("StandardOutputFileToAppend", l.console),
		property("StandardErrorFileToAppend", l.console),
		property("NoNewPrivileges", true),
		property("PrivateTmp", true),
		property("ProtectSystem", "strict"),
		property("ProtectHome", "yes"),
		property("ReadWritePaths", []string{l.dir}),
		property("UMask", uint32(0o077)),
		property("CPUAccounting", true),
		property("MemoryAccounting", true),
		property("IOAccounting", true),
		property("TasksAccounting", true),
		property("MemoryMax", l.memoryMax),
	}

	if l.cpu > 0 {
		properties = append(properties, property("CPUQuotaPerSecUSec", uint64(math.Round(l.cpu*1e6))))
	}

	if len(namespace) > 0 {
		properties = append(properties, property("NetworkNamespacePath", namespace))
	}

	if l.uid > 0 {
		properties = append(properties, property("CapabilityBoundingSet", setprivCapabilities))
	}

	return properties
}

func property(name string, value any) systemd.Property {
	return systemd.Property{Name: name, Value: dbus.MakeVariant(value)}
}

// unitCgroup is where a unit's cgroup is, given as systemd says it: relative to
// the root of the hierarchy.
func unitCgroup(controlGroup string) string {
	if len(controlGroup) == 0 {
		return ""
	}

	return filepath.Join(cgroupRoot, controlGroup)
}
