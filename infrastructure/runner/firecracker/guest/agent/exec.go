//go:build linux

package agent

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

// outputDrain is how long a command's output is still read after it ended.
// Something it left running may hold its terminal open for as long as it
// likes, and that is no reason to keep the one who ran it waiting.
const outputDrain = time.Second

// execSession is a command running inside the machine alongside its task.
type execSession struct {
	cgroup string
	ended  chan struct{}
}

// exec starts a command inside the task's root, and turns the connection it
// was asked on into the one its input and output travel on.
func (a *Agent) exec(rw http.ResponseWriter, r *http.Request) {
	var request guest.Exec
	if !decode(rw, r, &request) {
		return
	}

	a.lock.Lock()
	running := a.status.State == guest.StateRunning

	// a command runs as the task does unless it says otherwise, with the
	// task's environment and what it asked for on top.
	var task guest.Process
	if a.process != nil {
		task = *a.process
	}
	a.lock.Unlock()

	if !running {
		http.Error(rw, "the task is not running", http.StatusConflict)

		return
	}

	if len(request.User) == 0 {
		request.User = task.User
	}

	if len(request.WorkingDir) == 0 {
		request.WorkingDir = task.WorkingDir
	}

	request.Env = append(append([]string(nil), task.Env...), request.Env...)
	if _, found := envValue(request.Env, "TERM"); request.TTY && !found {
		request.Env = append(request.Env, "TERM=xterm")
	}

	id, err := newID()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)

		return
	}

	command, err := a.startCommand(id, request)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)

		return
	}

	conn, buffered, err := hijack(rw, guest.UpgradeExec, map[string]string{guest.ExecIDHeader: id})
	if err != nil {
		command.abandon()

		return
	}

	a.lock.Lock()
	a.execs[id] = &execSession{cgroup: execCgroup(id), ended: command.ended}
	a.lock.Unlock()

	go command.serve(conn, buffered)
}

// command is a command that has been started, and what connects it to
// whoever started it.
type command struct {
	agent *Agent

	// terminal is the command's terminal, when it has one; otherwise input,
	// stdout and stderr are its own.
	terminal *os.File
	input    *os.File
	stdout   *os.File
	stderr   *os.File

	status <-chan unix.WaitStatus
	ended  chan struct{}

	writing sync.Mutex
}

// startCommand starts a command inside the task's root and in a cgroup of its
// own, with a terminal if it asked for one.
func (a *Agent) startCommand(id string, request guest.Exec) (*command, error) {
	if len(request.Args) == 0 {
		return nil, errors.New("there is no command to run")
	}

	who, err := resolveUser(rootDir, request.User)
	if err != nil {
		return nil, err
	}

	env := withDefaults(request.Env, who.home)
	path, _ := envValue(env, "PATH")

	executable, err := lookPath(rootDir, request.Args[0], path)
	if err != nil {
		return nil, err
	}

	cgroup, err := openCgroup(execCgroup(id))
	if err != nil {
		return nil, err
	}
	defer cgroup.Close()

	c := &command{agent: a, ended: make(chan struct{})}

	// the ends the command is given, which the agent lets go of once it has
	// them.
	var given []*os.File

	attributes := &syscall.SysProcAttr{
		Chroot:      rootDir,
		Credential:  &syscall.Credential{Uid: who.uid, Gid: who.gid, Groups: who.groups},
		Setsid:      true,
		UseCgroupFD: true,
		CgroupFD:    int(cgroup.Fd()),
	}

	if request.TTY {
		terminal, replica, err := pty.Open()
		if err != nil {
			return nil, err
		}

		if request.Rows > 0 && request.Cols > 0 {
			_ = pty.Setsize(terminal, &pty.Winsize{Rows: request.Rows, Cols: request.Cols})
		}

		c.terminal = terminal
		given = []*os.File{replica, replica, replica}

		attributes.Setctty = true
		attributes.Ctty = 0
	} else {
		inputReader, inputWriter, err := os.Pipe()
		if err != nil {
			return nil, err
		}

		stdoutReader, stdoutWriter, err := os.Pipe()
		if err != nil {
			return nil, err
		}

		stderrReader, stderrWriter, err := os.Pipe()
		if err != nil {
			return nil, err
		}

		c.input, c.stdout, c.stderr = inputWriter, stdoutReader, stderrReader
		given = []*os.File{inputReader, stdoutWriter, stderrWriter}
	}

	_, status, err := a.reaper.spawn(func() (int, error) {
		started, err := os.StartProcess(executable, request.Args, &os.ProcAttr{
			Dir:   workingDir(request.WorkingDir),
			Env:   env,
			Files: given,
			Sys:   attributes,
		})
		if err != nil {
			return 0, err
		}

		pid := started.Pid
		_ = started.Release()

		return pid, nil
	})

	for _, file := range uniqueFiles(given) {
		file.Close()
	}

	if err != nil {
		c.closeOwn()

		return nil, fmt.Errorf("failed to start %s: %w", executable, err)
	}

	c.status = status

	return c, nil
}

// serve carries the command's input and output until it ends, then says what
// it ended with and lets go of the connection.
func (c *command) serve(conn net.Conn, buffered *bufio.ReadWriter) {
	defer conn.Close()
	defer c.closeOwn()

	go c.takeInput(buffered)

	var output sync.WaitGroup

	if c.terminal != nil {
		output.Add(1)
		go func() { defer output.Done(); c.giveOutput(conn, guest.FrameStdout, c.terminal) }()
	} else {
		output.Add(2)
		go func() { defer output.Done(); c.giveOutput(conn, guest.FrameStdout, c.stdout) }()
		go func() { defer output.Done(); c.giveOutput(conn, guest.FrameStderr, c.stderr) }()
	}

	status := <-c.status
	close(c.ended)

	drained := make(chan struct{})
	go func() { output.Wait(); close(drained) }()

	select {
	case <-drained:
	case <-time.After(outputDrain):
	}

	c.writing.Lock()
	defer c.writing.Unlock()

	_ = guest.WriteFrame(conn, guest.FrameExit, guest.ExitPayload(exitCode(status)))
}

// takeInput reads frames from whoever started the command until they go.
func (c *command) takeInput(reader io.Reader) {
	for {
		frame, err := guest.ReadFrame(reader)
		if err != nil {
			return
		}

		switch frame.Type {
		case guest.FrameStdin:
			target := c.input
			if c.terminal != nil {
				target = c.terminal
			}

			if target != nil {
				_, _ = target.Write(frame.Payload)
			}
		case guest.FrameResize:
			if rows, cols, err := guest.ParseResize(frame.Payload); err == nil && c.terminal != nil {
				_ = pty.Setsize(c.terminal, &pty.Winsize{Rows: rows, Cols: cols})
			}
		case guest.FrameCloseStdin:
			if c.input != nil {
				_ = c.input.Close()
			}
		}
	}
}

// giveOutput carries one of the command's streams to whoever started it.
func (c *command) giveOutput(conn net.Conn, stream guest.FrameType, reader io.Reader) {
	buffer := make([]byte, 32<<10)

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			c.writing.Lock()
			writeErr := guest.WriteFrame(conn, stream, buffer[:n])
			c.writing.Unlock()

			if writeErr != nil {
				return
			}
		}

		if err != nil {
			return
		}
	}
}

// abandon lets go of a command whose connection could not be taken over. It
// keeps running, and is ended like any other once nobody is attached to it.
func (c *command) abandon() {
	c.closeOwn()
}

func (c *command) closeOwn() {
	for _, file := range []*os.File{c.terminal, c.input, c.stdout, c.stderr} {
		if file != nil {
			_ = file.Close()
		}
	}
}

// endExec ends a command nobody is attached to any more: it is given a moment
// to finish on its own, asked to stop, and stopped for good if it will not.
func (a *Agent) endExec(rw http.ResponseWriter, r *http.Request) {
	var end guest.EndExec
	if !decode(rw, r, &end) {
		return
	}

	id := r.PathValue("id")

	a.lock.Lock()
	session, found := a.execs[id]
	delete(a.execs, id)
	a.lock.Unlock()

	if !found {
		answer(rw, guest.Ended{})

		return
	}

	wait := func(d time.Duration) {
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			if populated, err := isPopulated(session.cgroup); err != nil || !populated {
				return
			}

			time.Sleep(emptyInterval * 10)
		}
	}

	wait(end.Grace)

	signalled, err := signalCgroup(session.cgroup, syscall.SIGTERM)
	if err != nil || !signalled {
		_ = removeCgroup(session.cgroup, emptyTimeout)
		answer(rw, guest.Ended{})

		return
	}

	wait(end.KillGrace)

	if err := removeCgroup(session.cgroup, emptyTimeout); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)

		return
	}

	answer(rw, guest.Ended{Signalled: true})
}

// hijack takes a request's connection over, telling the other end what it now
// carries.
func hijack(rw http.ResponseWriter, protocol string, headers map[string]string) (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.(http.Hijacker)
	if !ok {
		http.Error(rw, "the connection cannot be taken over", http.StatusInternalServerError)

		return nil, nil, errors.New("the connection cannot be taken over")
	}

	conn, buffered, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, err
	}

	response := "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + protocol + "\r\n"
	for name, value := range headers {
		response += name + ": " + value + "\r\n"
	}
	response += "\r\n"

	if _, err := buffered.WriteString(response); err != nil {
		conn.Close()

		return nil, nil, err
	}

	if err := buffered.Flush(); err != nil {
		conn.Close()

		return nil, nil, err
	}

	return conn, buffered, nil
}

func newID() (string, error) {
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}

	return hex.EncodeToString(id), nil
}

func uniqueFiles(files []*os.File) []*os.File {
	var unique []*os.File

	for _, file := range files {
		seen := false
		for _, kept := range unique {
			seen = seen || kept == file
		}

		if !seen {
			unique = append(unique, file)
		}
	}

	return unique
}
