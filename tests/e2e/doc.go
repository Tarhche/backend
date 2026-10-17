//go:build e2e

// Package e2e holds the workload to working as a whole system: the blog, the
// control plane, an orchestrator, its vmhost with real microVMs on KVM, and the
// ingress in front of them, all running, with nothing in memory.
//
// It drives them the way the dashboard does: over the blog's HTTP API, plus the
// ingress for a VM's terminal and its ports, and the blog's websocket for the
// public code runner. It needs the whole stack up, VMs included, which on a Mac
// is Lima (CLAUDE.md, "VMs locally"), and is kept behind the e2e build tag so
// that go test ./... never reaches for it.
//
//	E2E_IDENTITY=admin E2E_PASSWORD=... go test -tags e2e -v -timeout 60m ./tests/e2e/
//
// The account it is given only has to be able to make users and roles: every
// test runs as a user of its own, made for the run with every workload
// permission and removed afterwards together with everything it made, so a run
// starts from nothing and leaves nothing, whoever else uses the stack.
//
// It reads:
//
//   - E2E_BLOG_URL, the blog's API (http://localhost:8000);
//   - E2E_INGRESS_URL, where the ingress is reached (http://localhost:8030);
//   - E2E_INGRESS_DOMAIN, the domain it serves ports under, which is sent as a
//     Host header rather than resolved (workload.localhost);
//   - E2E_IDENTITY and E2E_PASSWORD, to sign in as that account, or E2E_TOKEN,
//     an access token of it. An access token lives three minutes, far less
//     than a run, so with one the run's own account is left behind at the end:
//     signing in is what lets the run remove it.
package e2e
