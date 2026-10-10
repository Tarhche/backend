package presenter

import (
	"time"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Pagination is where a listing sits in the whole.
type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewPagination[T any](page workloadControlPlane.Page[T]) Pagination {
	return Pagination{
		TotalPages:  page.TotalPages,
		CurrentPage: page.CurrentPage,
	}
}

// ChosenVM is the Docker VM a container or a stack went into, and whether it
// was made for it.
type ChosenVM struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

func NewChosenVM(chosen workloadControlPlane.ChosenVM) ChosenVM {
	return ChosenVM{
		UUID:    chosen.UUID,
		Name:    chosen.Name,
		Created: chosen.Created,
	}
}

// VMLogLine is one line of what a VM wrote, and where it came from: kernel,
// runtime, main or exec.
type VMLogLine struct {
	At     time.Time `json:"at"`
	Source string    `json:"source"`
	Line   string    `json:"line"`
}

func NewVMLogLines(lines []vm.LogLine) []VMLogLine {
	items := make([]VMLogLine, len(lines))
	for i, l := range lines {
		items[i] = VMLogLine{At: l.At, Source: l.Source, Line: l.Line}
	}

	return items
}

// Truncated reports whether an answer of so many lines may have been cut.
//
// A node answers with no more than the last noderequest.MaxLogLines lines of
// a log, whatever was asked for, and the answer reaches the dashboard without
// saying whether it had to stop there. One that long, to a request that did
// not ask for that many or fewer, is one that may have.
func Truncated(lines int, tail uint) bool {
	return lines >= noderequest.MaxLogLines && (tail == 0 || tail > noderequest.MaxLogLines)
}
