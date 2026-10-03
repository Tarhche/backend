// Package vmstate keeps what vmhost knows about its VMs: one record each,
// state.json, in the VM's own directory under the data directory
// (layout.State), beside its scratch disk and its output.
//
// A VM outlives the vmhost that made it — its machine is a host systemd unit,
// and vmhost is redeployed with every release — so what vmhost knows has to
// outlive it too, and the next vmhost takes up from these records where the
// last one left off. They are on the host's disk rather than in a database
// because nothing but this host's vmhost ever reads them, and because a VM's
// record has to go when its disks go, which a directory of its own makes one
// operation.
//
// Records are read far more than they are written: an orchestrator lists its
// VMs several times a second for its heartbeats. So every record is held in
// memory, and a change is written through to disk — whole, into a file of its
// own that is renamed over the old one, so that nothing ever reads half of one
// — before it is said to be made. Ported from PR #101's record store.
package vmstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

// Store keeps vmhost's records of its VMs.
type Store struct {
	dataDir string

	lock sync.RWMutex
	vms  map[string]vm.VM
}

var _ vm.StateStore = (*Store)(nil)

// Open reads back every record kept under dataDir. A record that cannot be
// read stops vmhost from starting rather than being skipped: the VM it
// describes may still be running, and a vmhost that did not know about it
// would take its machine for one nothing accounts for, and end it.
func Open(dataDir string) (*Store, error) {
	dir := layout.VMs(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	s := &Store{dataDir: dataDir, vms: make(map[string]vm.VM)}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || !vm.IsID(entry.Name()) {
			continue
		}

		encoded, err := os.ReadFile(layout.State(dataDir, entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			// a directory whose record was never written: a create that
			// failed before it got that far, which nothing refers to.
			if err := os.RemoveAll(layout.VM(dataDir, entry.Name())); err != nil {
				return nil, err
			}

			continue
		}

		if err != nil {
			return nil, err
		}

		var record vm.VM
		if err := json.Unmarshal(encoded, &record); err != nil {
			return nil, fmt.Errorf("the record of vm %s cannot be read: %w", entry.Name(), err)
		}

		if record.ID != entry.Name() {
			return nil, fmt.Errorf("the record kept for vm %s is vm %q's", entry.Name(), record.ID)
		}

		s.vms[record.ID] = record
	}

	return s, nil
}

// All is every VM, oldest first.
func (s *Store) All(ctx context.Context) ([]vm.VM, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	all := make([]vm.VM, 0, len(s.vms))
	for _, v := range s.vms {
		all = append(all, clone(v))
	}

	slices.SortFunc(all, func(a vm.VM, b vm.VM) int {
		if order := a.CreatedAt.Compare(b.CreatedAt); order != 0 {
			return order
		}

		return strings.Compare(a.ID, b.ID)
	})

	return all, nil
}

// Get is one VM, or vm.ErrNotFound.
func (s *Store) Get(ctx context.Context, id string) (vm.VM, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	v, found := s.vms[id]
	if !found {
		return vm.VM{}, vm.ErrNotFound
	}

	return clone(v), nil
}

// Put keeps a VM, written whole.
func (s *Store) Put(ctx context.Context, v vm.VM) error {
	if !vm.IsID(v.ID) {
		return fmt.Errorf("%w: %q cannot name a vm", vm.ErrInvalid, v.ID)
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	return s.write(clone(v))
}

// Update changes one VM under the store's lock, so that two changes to it
// never undo each other, and says what it became. What is changed is a copy:
// a change that cannot be written leaves the VM as it was.
func (s *Store) Update(ctx context.Context, id string, change func(*vm.VM)) (vm.VM, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	v, found := s.vms[id]
	if !found {
		return vm.VM{}, vm.ErrNotFound
	}

	changed := clone(v)
	change(&changed)
	changed.ID = id

	if err := s.write(changed); err != nil {
		return vm.VM{}, err
	}

	return clone(changed), nil
}

// Remove forgets a VM, and everything kept beside its record: its scratch
// disk and its output. A VM that is not there is the outcome asked for.
func (s *Store) Remove(ctx context.Context, id string) error {
	// nothing is ever kept under what cannot name a vm, and a path made of
	// one could lead anywhere.
	if !vm.IsID(id) {
		return nil
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if err := os.RemoveAll(layout.VM(s.dataDir, id)); err != nil {
		return err
	}

	delete(s.vms, id)

	return nil
}

// write keeps a record on disk, and then in memory. The lock is held.
func (s *Store) write(v vm.VM) error {
	dir := layout.VM(s.dataDir, v.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	encoded, err := json.Marshal(v)
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())

	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()

		return err
	}

	// on the disk before it is said to be kept: a host that loses power
	// right after a VM was said to be made still knows about it.
	if err := temporary.Sync(); err != nil {
		temporary.Close()

		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	if err := os.Rename(temporary.Name(), filepath.Join(dir, layout.StateName)); err != nil {
		return err
	}

	s.vms[v.ID] = v

	return nil
}

// clone is a VM that shares nothing with the one it was made of, so that what
// is handed out can be changed by whoever has it without changing what is
// kept.
func clone(v vm.VM) vm.VM {
	v.Spec.Labels = maps.Clone(v.Spec.Labels)
	v.Spec.Entrypoint = slices.Clone(v.Spec.Entrypoint)
	v.Spec.Command = slices.Clone(v.Spec.Command)
	v.Spec.Env = slices.Clone(v.Spec.Env)
	v.Spec.ExposedPorts = slices.Clone(v.Spec.ExposedPorts)

	v.Spec.Networks = slices.Clone(v.Spec.Networks)
	for i := range v.Spec.Networks {
		v.Spec.Networks[i].Aliases = slices.Clone(v.Spec.Networks[i].Aliases)
	}

	v.Process.Args = slices.Clone(v.Process.Args)
	v.Process.Env = slices.Clone(v.Process.Env)

	v.Interfaces = slices.Clone(v.Interfaces)
	for i := range v.Interfaces {
		v.Interfaces[i].Aliases = slices.Clone(v.Interfaces[i].Aliases)
	}

	return v
}
