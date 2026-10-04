package api

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/dialVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/endExec"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/execVM"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// A command's terminal, and a connection to one of a task's ports, are
// streams: the connection the request came on is switched to the agent's own
// protocol (vm.UpgradeExec, vm.UpgradeDial) once the agent has agreed, and
// from then on vmhost only carries bytes between it and the agent's
// connection. What travels is the agent's to say — guest frames, or the task's
// own bytes — and vmhost reads none of it.

type execHandler struct {
	useCase *execVM.UseCase
}

func NewExecHandler(useCase *execVM.UseCase) *execHandler {
	return &execHandler{useCase: useCase}
}

// ServeHTTP takes a guest.Exec, starts the command inside the VM, and switches
// the connection to vm.UpgradeExec, saying what the agent calls the command in
// vm.ExecIDHeader.
func (h *execHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if !upgradeAsked(rw, r, vm.UpgradeExec) {
		return
	}

	var exec guest.Exec
	if !decode(rw, r, &exec) {
		return
	}

	response, err := h.useCase.Execute(r.Context(), &execVM.Request{ID: r.PathValue("id"), Exec: exec})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		carry(rw, r, vm.UpgradeExec, http.Header{vm.ExecIDHeader: {response.ExecID}}, response.Conn)
	}
}

type endExecHandler struct {
	useCase *endExec.UseCase
}

func NewEndExecHandler(useCase *endExec.UseCase) *endExecHandler {
	return &endExecHandler{useCase: useCase}
}

// ServeHTTP takes a guest.EndExec and answers a guest.Ended.
func (h *endExecHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var end guest.EndExec
	if !decode(rw, r, &end) {
		return
	}

	response, err := h.useCase.Execute(r.Context(), &endExec.Request{ID: r.PathValue("id"), Exec: r.PathValue("exec"), End: end})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		reply(rw, http.StatusOK, response.Ended)
	}
}

type dialHandler struct {
	useCase *dialVM.UseCase
}

func NewDialHandler(useCase *dialVM.UseCase) *dialHandler {
	return &dialHandler{useCase: useCase}
}

// ServeHTTP connects to the task's port named by QueryPort, and switches the
// connection to vm.UpgradeDial: a raw byte stream to that port.
func (h *dialHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if !upgradeAsked(rw, r, vm.UpgradeDial) {
		return
	}

	port, err := strconv.ParseUint(r.URL.Query().Get(vm.QueryPort), 10, 16)
	if err != nil {
		failed(rw, r, fmt.Errorf("%w: %s is not a port", vm.ErrInvalid, vm.QueryPort))

		return
	}

	response, err := h.useCase.Execute(r.Context(), &dialVM.Request{ID: r.PathValue("id"), Port: uint16(port)})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		carry(rw, r, vm.UpgradeDial, nil, response.Conn)
	}
}

// upgradeAsked refuses a request that asks to switch to another protocol than
// the route's. One that asks for none is switched all the same, as docker's
// attach is.
func upgradeAsked(rw http.ResponseWriter, r *http.Request, protocol string) bool {
	asked := r.Header.Get("Upgrade")
	if len(asked) == 0 || strings.EqualFold(asked, protocol) {
		return true
	}

	failed(rw, r, fmt.Errorf("%w: this route switches to %s, not to %s", vm.ErrInvalid, protocol, asked))

	return false
}

// carry switches the connection the request came on to protocol, and carries
// bytes between it and the agent's connection until either is done.
func carry(rw http.ResponseWriter, r *http.Request, protocol string, header http.Header, agent net.Conn) {
	// what is left of the request's body is read first: after the switch
	// whatever the client sends goes to the agent as it is, and the end of
	// its request must not.
	_, _ = io.Copy(io.Discard, r.Body)

	client, buffered, err := http.NewResponseController(rw).Hijack()
	if err != nil {
		agent.Close()
		failed(rw, r, fmt.Errorf("the connection cannot be switched to %s: %w", protocol, err))

		return
	}

	// the server's deadlines were the request's: what the connection
	// carries from now on lasts as long as it lasts.
	_ = client.SetDeadline(time.Time{})

	if err := switchProtocols(buffered.Writer, protocol, header); err != nil {
		client.Close()
		agent.Close()

		return
	}

	splice(client, buffered.Reader, agent)
}

// switchProtocols says the connection now carries protocol.
func switchProtocols(w *bufio.Writer, protocol string, header http.Header) error {
	answer := http.Header{
		"Connection": {"Upgrade"},
		"Upgrade":    {protocol},
	}

	for key, values := range header {
		answer[key] = values
	}

	if _, err := io.WriteString(w, "HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		return err
	}

	if err := answer.Write(w); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "\r\n"); err != nil {
		return err
	}

	return w.Flush()
}

// splice carries bytes between the client and the agent until the guest ends
// the stream.
//
// A machine's vsock passes no half-close on: telling the agent's side that the
// client is done sending would end the whole connection inside the guest,
// whatever the task still had to answer. So a client that is done sending is
// not passed on — the agent's side stays open, and what it sends still reaches
// the client — and the stream lasts until the guest ends it, which the agent
// does when the task is done sending, or its command ended. A client that goes
// away, rather than finishing, takes the agent's side with it; one that went
// away quietly is found out the next time the guest sends it anything.
func splice(client net.Conn, fromClient io.Reader, agent net.Conn) {
	sent := make(chan struct{})

	go func() {
		defer close(sent)

		if _, err := io.Copy(agent, fromClient); err != nil {
			agent.Close()
		}
	}()

	_, _ = io.Copy(client, agent)

	client.Close()
	agent.Close()

	<-sent
}
