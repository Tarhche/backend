// Command probe speaks the two websockets tests/e2e/microvm.sh cannot speak
// itself: a terminal opened inside a task, which the ingress carries to the node
// holding it, and the code runner's request and the reply it waits for, on the
// blog's /api/ws.
//
//	probe attach <ws url> <input> <expected>...
//	probe run-code <ws url> <runner> <code> <expected>
//
// attach resizes the terminal to 40 rows and 120 columns, types input into it,
// and waits for every expected text to appear in what comes back. run-code asks
// for code to be run by runner, waits for the final reply, and checks the
// output holds expected; it prints the task that ran it as "task: <uuid>".
//
// Either exits 0 once what it waits for has arrived, and 1 with what it saw
// otherwise. PROBE_TIMEOUT bounds the wait (a Go duration, 2m by default), and
// PROBE_TOKEN, when set, is sent as the bearer token a node checks a terminal's
// owner against.
//
// It is a program rather than a test because the e2e script drives it against a
// stack that is already running, step by step, and reads its exit status.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultTimeout = 2 * time.Minute

	// the terminal's size after the resize; a shell asked `stty size` answers
	// "40 120", which is how the e2e script sees that the resize arrived.
	terminalRows = 40
	terminalCols = 120

	// runCodeSubject is what the blog's websocket gateway carries a request to
	// the code runner under (application/code/runCode).
	runCodeSubject = "runCode"

	// replyChunk is a reply that is one piece of a longer answer
	// (domain.ReplyChunk); anything else ends the request.
	replyChunk = 1
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	timeout := defaultTimeout
	if value := os.Getenv("PROBE_TIMEOUT"); len(value) > 0 {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("PROBE_TIMEOUT: %w", err)
		}

		timeout = parsed
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch {
	case len(args) >= 4 && args[0] == "attach":
		return attach(ctx, args[1], args[2], args[3:])
	case len(args) == 5 && args[0] == "run-code":
		return runCode(ctx, args[1], args[2], args[3], args[4])
	}

	return errors.New("usage: probe attach <ws url> <input> <expected>... | probe run-code <ws url> <runner> <code> <expected>")
}

// dial opens a websocket, with the bearer token when there is one. A refused
// upgrade says what the server answered, which is the useful part of a 404 or
// a 401.
func dial(ctx context.Context, url string) (*websocket.Conn, error) {
	header := http.Header{}
	if token := os.Getenv("PROBE_TOKEN"); len(token) > 0 {
		header.Set("Authorization", "Bearer "+token)
	}

	conn, response, err := websocket.DefaultDialer.DialContext(ctx, url, header)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("%s: %w (%s)", url, err, response.Status)
		}

		return nil, fmt.Errorf("%s: %w", url, err)
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	}

	return conn, nil
}

// attach types into a terminal inside a task and waits for what it should
// print. Binary frames are the terminal's bytes, both ways; a text frame is a
// control message, which is how a terminal is resized.
func attach(ctx context.Context, url string, input string, expected []string) error {
	conn, err := dial(ctx, url)
	if err != nil {
		return err
	}
	defer conn.Close()

	resize, err := json.Marshal(map[string]any{"type": "resize", "rows": terminalRows, "cols": terminalCols})
	if err != nil {
		return err
	}

	if err := conn.WriteMessage(websocket.TextMessage, resize); err != nil {
		return fmt.Errorf("resizing the terminal: %w", err)
	}

	// a moment for the resize to reach the terminal before anything asks it
	// for its size: the two travel separately, through vmhost for a microVM.
	time.Sleep(500 * time.Millisecond)

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte(input)); err != nil {
		return fmt.Errorf("typing into the terminal: %w", err)
	}

	var output strings.Builder
	for !containsAll(output.String(), expected) {
		kind, payload, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("the terminal printed %q and then %w, before %q", output.String(), err, expected)
		}

		if kind == websocket.BinaryMessage {
			output.Write(payload)
		}
	}

	fmt.Printf("%s\n", output.String())

	return nil
}

// request is what the blog's websocket takes, and reply what it answers: the
// payload is JSON of its own, carried as bytes.
type request struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Payload []byte `json:"payload"`
}

type reply struct {
	RequestID string `json:"request_id"`
	Kind      uint8  `json:"kind"`
	Payload   []byte `json:"payload"`
}

// outcome is what the code runner answers: the run's output and state, or why
// it was not run.
type outcome struct {
	TaskUUID string            `json:"task_uuid"`
	Logs     []byte            `json:"logs"`
	State    string            `json:"state"`
	Error    string            `json:"error"`
	Errors   map[string]string `json:"errors"`
}

// runCode asks the code runner to run code, as the playground does, and checks
// what it printed.
func runCode(ctx context.Context, url string, runner string, code string, expected string) error {
	conn, err := dial(ctx, url)
	if err != nil {
		return err
	}
	defer conn.Close()

	id, err := randomID()
	if err != nil {
		return err
	}

	// the gateway gives the request an id of its own on the way, and answers
	// under this one
	payload, err := json.Marshal(map[string]string{"id": id, "code": code, "runner": runner})
	if err != nil {
		return err
	}

	if err := conn.WriteJSON(request{ID: id, Subject: runCodeSubject, Payload: payload}); err != nil {
		return fmt.Errorf("asking for the code to be run: %w", err)
	}

	for {
		var answer reply
		if err := conn.ReadJSON(&answer); err != nil {
			return fmt.Errorf("waiting for the code runner: %w", err)
		}

		if answer.RequestID != id || answer.Kind == replyChunk {
			continue
		}

		var result outcome
		if err := json.Unmarshal(answer.Payload, &result); err != nil {
			return fmt.Errorf("reading the code runner's answer %q: %w", answer.Payload, err)
		}

		fmt.Printf("task: %s\nstate: %s\noutput: %s\n", result.TaskUUID, result.State, result.Logs)

		switch {
		case len(result.Errors) > 0:
			return fmt.Errorf("the code was not run: %v", result.Errors)
		case len(result.Error) > 0:
			return fmt.Errorf("the code was not run: %s", result.Error)
		case !strings.Contains(string(result.Logs), expected):
			return fmt.Errorf("the output does not hold %q", expected)
		}

		return nil
	}
}

func containsAll(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}

	return true
}

func randomID() (string, error) {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}

	return "e2e-" + hex.EncodeToString(buffer), nil
}
