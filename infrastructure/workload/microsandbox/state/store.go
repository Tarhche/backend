// Package state keeps the workload-microsandbox service's run records on disk,
// one JSON file for each run, so that a service that restarts knows what it
// held.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

const (
	// extension is a record's, which is its run's ID and this.
	extension = ".json"

	// corrupt is added to a record that could not be read, so that it is
	// kept for somebody to look at and not read again.
	corrupt = ".corrupt"

	fileMode      = 0o600
	directoryMode = 0o700
)

// validID is what a run's ID may be. IDs are the service's own, but they name
// files, so one that could climb out of the directory is refused all the same.
var validID = regexp.MustCompile(`^[0-9A-Za-z_-]{1,128}$`)

// Store keeps the records in a directory of their own.
//
// A record is written whole and atomically: to a temporary file beside it,
// synced, and renamed over the old one, and the directory synced after it. A
// service that dies halfway through a write finds the record as it was before,
// never half of each.
type Store struct {
	directory string
	logger    *slog.Logger
}

var _ runs.Records = &Store{}

// NewStore keeps records in directory, which is made when it is not there.
func NewStore(directory string, logger *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return nil, fmt.Errorf("state: the records' directory could not be made: %w", err)
	}

	return &Store{directory: directory, logger: logger}, nil
}

// Save writes a record over the one before it.
func (s *Store) Save(record runs.Record) error {
	if !validID.MatchString(record.ID) {
		return fmt.Errorf("state: %q is not a run's ID", record.ID)
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("state: run %s could not be encoded: %w", record.ID, err)
	}

	temporary, err := os.CreateTemp(s.directory, "."+record.ID+".*.tmp")
	if err != nil {
		return fmt.Errorf("state: run %s could not be written: %w", record.ID, err)
	}

	// whatever happens below, no temporary file is left behind.
	defer os.Remove(temporary.Name())

	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()

		return fmt.Errorf("state: run %s could not be written: %w", record.ID, err)
	}

	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()

		return fmt.Errorf("state: run %s could not be written: %w", record.ID, err)
	}

	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()

		return fmt.Errorf("state: run %s could not be written: %w", record.ID, err)
	}

	if err := temporary.Close(); err != nil {
		return fmt.Errorf("state: run %s could not be written: %w", record.ID, err)
	}

	if err := os.Rename(temporary.Name(), s.path(record.ID)); err != nil {
		return fmt.Errorf("state: run %s could not be written: %w", record.ID, err)
	}

	return s.sync()
}

// Load reads every record, oldest first.
//
// A record that cannot be read is set aside, renamed with .corrupt, and
// reported, rather than keeping the service from starting: one run is lost,
// rather than all of them. Temporary files a service left behind when it died
// in the middle of a write are removed.
func (s *Store) Load() ([]runs.Record, error) {
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return nil, fmt.Errorf("state: the records could not be listed: %w", err)
	}

	var records []runs.Record

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			continue
		}

		if strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".tmp") {
			_ = os.Remove(filepath.Join(s.directory, name))

			continue
		}

		id, isRecord := strings.CutSuffix(name, extension)
		if !isRecord || !validID.MatchString(id) {
			continue
		}

		record, err := s.read(id)
		if err != nil {
			s.setAside(id, err)

			continue
		}

		records = append(records, record)
	}

	slices.SortFunc(records, func(a, b runs.Record) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}

		return strings.Compare(a.ID, b.ID)
	})

	return records, nil
}

// Delete removes a run's record. One that is not there is no error.
func (s *Store) Delete(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("state: %q is not a run's ID", id)
	}

	if err := os.Remove(s.path(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("state: run %s could not be removed: %w", id, err)
	}

	return s.sync()
}

func (s *Store) read(id string) (runs.Record, error) {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return runs.Record{}, err
	}

	var record runs.Record
	if err := json.Unmarshal(data, &record); err != nil {
		return runs.Record{}, err
	}

	if record.ID != id {
		return runs.Record{}, fmt.Errorf("the record is of run %q", record.ID)
	}

	return record, nil
}

func (s *Store) setAside(id string, reason error) {
	s.logger.Error("a run's record could not be read, and is set aside", "run", id, "error", reason)

	if err := os.Rename(s.path(id), s.path(id)+corrupt); err != nil {
		s.logger.Error("a run's record that could not be read could not be set aside", "run", id, "error", err)
	}
}

func (s *Store) path(id string) string {
	return filepath.Join(s.directory, id+extension)
}

// sync makes a rename or a removal in the directory last, which syncing the
// file alone does not.
func (s *Store) sync() error {
	directory, err := os.Open(s.directory)
	if err != nil {
		return fmt.Errorf("state: the records' directory could not be synced: %w", err)
	}
	defer directory.Close()

	if err := directory.Sync(); err != nil {
		return fmt.Errorf("state: the records' directory could not be synced: %w", err)
	}

	return nil
}
