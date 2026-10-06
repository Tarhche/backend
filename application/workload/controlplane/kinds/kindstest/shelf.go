package kindstest

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Shelf is fans kept on a shelf rather than as records: the extras of a fan
// strategy that shows them beside its own (Shelved), as the code runner's
// runs are shown among anybody's VMs. A fan on the shelf can be stopped,
// taken away and asked for its log, and is refused anything else under
// "fan", as "immutable".
type Shelf struct {
	lock sync.Mutex

	// fans are what is on it, newest first.
	fans []Fan
}

var _ kind.Extras = &Shelf{}

// NewShelf is a shelf with fans on it, given newest first.
func NewShelf(fans ...Fan) *Shelf {
	return &Shelf{fans: fans}
}

// AShelvedFan is a fan on a shelf: the uuid's, running, made at Moment and
// nobody's own, as changes say otherwise.
func AShelvedFan(uuid string, changes ...func(*Fan)) Fan {
	fan := Typed(AFan(uuid, Running, Running))
	fan.Metadata.OwnerUUID = "guest"
	fan.Metadata.Owners = nil
	fan.Metadata.Node = NodeName

	for _, change := range changes {
		change(&fan)
	}

	return fan
}

func (s *Shelf) All(context.Context) ([]kind.Raw, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	all := make([]kind.Raw, len(s.fans))
	for i := range s.fans {
		all[i] = raw(s.fans[i])
	}

	return all, nil
}

func (s *Shelf) One(_ context.Context, uuid string) (kind.Raw, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for i := range s.fans {
		if s.fans[i].Metadata.UUID == uuid {
			return raw(s.fans[i]), nil
		}
	}

	return kind.Raw{}, domain.ErrNotExists
}

func (s *Shelf) Act(_ context.Context, r kind.Raw, action string, _ []byte) (kind.Raw, bool, domain.ValidationErrors, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	i := slices.IndexFunc(s.fans, func(f Fan) bool { return f.Metadata.UUID == r.Metadata.UUID })
	if i < 0 {
		return kind.Raw{}, false, nil, domain.ErrNotExists
	}

	switch action {
	case "stop":
		s.fans[i].Status.State = Stopping
		s.fans[i].Status.Expected = Stopped

		return raw(s.fans[i]), false, nil, nil
	case "delete":
		s.fans = slices.Delete(s.fans, i, i+1)

		return kind.Raw{}, true, nil, nil
	}

	return kind.Raw{}, false, domain.ValidationErrors{"fan": "immutable"}, nil
}

func (s *Shelf) Query(_ context.Context, r kind.Raw, action string, _ []byte) ([]byte, domain.ValidationErrors, error) {
	if action != "logs" {
		return nil, domain.ValidationErrors{"fan": "immutable"}, nil
	}

	answer, err := json.Marshal([]string{"the shelf's log of " + r.Metadata.UUID})

	return answer, nil, err
}

// Shelved is the fan's control-plane strategy with a shelf of fans shown
// beside its records.
type Shelved struct {
	*Fans

	Shelf *Shelf
}

var _ kind.Extender = &Shelved{}

func (s *Shelved) Extras() kind.Extras {
	return s.Shelf
}

func raw(fan Fan) kind.Raw {
	encoded, err := kind.Encode(fan)
	if err != nil {
		panic(err)
	}

	return encoded
}
