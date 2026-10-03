#!/usr/bin/env bash
#
# The end-to-end test of the workload against a stack that is running, for one
# runtime class (plan §14.3): E2E_RUNTIME, firecracker by default, or sysbox. It
# goes through the doors users go through:
#
#   - a task, asked of the control plane's API: its port fetched through the
#     ingress by the task's own hostname, a terminal opened in it over the
#     ingress's websocket and resized, its log followed as it is written, and
#     the task stopped, restarted, killed and deleted;
#   - a stack of two services, one of which reaches the other by its name;
#   - a code-runner snippet, sent to the blog's websocket as the playground
#     sends it, whose output comes back from a task of the class the blog runs
#     snippets with (E2E_CODE_RUNNER_RUNTIME, E2E_RUNTIME by default).
#
# make e2e-microvm and make e2e-sysbox run it against the local stack; the
# E2E_* variables below point it elsewhere. It needs curl, jq and Go, which
# runs tests/e2e/probe for the two websockets. What it makes it deletes again,
# whether it passes or not.
#
# A terminal is opened without a token, which a node allows for a task whose
# owner it does not know. Where it knows owners, E2E_OWNER has to be the subject
# of E2E_TOKEN, an access token, which is then sent with it.

set -euo pipefail

runtime=${E2E_RUNTIME:-firecracker}
code_runner_runtime=${E2E_CODE_RUNNER_RUNTIME:-$runtime}
controlplane=${E2E_CONTROLPLANE_URL:-http://127.0.0.1:8020}
ingress=${E2E_INGRESS_URL:-http://127.0.0.1:8030}
ingress_domain=${E2E_INGRESS_DOMAIN:-workload.localhost}
blog=${E2E_BLOG_URL:-http://127.0.0.1:8000}
owner=${E2E_OWNER:-00000000-0000-4000-8000-000000000e2e}
timeout=${E2E_TIMEOUT:-300}
web_image=${E2E_WEB_IMAGE:-nginx:alpine}
tools_image=${E2E_TOOLS_IMAGE:-busybox:1.37}
code_runner=${E2E_CODE_RUNNER:-nodejs-22.14}

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
work=$(mktemp -d)
run_id="e2e-$(date +%s)-$RANDOM"
tasks=()
stacks=()

say() { printf '\n==> %s\n' "$*"; }
ok() { printf '    ok: %s\n' "$*"; }
fail() {
	printf '\nFAIL: %s\n' "$*" >&2
	exit 1
}

# api METHOD PATH [BODY] prints what the control plane answered, and fails
# unless it answered with a 2xx.
api() {
	local method=$1 path=$2 body=${3-} code
	local args=(-sS -X "$method" -o "$work/answer" -w '%{http_code}' --max-time 30)
	if [[ -n $body ]]; then
		args+=(-H 'Content-Type: application/json' --data "$body")
	fi

	code=$(curl "${args[@]}" "$controlplane$path") || return 1
	cat "$work/answer"
	[[ $code == 2* ]]
}

# eventually COMMAND... runs a command until it succeeds, for E2E_TIMEOUT
# seconds at most.
eventually() {
	local deadline=$((SECONDS + timeout))
	until "$@"; do
		if ((SECONDS >= deadline)); then
			return 1
		fi
		sleep 2
	done
}

task_state() { api GET "/api/tasks/$1" | jq -r '.current_state'; }
task_is() { [[ $(task_state "$1" 2>/dev/null) == "$2" ]]; }
# a task at rest after it ran: stopped, completed or failed, which is when the
# control plane lets it be deleted
task_has_ended() { [[ $(task_state "$1" 2>/dev/null) =~ ^(stopped|completed|failed)$ ]]; }
task_is_gone() { ! api GET "/api/tasks/$1" >/dev/null 2>&1 && [[ $(cat "$work/answer") != *current_state* ]]; }
stack_is_gone() { ! api GET "/api/stacks/$1" >/dev/null 2>&1 && [[ $(cat "$work/answer") != *services* ]]; }
stack_is_running() { api GET "/api/stacks/$1" | jq -e '(.services | length) > 0 and all(.services[]; .state == "running")' >/dev/null; }

# what a task says about itself while it waits, when waiting fails
describe_task() {
	api GET "/api/tasks/$1" | jq -c '{current_state, expected_state, node_name, reason, retries}' 2>/dev/null || true
}

# fetch SLUG PORT PATH asks the ingress for a task's port by the task's hostname
fetch() { curl -fsS --max-time 10 -H "Host: $1-$2.$ingress_domain" "$ingress$3"; }
fetch_holds() { fetch "$1" "$2" "$3" 2>/dev/null | grep -q "$4"; }

# log_holds TASK AFTER TEXT: the task's log, from after a moment, holds a text
log_holds() {
	local path="/api/tasks/$1/logs"
	if [[ -n $2 ]]; then
		path+="?after=$(jq -rn --arg after "$2" '$after | @uri')"
	fi
	api GET "$path" | jq -e --arg text "$3" 'any(.items[]?; .content | contains($text))' >/dev/null
}

# the class the control plane says a task runs with. A control plane that
# predates classes says nothing, which is sysbox and nothing else.
check_class() {
	local class
	class=$(api GET "/api/tasks/$1" | jq -r '.runtime // empty')
	if [[ -z $class && $2 != sysbox ]]; then
		fail "the control plane says nothing of a task's class, so $2 cannot have been asked for"
	fi
	[[ -z $class || $class == "$2" ]] || fail "task $1 runs with $class, not $2"
}

probe() { (cd "$root" && PROBE_TOKEN=${E2E_TOKEN:-} go run ./tests/e2e/probe "$@"); }

cleanup() {
	local status=$? uuid
	set +e
	for uuid in "${stacks[@]+"${stacks[@]}"}"; do
		api DELETE "/api/stacks/$uuid" >/dev/null 2>&1
	done
	for uuid in "${tasks[@]+"${tasks[@]}"}"; do
		api DELETE "/api/tasks/$uuid?force=true" >/dev/null 2>&1
	done
	rm -rf "$work"
	if ((status == 0)); then
		printf '\nPASS: the %s class, end to end\n' "$runtime"
	fi
	exit "$status"
}
trap cleanup EXIT

for tool in curl jq go; do
	command -v "$tool" >/dev/null || fail "$tool is needed and is not here"
done

say "the control plane allows $runtime, and a node offers it"
if classes=$(api GET /api/runtimes 2>/dev/null); then
	jq -e --arg class "$runtime" 'any(.items[]; .class == $class and .available)' <<<"$classes" >/dev/null ||
		fail "the control plane lists no available $runtime: $(jq -c '[.items[] | {class, available, nodes}]' <<<"$classes")"
	ok "$(jq -c --arg class "$runtime" '.items[] | select(.class == $class) | {class, default, nodes}' <<<"$classes")"
elif [[ $runtime == sysbox ]]; then
	ok "the control plane predates classes, and runs everything as sysbox"
else
	fail "the control plane does not list runtime classes (GET /api/runtimes)"
fi

say "a $runtime task runs, and answers on its port through the ingress"
task=$(api POST /api/tasks/run "$(jq -n --arg name "$run_id-web" --arg owner "$owner" \
	--arg image "$web_image" --arg runtime "$runtime" '{
		name: $name, owner_uuid: $owner,
		service: {
			image: $image, runtime: $runtime, ports: ["80"],
			deploy: {resources: {limits: {cpus: "0.5", memory: "256M", disk: "256M"}}}
		}
	}')") || fail "the control plane did not take the task: $(cat "$work/answer")"
uuid=$(jq -r .uuid <<<"$task")
slug=$(jq -r .slug <<<"$task")
tasks+=("$uuid")
eventually task_is "$uuid" running || fail "task $uuid never ran: $(describe_task "$uuid")"
check_class "$uuid" "$runtime"
ok "task $uuid ($slug) is running on $(api GET "/api/tasks/$uuid" | jq -r .node_name)"
eventually fetch_holds "$slug" 80 / 'Welcome to nginx' || fail "http://$slug-80.$ingress_domain/ never answered through the ingress"
ok "http://$slug-80.$ingress_domain/ answers"

say "a terminal opens in it, and is resized"
PROBE_TIMEOUT=60s probe attach "${ingress/http/ws}/tasks/$uuid/attach" \
	$'stty size; echo "$((6 * 7))e2e"; exit\n' '40 120' '42e2e' >"$work/terminal" ||
	fail "the terminal did not answer as it should: $(cat "$work/terminal")"
ok "it ran a command and saw itself 40 rows by 120 columns"

say "its log is followed as it is written"
since=$(api GET "/api/tasks/$uuid/logs" | jq -r '.items[-1].at // empty')
marker="$run_id-followed"
fetch "$slug" 80 "/$marker" >/dev/null 2>&1 || true
eventually log_holds "$uuid" "$since" "$marker" || fail "nginx's line for /$marker never reached the log after $since"
ok "the line for /$marker arrived after the ones before it"

say "it stops, restarts, is killed and is deleted"
api POST "/api/tasks/$uuid/stop" >/dev/null || fail "stop was refused: $(cat "$work/answer")"
eventually task_is "$uuid" stopped || fail "task $uuid never stopped: $(describe_task "$uuid")"
ok "stopped"
api POST "/api/tasks/$uuid/restart" >/dev/null || fail "restart was refused: $(cat "$work/answer")"
eventually task_is "$uuid" running || fail "task $uuid never ran again: $(describe_task "$uuid")"
eventually fetch_holds "$slug" 80 / 'Welcome to nginx' || fail "the restarted task never answered through the ingress"
ok "restarted, and answering again"
api POST "/api/tasks/$uuid/kill" >/dev/null || fail "kill was refused: $(cat "$work/answer")"
eventually task_has_ended "$uuid" || fail "task $uuid survived being killed: $(describe_task "$uuid")"
ok "killed: $(task_state "$uuid")"
api DELETE "/api/tasks/$uuid" >/dev/null || fail "delete was refused: $(cat "$work/answer")"
eventually task_is_gone "$uuid" || fail "task $uuid is still there"
ok "deleted"

say "a stack's services reach each other by name"
stack=$(api POST /api/stacks/run "$(jq -n --arg name "$run_id-stack" --arg owner "$owner" \
	--arg runtime "$runtime" --arg web "$web_image" --arg tools "$tools_image" '{
		name: $name, owner_uuid: $owner, runtime: $runtime,
		services: {
			web: {image: $web},
			client: {
				image: $tools,
				command: ["sh", "-c", "until wget -qO- http://web/ | grep -q \"Welcome to nginx\"; do sleep 1; done; echo e2e-peer-reached; exec sleep 3600"]
			}
		}
	}')") || fail "the control plane did not take the stack: $(cat "$work/answer")"
stack_uuid=$(jq -r .uuid <<<"$stack")
stacks+=("$stack_uuid")
eventually stack_is_running "$stack_uuid" || fail "stack $stack_uuid never ran: $(api GET "/api/stacks/$stack_uuid" | jq -c '[.services[] | {service_name, state}]')"
client=$(api GET "/api/stacks/$stack_uuid" | jq -r '.services[] | select(.service_name == "client") | .uuid')
check_class "$client" "$runtime"
eventually log_holds "$client" "" e2e-peer-reached || fail "client never reached web by its name"
ok "client reached http://web/"
api DELETE "/api/stacks/$stack_uuid" >/dev/null || fail "deleting the stack was refused: $(cat "$work/answer")"
eventually stack_is_gone "$stack_uuid" || fail "stack $stack_uuid is still there"
ok "the stack is deleted"

say "a code-runner snippet runs with $code_runner_runtime, and its output comes back"
PROBE_TIMEOUT=${timeout}s probe run-code "${blog/http/ws}/api/ws" "$code_runner" \
	"console.log('e2e-' + 6 * 7 + '-$run_id')" "e2e-42-$run_id" >"$work/snippet" ||
	fail "the snippet did not come back as it should: $(cat "$work/snippet")"
snippet_task=$(sed -n 's/^task: //p' "$work/snippet")
if [[ -n $snippet_task ]] && api GET "/api/tasks/$snippet_task" >/dev/null 2>&1; then
	tasks+=("$snippet_task")
	check_class "$snippet_task" "$code_runner_runtime"
fi
ok "it printed e2e-42-$run_id"
