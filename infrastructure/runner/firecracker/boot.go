//go:build linux

package firecracker

import (
	"context"
	"fmt"
	"io"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/sirupsen/logrus"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

const (
	// guestCID is the machine's own end of its vsock. Every machine has the
	// same one: the host reaches each through a socket of its own, so the
	// number never has to tell one machine from another.
	guestCID = 3

	// bootArgs is the kernel's command line. The agent is /init in the
	// initramfs and takes the one argument after the dashes, which names the
	// command it runs as; the root it boots the task in is put together from
	// the disks once the machine is told what it is. A machine asked to reboot
	// ends instead, which is what turning one off is.
	bootArgs = "console=ttyS0 reboot=k panic=1 pci=off quiet loglevel=3 rdinit=/init -- serve-runner-guest"
)

// boot tells a machine's firecracker what the machine is, and starts it. The
// paths it is told are the ones the launcher linked into the machine's own
// directory, which is where firecracker finds them.
func (r *Runtime) boot(ctx context.Context, launched machine.Machine, rec record) error {
	quiet := logrus.New()
	quiet.Out = io.Discard

	client := firecracker.NewClient(launched.Socket, logrus.NewEntry(quiet), false)

	vcpus, memory, smt := int64(rec.VCPUs), int64(rec.MemoryMiB), false
	if _, err := client.PutMachineConfiguration(ctx, &models.MachineConfiguration{VcpuCount: &vcpus, MemSizeMib: &memory, Smt: &smt}); err != nil {
		return fmt.Errorf("the machine's size was refused: %w", err)
	}

	kernel := launched.Files.Kernel
	if _, err := client.PutGuestBootSource(ctx, &models.BootSource{KernelImagePath: &kernel, InitrdPath: launched.Files.Initrd, BootArgs: bootArgs}); err != nil {
		return fmt.Errorf("the machine's kernel was refused: %w", err)
	}

	for i, drive := range launched.Files.Drives {
		id := fmt.Sprintf("drive%d", i)
		path := drive

		// the image is shared by every machine that runs it, and only ever
		// read; the scratch disk is the machine's own.
		readOnly, root := i == 0, false

		if _, err := client.PutGuestDriveByID(ctx, id, &models.Drive{DriveID: &id, PathOnHost: &path, IsReadOnly: &readOnly, IsRootDevice: &root}); err != nil {
			return fmt.Errorf("the machine's disk %d was refused: %w", i, err)
		}
	}

	for i, tap := range launched.Taps {
		id := fmt.Sprintf("eth%d", i)
		device := tap.Device

		if _, err := client.PutGuestNetworkInterfaceByID(ctx, id, &models.NetworkInterface{IfaceID: &id, HostDevName: &device, GuestMac: rec.Interfaces[i].MAC}); err != nil {
			return fmt.Errorf("the machine's network device %d was refused: %w", i, err)
		}
	}

	cid, socket := int64(guestCID), vsockName
	if _, err := client.PutGuestVsock(ctx, &models.Vsock{GuestCid: &cid, UdsPath: &socket}); err != nil {
		return fmt.Errorf("the machine's vsock was refused: %w", err)
	}

	action := models.InstanceActionInfoActionTypeInstanceStart
	if _, err := client.CreateSyncAction(ctx, &models.InstanceActionInfo{ActionType: &action}); err != nil {
		return fmt.Errorf("the machine did not start: %w", err)
	}

	return nil
}
