//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"text/tabwriter"
	"time"
)

const (
	// renewAfter is how old an access token is let get before another is
	// asked for. The blog's live three minutes, and a run takes far longer.
	renewAfter = 2 * time.Minute

	// requestTimeout bounds one request. Creating a container, which may make
	// and boot a Docker VM and pull an image before it answers, is the longest.
	requestTimeout = 15 * time.Minute

	// poll is how often something being waited for is looked at again.
	poll = 2 * time.Second
)

// environment is where the stack is reached.
type environment struct {
	blogURL       string
	ingressURL    string
	ingressDomain string
}

var (
	env environment

	// user is who every test runs as: an account made for the run, with
	// every workload permission and nothing of its own to begin with.
	user *session

	// admin is the account the run was given, which owns none of what the
	// run makes.
	admin *session

	// timings are how long each step took, said at the end of the run.
	timings recorder
)

func TestMain(m *testing.M) {
	env = environment{
		blogURL:       setting("E2E_BLOG_URL", "http://localhost:8000"),
		ingressURL:    setting("E2E_INGRESS_URL", "http://localhost:8030"),
		ingressDomain: setting("E2E_INGRESS_DOMAIN", "workload.localhost"),
	}

	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	var err error

	admin, err = adminSession(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)

		return 1
	}

	account, err := newAccount(ctx, admin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: making the run's account:", err)

		// what was made of it before it failed goes with it.
		if account != nil {
			_ = account.remove(ctx, admin)
		}

		return 1
	}

	user = account.session

	code := m.Run()

	// everything the run made goes with the account it made it as, whatever
	// the tests left behind when they failed halfway.
	if err := account.remove(ctx, admin); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: removing what the run made:", err)

		code = max(code, 1)
	}

	timings.print(os.Stdout)

	return code
}

// setting is an environment variable, or what it is when unset.
func setting(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return strings.TrimRight(value, "/")
	}

	return fallback
}

// adminSession is the account the run is given, which makes and removes the
// run's own.
func adminSession(ctx context.Context) (*session, error) {
	identity, password := os.Getenv("E2E_IDENTITY"), os.Getenv("E2E_PASSWORD")

	switch {
	case len(identity) > 0 && len(password) > 0:
		return signIn(ctx, identity, password)
	case len(os.Getenv("E2E_TOKEN")) > 0:
		return &session{access: os.Getenv("E2E_TOKEN"), issued: time.Now()}, nil
	default:
		return nil, errors.New("set E2E_TOKEN, or E2E_IDENTITY and E2E_PASSWORD, to an account that may make users and roles")
	}
}

// account is the user a run is made under, and the role that gives it every
// workload permission.
type account struct {
	session  *session
	userUUID string
	roleUUID string
}

// newAccount makes a user of the run's own and gives it every workload
// permission there is, admin and self alike, as the dashboard's permissions
// list says them.
func newAccount(ctx context.Context, admin *session) (*account, error) {
	suffix := random(4)
	username := "e2e-" + suffix
	password := "E2e-" + random(12)

	var permissions struct {
		Items []struct {
			Value string `json:"value"`
		} `json:"items"`
	}
	if err := admin.do(ctx, http.MethodGet, "/api/dashboard/permissions", nil, http.StatusOK, &permissions); err != nil {
		return nil, err
	}

	var workload []string
	for _, p := range permissions.Items {
		if strings.Contains(p.Value, "workload.") {
			workload = append(workload, p.Value)
		}
	}

	if len(workload) == 0 {
		return nil, errors.New("the dashboard lists no workload permissions")
	}

	var created struct {
		UUID string `json:"uuid"`
	}
	if err := admin.do(ctx, http.MethodPost, "/api/dashboard/users", map[string]any{
		"name":          "e2e " + suffix,
		"email":         username + "@workload.test",
		"username":      username,
		"password":      password,
		"language_code": "en",
	}, http.StatusCreated, &created); err != nil {
		return nil, err
	}

	a := &account{userUUID: created.UUID}

	var role struct {
		UUID string `json:"uuid"`
	}
	if err := admin.do(ctx, http.MethodPost, "/api/dashboard/roles", map[string]any{
		"name":        username,
		"description": "every workload permission, for one end-to-end run",
		"permissions": workload,
		"user_uuids":  []string{created.UUID},
	}, http.StatusCreated, &role); err != nil {
		return a, err
	}

	a.roleUUID = role.UUID

	s, err := signIn(ctx, username, password)
	if err != nil {
		return a, err
	}

	a.session = s

	return a, nil
}

// remove takes away everything the account made, and then the account: its
// VMs, which take their stacks with them, its snapshots, and the user and
// role themselves.
func (a *account) remove(ctx context.Context, admin *session) error {
	var errs []error

	if a.session != nil {
		errs = append(errs, a.removeVMs(ctx), a.removeSnapshots(ctx))
	}

	if len(a.roleUUID) > 0 {
		errs = append(errs, admin.do(ctx, http.MethodDelete, "/api/dashboard/roles/"+a.roleUUID, nil, http.StatusNoContent, nil))
	}

	if len(a.userUUID) > 0 {
		errs = append(errs, admin.do(ctx, http.MethodDelete, "/api/dashboard/users/"+a.userUUID, nil, http.StatusNoContent, nil))
	}

	return errors.Join(errs...)
}

func (a *account) removeVMs(ctx context.Context) error {
	var vms page[vmView]
	if err := a.session.do(ctx, http.MethodGet, "/api/dashboard/my/workload/vms", nil, http.StatusOK, &vms); err != nil {
		return err
	}

	for _, v := range vms.Items {
		status, body, err := a.session.request(ctx, http.MethodDelete, "/api/dashboard/my/workload/vms/"+v.UUID, nil)
		if err != nil {
			return err
		}

		// a VM is deleted in its own time: asking is accepted, and it goes
		// once its node has let it go.
		if status != http.StatusAccepted && status != http.StatusNotFound {
			return fmt.Errorf("deleting vm %s: %d %s", v.UUID, status, body)
		}
	}

	deadline := time.Now().Add(5 * time.Minute)
	for {
		if err := a.session.do(ctx, http.MethodGet, "/api/dashboard/my/workload/vms", nil, http.StatusOK, &vms); err != nil {
			return err
		}

		if len(vms.Items) == 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%d vms are still there", len(vms.Items))
		}

		time.Sleep(poll)
	}
}

func (a *account) removeSnapshots(ctx context.Context) error {
	var snapshots page[snapshotView]
	if err := a.session.do(ctx, http.MethodGet, "/api/dashboard/my/workload/snapshots", nil, http.StatusOK, &snapshots); err != nil {
		return err
	}

	for _, s := range snapshots.Items {
		status, body, err := a.session.request(ctx, http.MethodDelete, "/api/dashboard/my/workload/snapshots/"+s.UUID, nil)
		if err != nil {
			return err
		}

		if status != http.StatusNoContent && status != http.StatusNotFound {
			return fmt.Errorf("deleting snapshot %s: %d %s", s.UUID, status, body)
		}
	}

	return nil
}

// random is n random bytes, as hex.
func random(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

// session is one account signed in to the blog. Its access token is renewed
// as it ages, with its refresh token, so a run outlives the tokens it starts
// with.
type session struct {
	mu      sync.Mutex
	access  string
	refresh string
	issued  time.Time
}

type tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func signIn(ctx context.Context, identity, password string) (*session, error) {
	var t tokens
	if err := (&session{}).send(ctx, http.MethodPost, "/api/auth/login", "", map[string]string{
		"identity": identity,
		"password": password,
	}, http.StatusOK, &t); err != nil {
		return nil, fmt.Errorf("signing in as %s: %w", identity, err)
	}

	return &session{access: t.AccessToken, refresh: t.RefreshToken, issued: time.Now()}, nil
}

// token is an access token that has some life left in it, when the session
// can renew one.
func (s *session) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if time.Since(s.issued) < renewAfter || len(s.refresh) == 0 {
		return s.access, nil
	}

	var t tokens
	if err := (&session{}).send(ctx, http.MethodPost, "/api/auth/token/refresh", "", map[string]string{
		"token": s.refresh,
	}, http.StatusOK, &t); err != nil {
		return "", fmt.Errorf("renewing an access token: %w", err)
	}

	s.access, s.refresh, s.issued = t.AccessToken, t.RefreshToken, time.Now()

	return s.access, nil
}

// do makes one request to the blog and decodes its answer into out, unless
// the answer is not the status wanted.
func (s *session) do(ctx context.Context, method, path string, body any, want int, out any) error {
	token, err := s.token(ctx)
	if err != nil {
		return err
	}

	return s.send(ctx, method, path, token, body, want, out)
}

func (s *session) send(ctx context.Context, method, path, token string, body any, want int, out any) error {
	status, answer, err := s.requestWith(ctx, method, path, token, "", body)
	if err != nil {
		return err
	}

	if status != want {
		return fmt.Errorf("%s %s: %d, wanted %d: %s", method, path, status, want, answer)
	}

	if out == nil || len(answer) == 0 {
		return nil
	}

	if err := json.Unmarshal(answer, out); err != nil {
		return fmt.Errorf("%s %s: %w: %s", method, path, err, answer)
	}

	return nil
}

// request makes one request to the blog and answers with what came back,
// whatever its status.
func (s *session) request(ctx context.Context, method, path string, body any) (int, []byte, error) {
	token, err := s.token(ctx)
	if err != nil {
		return 0, nil, err
	}

	return s.requestWith(ctx, method, path, token, "", body)
}

func (s *session) requestWith(ctx context.Context, method, path, token, language string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}

		reader = bytes.NewReader(encoded)
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	r, err := http.NewRequestWithContext(ctx, method, env.blogURL+path, reader)
	if err != nil {
		return 0, nil, err
	}

	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}

	if len(token) > 0 {
		r.Header.Set("Authorization", "Bearer "+token)
	}

	if len(language) > 0 {
		r.Header.Set("X-Language-Code", language)
	}

	response, err := http.DefaultClient.Do(r)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()

	answer, err := io.ReadAll(response.Body)

	return response.StatusCode, answer, err
}

// call is do for a test: what goes wrong ends it.
func (s *session) call(t testing.TB, method, path string, body any, want int, out any) {
	t.Helper()

	if err := s.do(t.Context(), method, path, body, want, out); err != nil {
		t.Fatal(err)
	}
}

// status is what the blog answers a request with, and what it said.
func (s *session) status(t testing.TB, method, path string, body any) (int, []byte) {
	t.Helper()

	status, answer, err := s.request(t.Context(), method, path, body)
	if err != nil {
		t.Fatal(err)
	}

	return status, answer
}

// eventually waits for check to say it is done, saying each new state it
// reports on the way, and ends the test when it is not done in time.
func eventually(t testing.TB, what string, timeout time.Duration, check func() (done bool, state string)) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	last := ""

	for {
		done, state := check()
		if state != last {
			t.Logf("%s: %s", what, state)
			last = state
		}

		if done {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("%s: still %s after %s", what, state, timeout)
		}

		time.Sleep(poll)
	}
}

// recorder keeps how long each step of each test took.
type recorder struct {
	mu    sync.Mutex
	steps []timing
}

type timing struct {
	test string
	step string
	took time.Duration
}

// step runs one step of a test and keeps how long it took.
func (r *recorder) step(t *testing.T, name string, f func(t *testing.T)) {
	t.Helper()

	start := time.Now()
	defer func() {
		took := time.Since(start)

		r.mu.Lock()
		r.steps = append(r.steps, timing{test: t.Name(), step: name, took: took})
		r.mu.Unlock()

		t.Logf("%s: %s", name, took.Round(100*time.Millisecond))
	}()

	f(t)
}

func (r *recorder) print(w io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.steps) == 0 {
		return
	}

	totals := make(map[string]time.Duration)
	var tests []string

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "\nTEST\tSTEP\tTOOK")

	for _, s := range r.steps {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", s.test, s.step, s.took.Round(100*time.Millisecond))

		if _, ok := totals[s.test]; !ok {
			tests = append(tests, s.test)
		}

		totals[s.test] += s.took
	}

	slices.Sort(tests)
	for _, test := range tests {
		fmt.Fprintf(tw, "%s\ttotal\t%s\n", test, totals[test].Round(100*time.Millisecond))
	}

	_ = tw.Flush()
}

// page is one page of a listing.
type page[T any] struct {
	Items []T `json:"items"`
}

type resources struct {
	CPUs   uint   `json:"cpus"`
	Memory uint64 `json:"memory"`
	Disk   uint64 `json:"disk"`
}

type network struct {
	Ingress string `json:"ingress"`
	Egress  string `json:"egress"`
}

type vmView struct {
	UUID           string    `json:"uuid"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	OwnerUUID      string    `json:"owner_uuid"`
	Kind           string    `json:"kind"`
	Image          string    `json:"image"`
	Resources      resources `json:"resources"`
	Ports          []uint    `json:"ports"`
	Network        network   `json:"network"`
	PersistentDisk bool      `json:"persistent_disk"`
	State          string    `json:"state"`
	ExpectedState  string    `json:"expected_state"`
	Reason         string    `json:"reason"`
	NodeName       string    `json:"node_name"`
	Stats          *struct {
		CPUPercent  float64   `json:"cpu_percent"`
		MemoryUsed  uint64    `json:"memory_used"`
		MemoryLimit uint64    `json:"memory_limit"`
		DiskUsed    uint64    `json:"disk_used"`
		DiskTotal   uint64    `json:"disk_total"`
		NetworkRx   uint64    `json:"network_rx"`
		NetworkTx   uint64    `json:"network_tx"`
		SampledAt   time.Time `json:"sampled_at"`
	} `json:"stats"`
	URLs []struct {
		Port uint   `json:"port"`
		URL  string `json:"url"`
	} `json:"urls"`
}

func (v vmView) String() string {
	if len(v.Reason) > 0 {
		return v.State + " (" + v.Reason + ")"
	}

	return v.State
}

type snapshotView struct {
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	VMUUID string `json:"vm_uuid"`
	Kind   string `json:"kind"`
	Image  string `json:"image"`
	Disk   uint64 `json:"disk"`
	Engine string `json:"engine"`
	Size   int64  `json:"size"`
	State  string `json:"state"`
	Reason string `json:"reason"`
}

// vmPath is where one of the run's VMs is, on the self routes.
func vmPath(uuid string, rest ...string) string {
	return "/api/dashboard/my/workload/vms/" + url.PathEscape(uuid) + strings.Join(rest, "")
}

// getVM reads one of the run's VMs.
func getVM(t testing.TB, uuid string) vmView {
	t.Helper()

	var v vmView
	user.call(t, http.MethodGet, vmPath(uuid), nil, http.StatusOK, &v)

	return v
}

// waitForVM waits until one of the run's VMs is in state, and ends the test
// when it fails on the way, since nothing more is coming.
func waitForVM(t testing.TB, uuid, state string, timeout time.Duration) vmView {
	t.Helper()

	var v vmView
	eventually(t, "vm "+uuid, timeout, func() (bool, string) {
		v = getVM(t, uuid)
		if v.State == "failed" && state != "failed" {
			t.Fatalf("vm %s failed: %s", uuid, v.Reason)
		}

		return v.State == state, v.String()
	})

	return v
}

// waitForVMGone waits until one of the run's VMs is no longer there at all.
func waitForVMGone(t testing.TB, uuid string, timeout time.Duration) {
	t.Helper()

	eventually(t, "vm "+uuid, timeout, func() (bool, string) {
		status, body := user.status(t, http.MethodGet, vmPath(uuid), nil)
		if status == http.StatusNotFound {
			return true, "gone"
		}

		var v vmView
		_ = json.Unmarshal(body, &v)

		return false, v.String()
	})
}

// ingressGet asks the ingress for path on host, which names a VM's port as
// <slug>-<port>.<domain>. The name is sent rather than resolved, as a
// browser that resolved it would send it.
func ingressGet(ctx context.Context, host, path string) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, env.ingressURL+path, nil)
	if err != nil {
		return 0, "", err
	}

	r.Host = host

	response, err := http.DefaultClient.Do(r)
	if err != nil {
		return 0, "", err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)

	return response.StatusCode, string(body), err
}

// portHost is the name a VM's port is served under.
func portHost(slug string, port uint) string {
	return fmt.Sprintf("%s-%d.%s", slug, port, env.ingressDomain)
}

// waitForPort waits until the ingress serves a VM's port with an answer that
// holds want.
func waitForPort(t testing.TB, slug string, port uint, want string, timeout time.Duration) string {
	t.Helper()

	host := portHost(slug, port)

	var body string
	eventually(t, host, timeout, func() (bool, string) {
		status, answer, err := ingressGet(t.Context(), host, "/")
		if err != nil {
			return false, err.Error()
		}

		body = answer

		return status == http.StatusOK && strings.Contains(answer, want), fmt.Sprintf("%d %s", status, firstLine(answer))
	})

	return body
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if len(line) > 120 {
		line = line[:120] + "…"
	}

	return line
}
