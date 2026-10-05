//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// runAnswer is what the code runner answers a snippet that only prints with,
// once its task has ended: what it printed, and how its task ended, which is
// what its process's exit code decided — completed for 0, failed for any
// other it chose itself.
type runAnswer struct {
	TaskUUID string `json:"task_uuid"`
	Logs     []byte `json:"logs"`
	State    string `json:"state"`
	Error    string `json:"error"`
}

// runSnippet runs code the way a reader of an article does: over the blog's
// websocket, signed in as nobody, and waits for the one answer it gets.
func runSnippet(t testing.TB, runner, code string) runAnswer {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	address := "ws" + strings.TrimPrefix(env.blogURL, "http") + "/api/ws"

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, address, nil)
	if err != nil {
		t.Fatalf("opening the code runner's websocket: %v", err)
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	payload, err := json.Marshal(map[string]string{"runner": runner, "code": code})
	if err != nil {
		t.Fatal(err)
	}

	id := "e2e-" + random(4)
	if err := conn.WriteJSON(map[string]any{"id": id, "subject": "runCode", "payload": payload}); err != nil {
		t.Fatalf("asking for the snippet to be run: %v", err)
	}

	for {
		var reply struct {
			RequestID string `json:"request_id"`
			Payload   []byte `json:"payload"`
		}
		if err := conn.ReadJSON(&reply); err != nil {
			t.Fatalf("waiting for the snippet's answer: %v", err)
		}

		if reply.RequestID != id {
			continue
		}

		var answer runAnswer
		if err := json.Unmarshal(reply.Payload, &answer); err != nil {
			t.Fatalf("the code runner answered %q: %v", reply.Payload, err)
		}

		return answer
	}
}

// TestCodeRunner runs snippets through the public code runner, each in an
// ephemeral VM of its own: one that prints and ends, a Go one that is built
// before it does, one that prints and runs past its time, whose runner exits
// with 124 and is a snippet that failed, and two that serve a port through the
// ingress, one until it is stopped and one until it ends.
func TestCodeRunner(t *testing.T) {
	// the code runner keeps the answers of snippets it has run, so each run
	// asks for code it has not seen.
	token := random(6)
	runner := "nodejs-22.14"

	timings.step(t, "a snippet exiting with 0", func(t *testing.T) {
		answer := runSnippet(t, runner, fmt.Sprintf(`console.log("hello from e2e %s");
`, token))

		output := string(answer.Logs)
		if len(answer.Error) > 0 || answer.State != "completed" || !strings.Contains(output, "hello from e2e "+token) {
			t.Fatalf("the snippet is %s (%s), printing %q", answer.State, answer.Error, output)
		}
	})

	// a Go snippet is built before it runs, from a standard library its image
	// keeps no build of, so it is the one that needs the most of its VM, and of
	// its 30 s. The runner's script exits with 0 when the build fails, so only
	// what the snippet printed says it was built: a build killed for want of
	// memory or disk prints what killed it instead.
	timings.step(t, "a Go snippet, built and run", func(t *testing.T) {
		answer := runSnippet(t, "go-1.24", fmt.Sprintf(`package main

import (
	"fmt"
	"strings"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	words := make([]string, 3)
	for i := range words {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			words[i] = strings.Repeat("go", i+1)
		}(i)
	}
	wg.Wait()
	fmt.Println("hello from e2e %s", strings.Join(words, " "))
}
`, token))

		output := string(answer.Logs)
		if len(answer.Error) > 0 || answer.State != "completed" || !strings.Contains(output, "hello from e2e "+token+" go gogo gogogo") {
			t.Fatalf("the snippet is %s (%s), printing %q", answer.State, answer.Error, output)
		}
	})

	// the runner's own script answers for the snippet's exit: it exits with
	// 124 when the snippet outlives its time, and with 0 otherwise, whatever
	// the snippet exited with. So 124 is the code that tells a snippet that
	// failed from one that did not.
	timings.step(t, "a snippet exiting with 124", func(t *testing.T) {
		answer := runSnippet(t, runner, fmt.Sprintf(`console.log("still going e2e %s");
setInterval(() => {}, 1000);
`, token))

		output := string(answer.Logs)
		if answer.State != "failed" || !strings.Contains(output, "still going e2e "+token) || !strings.Contains(output, "timed out") {
			t.Fatalf("the snippet is %s (%s), printing %q", answer.State, answer.Error, output)
		}

		t.Logf("it printed %q", output)
	})

	timings.step(t, "a snippet serving a port, stopped", func(t *testing.T) {
		servePort(t, runner, token, false)
	})

	timings.step(t, "a snippet serving a port, ending on its own", func(t *testing.T) {
		servePort(t, runner, token, true)
	})
}

// liveAnswer is one of the answers a snippet that serves a port is followed
// with, while it runs and once when it ends.
type liveAnswer struct {
	TaskUUID  string `json:"task_uuid"`
	State     string `json:"state"`
	Logs      []byte `json:"logs"`
	Error     string `json:"error"`
	Endpoints []struct {
		TaskPort uint   `json:"task_port"`
		URL      string `json:"url"`
	} `json:"endpoints"`
}

// servePort runs a snippet that serves a port the way an article's reader
// does: followed over the websocket and served through the ingress by the
// name its answers give. Then it is stopped, or, when it ends on its own, it
// is followed until its last answer says so.
func servePort(t *testing.T, runner, token string, ends bool) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(env.blogURL, "http")+"/api/ws", nil)
	if err != nil {
		t.Fatalf("opening the code runner's websocket: %v", err)
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	send := func(id, subject string, body any) {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}

		if err := conn.WriteJSON(map[string]any{"id": id, "subject": subject, "payload": payload}); err != nil {
			t.Fatalf("sending %s: %v", subject, err)
		}
	}

	// what the server says is read as it comes, whatever the test is doing
	// meanwhile, as a browser reads it: the server pings, and closes a
	// connection whose pongs stop.
	type reply struct {
		RequestID string `json:"request_id"`
		Kind      int    `json:"kind"`
		Payload   []byte `json:"payload"`
	}

	replies := make(chan reply, 64)
	failed := make(chan error, 1)

	go func() {
		for {
			var r reply
			if err := conn.ReadJSON(&r); err != nil {
				failed <- err

				return
			}

			replies <- r
		}
	}()

	// next is the next answer to request, and whether it is its last.
	next := func(request string) ([]byte, bool) {
		for {
			select {
			case r := <-replies:
				// a chunk is 1; anything else is an answer's last.
				if r.RequestID == request {
					return r.Payload, r.Kind != 1
				}
			case err := <-failed:
				t.Fatalf("following the snippet: %v", err)
			case <-ctx.Done():
				t.Fatalf("following the snippet: %v", ctx.Err())
			}
		}
	}

	code := fmt.Sprintf(`require("http").createServer((q, s) => s.end("served by e2e %s")).listen(3000, () => console.log("listening"));`, token)
	if ends {
		code += "\nsetTimeout(() => process.exit(0), 20000);"
	}

	id := "e2e-" + random(4)
	send(id, "runCode", map[string]any{
		"runner": runner,
		"code":   code,
		"ports":  []uint{3000},
	})

	var answer liveAnswer
	for answer.State != "running" || len(answer.Endpoints) == 0 {
		payload, ended := next(id)

		answer = liveAnswer{}
		if err := json.Unmarshal(payload, &answer); err != nil {
			t.Fatalf("the code runner answered %q: %v", payload, err)
		}

		if ended {
			t.Fatalf("the snippet ended before it served anything: %s (%s), printing %q", answer.State, answer.Error, answer.Logs)
		}
	}

	endpoint, err := url.Parse(answer.Endpoints[0].URL)
	if err != nil || answer.Endpoints[0].TaskPort != 3000 {
		t.Fatalf("the snippet is served at %+v", answer.Endpoints)
	}

	eventually(t, endpoint.Host, time.Minute, func() (bool, string) {
		status, body, err := ingressGet(t.Context(), endpoint.Host, "/")
		if err != nil {
			return false, err.Error()
		}

		return status == http.StatusOK && body == "served by e2e "+token, fmt.Sprintf("%d %s", status, firstLine(body))
	})

	if ends {
		for {
			payload, ended := next(id)
			if !ended {
				continue
			}

			answer = liveAnswer{}
			if err := json.Unmarshal(payload, &answer); err != nil || answer.State != "completed" || len(answer.Endpoints) > 0 {
				t.Fatalf("a snippet that ended on its own last answered %q", payload)
			}

			return
		}
	}

	// stopping a snippet is taking its task away, which is answered on its
	// own: what is left of the run is not followed any further, as the page
	// does not.
	stop := "e2e-" + random(4)
	send(stop, "codeStop", map[string]any{"task_uuid": answer.TaskUUID})

	var stopped struct {
		Errors map[string]string `json:"errors"`
	}
	if payload, _ := next(stop); json.Unmarshal(payload, &stopped) != nil || len(stopped.Errors) > 0 {
		t.Fatalf("stopping the snippet was answered %q", payload)
	}

	eventually(t, endpoint.Host, time.Minute, func() (bool, string) {
		status, body, err := ingressGet(t.Context(), endpoint.Host, "/")
		if err != nil {
			return false, err.Error()
		}

		return status != http.StatusOK, fmt.Sprintf("%d %s", status, firstLine(body))
	})
}
