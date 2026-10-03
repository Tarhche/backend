package api

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/createVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/deleteImage"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/deleteVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/dialVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/endExec"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/ensureNetwork"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/execVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getImages"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getInfo"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMLogs"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMStats"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMs"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/killVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/prepareImage"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/removeNetwork"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/restartVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/startVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/stopVM"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCases are what vmhost's routes are answered by, one each.
type UseCases struct {
	GetInfo       *getInfo.UseCase
	PrepareImage  *prepareImage.UseCase
	GetImages     *getImages.UseCase
	DeleteImage   *deleteImage.UseCase
	EnsureNetwork *ensureNetwork.UseCase
	RemoveNetwork *removeNetwork.UseCase
	CreateVM      *createVM.UseCase
	GetVMs        *getVMs.UseCase
	GetVM         *getVM.UseCase
	StartVM       *startVM.UseCase
	StopVM        *stopVM.UseCase
	RestartVM     *restartVM.UseCase
	KillVM        *killVM.UseCase
	DeleteVM      *deleteVM.UseCase
	GetVMLogs     *getVMLogs.UseCase
	GetVMStats    *getVMStats.UseCase
	ExecVM        *execVM.UseCase
	EndExec       *endExec.UseCase
	DialVM        *dialVM.UseCase
}

// NewRouter is vmhost's API: every route of domain/workload/vm/api.go, as it
// is written there, answered by its use case.
func NewRouter(useCases UseCases) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle(vm.RouteInfo, NewInfoHandler(useCases.GetInfo))

	mux.Handle(vm.RoutePrepareImage, NewPrepareImageHandler(useCases.PrepareImage))
	mux.Handle(vm.RouteImages, NewImagesHandler(useCases.GetImages))
	mux.Handle(vm.RouteDeleteImage, NewDeleteImageHandler(useCases.DeleteImage))

	mux.Handle(vm.RouteEnsureNetwork, NewEnsureNetworkHandler(useCases.EnsureNetwork))
	mux.Handle(vm.RouteRemoveNetwork, NewRemoveNetworkHandler(useCases.RemoveNetwork))

	mux.Handle(vm.RouteCreateVM, NewCreateVMHandler(useCases.CreateVM))
	mux.Handle(vm.RouteVMs, NewVMsHandler(useCases.GetVMs))
	mux.Handle(vm.RouteVM, NewVMHandler(useCases.GetVM))
	mux.Handle(vm.RouteStartVM, NewStartVMHandler(useCases.StartVM))
	mux.Handle(vm.RouteStopVM, NewStopVMHandler(useCases.StopVM))
	mux.Handle(vm.RouteRestartVM, NewRestartVMHandler(useCases.RestartVM))
	mux.Handle(vm.RouteKillVM, NewKillVMHandler(useCases.KillVM))
	mux.Handle(vm.RouteDeleteVM, NewDeleteVMHandler(useCases.DeleteVM))
	mux.Handle(vm.RouteVMLogs, NewVMLogsHandler(useCases.GetVMLogs))
	mux.Handle(vm.RouteVMStats, NewVMStatsHandler(useCases.GetVMStats))

	mux.Handle(vm.RouteExec, NewExecHandler(useCases.ExecVM))
	mux.Handle(vm.RouteEndExec, NewEndExecHandler(useCases.EndExec))
	mux.Handle(vm.RouteDial, NewDialHandler(useCases.DialVM))

	return mux
}
