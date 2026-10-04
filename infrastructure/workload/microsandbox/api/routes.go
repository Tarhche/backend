package api

// Version is the major version of this contract: the /v1 in every route, and
// what Info.APIVersion says the service speaks.
const Version = "1"

// The routes are Go 1.22 mux patterns. The service registers them exactly as
// written and the client asks for them, so writing them once, here, is what
// keeps the two from drifting apart.
//
// Each says what it answers and how it fails, and a failure's body is always
// an ErrorResponse. Besides what each lists, a run or an exec that is not
// there is 404 not_found, and creating, starting, stopping, killing,
// restarting or deleting a run before the service is ready is 503
// unavailable.
const (
	// RouteInfo answers Info.
	RouteInfo = "GET /v1/info"

	// RouteListRuns answers the RunList of the node QueryNode names, which is
	// required, narrowed by QueryTask and QuerySlug when they are given.
	// Without a node it is 400 invalid.
	RouteListRuns = "GET /v1/runs"

	// RouteCreateRun takes a RunSpec and answers 201 with the Run, created.
	// It boots nothing, as docker create starts nothing. A spec that does
	// not hold is 400 invalid, one the service cannot run is 400
	// not_supported, and a name its node already uses is 409 name_in_use.
	RouteCreateRun = "POST /v1/runs"

	// RouteGetRun answers the Run.
	RouteGetRun = "GET /v1/runs/{id}"

	// RouteDeleteRun kills the run if it is running, destroys its sandbox
	// and forgets it, its log included, and answers 204.
	RouteDeleteRun = "DELETE /v1/runs/{id}"

	// RouteStartRun pulls the image if it is missing, boots the VM, starts
	// the main process, and answers the Run. A node whose memory budget
	// cannot take the run is 409 capacity, and an image that cannot be
	// pulled is 502 pull_failed.
	RouteStartRun = "POST /v1/runs/{id}/start"

	// RouteStopRun stops the main process, with the image's stop signal and
	// then SIGKILL once the StopRequest's timeout is up, stops the VM, and
	// answers the Run.
	RouteStopRun = "POST /v1/runs/{id}/stop"

	// RouteKillRun ends the main process with SIGKILL at once, stops the VM,
	// and answers the Run.
	RouteKillRun = "POST /v1/runs/{id}/kill"

	// RouteRestartRun stops the run as RouteStopRun does, starts it as
	// RouteStartRun does, and answers the Run.
	RouteRestartRun = "POST /v1/runs/{id}/restart"

	// RouteRunLogs streams the run's log as ContentTypeNDJSON, from
	// QuerySince on, and keeps it open while the run runs when QueryFollow
	// is "true". See LogLine.
	RouteRunLogs = "GET /v1/runs/{id}/logs"

	// RouteRunStats answers the run's Stats. A run that is not running has
	// none, and is 409 not_running.
	RouteRunStats = "GET /v1/runs/{id}/stats"

	// RouteExec upgrades to a websocket speaking ExecSubprotocol, which
	// carries a command inside the run; see ExecRequest. A run that is not
	// running is 409 not_running.
	RouteExec = "GET /v1/runs/{id}/exec"

	// RouteEndExec ends a command RouteExec started, and everything it
	// started in turn, and answers 204: it gives the command a moment to
	// finish, asks it to stop, and kills it if it will not.
	RouteEndExec = "POST /v1/runs/{id}/execs/{exec}/end"

	// RouteNodeStats answers Stats summed over the running runs of the node
	// QueryNode names, so a node's heartbeat makes one call rather than one
	// for each of its runs. Without a node it is 400 invalid.
	RouteNodeStats = "GET /v1/stats"

	// RoutePullImage takes a PullRequest and answers 204 once the image is
	// cached. An image with no variant for the service's architecture is
	// 400 not_supported, and one that cannot be pulled is 502 pull_failed.
	RoutePullImage = "POST /v1/images/pull"
)

// The query parameters the routes read, and the wildcards their patterns name.
const (
	// QueryNode is the node whose runs are listed or summed.
	QueryNode = "node"

	// QueryTask narrows a listing to the runs of one task, by its UUID.
	QueryTask = "task"

	// QuerySlug narrows a listing to the runs answering to one slug.
	QuerySlug = "slug"

	// QuerySince is the earliest line a log stream starts from, inclusive,
	// written as time.RFC3339Nano writes it. Without it the log starts at
	// the beginning.
	QuerySince = "since"

	// QueryFollow is "true" to keep a log stream open while the run runs.
	QueryFollow = "follow"

	// WildcardRun is the run's ID in every /v1/runs/{id} route.
	WildcardRun = "id"

	// WildcardExec is the exec's ID in RouteEndExec, as its started frame
	// gave it.
	WildcardExec = "exec"
)

const (
	// ContentTypeNDJSON is the content type of a streamed answer: one JSON
	// value on each line.
	ContentTypeNDJSON = "application/x-ndjson"

	// ExecSubprotocol names the frames RouteExec carries. The client offers
	// it and the service answers with it, so both ends know which frames
	// they speak before the first one is sent.
	ExecSubprotocol = "workload-microsandbox.v1"
)
