package firecracker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk/client"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

// apiTimeout bounds each request to a machine's firecracker, which answers at
// once or not at all: it is a process of the same host, configuring a machine
// that has not started yet.
const apiTimeout = 5 * time.Second

// bootPlan is what a machine's firecracker is told the machine is, in the
// SDK's models. The files it names are the ones linked into the machine's own
// directory, where its firecracker runs and finds them.
type bootPlan struct {
	machine models.MachineConfiguration
	boot    models.BootSource
	drives  []models.Drive
	nics    []models.NetworkInterface
	vsock   models.Vsock
}

// planOf is what a machine's firecracker is told to make of spec.
//
// Every drive is a plain virtio disk, none of them the root: the machine boots
// into the agent on its initramfs, which puts the task's root together from
// the drives once it is told which is which. Every machine has the same vsock
// CID, since the host reaches each through a socket of its own.
func planOf(spec vm.MachineSpec) bootPlan {
	vcpus, memory, smt := int64(spec.VCPUs), int64(spec.MemoryMiB), false

	kernel := kernelName

	args := spec.KernelArgs
	if len(args) == 0 {
		args = guest.KernelArgs
	}

	plan := bootPlan{
		machine: models.MachineConfiguration{VcpuCount: &vcpus, MemSizeMib: &memory, Smt: &smt},
		boot:    models.BootSource{KernelImagePath: &kernel, BootArgs: args},
	}

	if len(spec.Initrd) > 0 {
		plan.boot.InitrdPath = initrdName
	}

	for i, drive := range spec.Drives {
		id, path, readOnly, root := driveName(i), driveName(i), drive.ReadOnly, false

		plan.drives = append(plan.drives, models.Drive{DriveID: &id, PathOnHost: &path, IsReadOnly: &readOnly, IsRootDevice: &root})
	}

	for i, nic := range spec.NICs {
		id, device := nicName(i), nic.Device

		plan.nics = append(plan.nics, models.NetworkInterface{IfaceID: &id, HostDevName: &device, GuestMac: nic.MAC})
	}

	cid, socket := int64(guest.CID), layout.VsockSocketName
	plan.vsock = models.Vsock{GuestCid: &cid, UdsPath: &socket}

	return plan
}

// configure tells a machine's firecracker, through the API socket it took,
// what the machine is, and starts the machine.
func configure(ctx context.Context, socket string, plan bootPlan) error {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
			var dialer net.Dialer

			return dialer.DialContext(ctx, "unix", socket)
		},
	}
	defer transport.CloseIdleConnections()

	runtime := httptransport.New(client.DefaultHost, client.DefaultBasePath, client.DefaultSchemes)
	runtime.Transport = transport

	api := client.New(runtime, strfmt.Default).Operations

	// each request is bounded on its own, as the SDK bounds them.
	ask := func(what string, request func(ctx context.Context) error) error {
		ctx, cancel := context.WithTimeout(ctx, apiTimeout)
		defer cancel()

		if err := request(ctx); err != nil {
			return fmt.Errorf("%s was refused: %w", what, err)
		}

		return nil
	}

	if err := ask("the machine's size", func(ctx context.Context) error {
		_, err := api.PutMachineConfiguration(operations.NewPutMachineConfigurationParamsWithContext(ctx).WithBody(&plan.machine))

		return err
	}); err != nil {
		return err
	}

	if err := ask("the machine's kernel", func(ctx context.Context) error {
		_, err := api.PutGuestBootSource(operations.NewPutGuestBootSourceParamsWithContext(ctx).WithBody(&plan.boot))

		return err
	}); err != nil {
		return err
	}

	for i := range plan.drives {
		drive := &plan.drives[i]

		if err := ask(fmt.Sprintf("the machine's disk %d", i), func(ctx context.Context) error {
			_, err := api.PutGuestDriveByID(operations.NewPutGuestDriveByIDParamsWithContext(ctx).WithDriveID(*drive.DriveID).WithBody(drive))

			return err
		}); err != nil {
			return err
		}
	}

	for i := range plan.nics {
		nic := &plan.nics[i]

		if err := ask(fmt.Sprintf("the machine's network device %d", i), func(ctx context.Context) error {
			_, err := api.PutGuestNetworkInterfaceByID(operations.NewPutGuestNetworkInterfaceByIDParamsWithContext(ctx).WithIfaceID(*nic.IfaceID).WithBody(nic))

			return err
		}); err != nil {
			return err
		}
	}

	if err := ask("the machine's vsock", func(ctx context.Context) error {
		_, err := api.PutGuestVsock(operations.NewPutGuestVsockParamsWithContext(ctx).WithBody(&plan.vsock))

		return err
	}); err != nil {
		return err
	}

	start := models.InstanceActionInfoActionTypeInstanceStart

	return ask("starting the machine", func(ctx context.Context) error {
		_, err := api.CreateSyncAction(operations.NewCreateSyncActionParamsWithContext(ctx).WithInfo(&models.InstanceActionInfo{ActionType: &start}))

		return err
	})
}
