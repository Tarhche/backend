package vmhost

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost/wire"
)

// snapshot answers with a VM's archive as the response's body, streamed as
// the engine writes it, and what was written in a trailer once it has been.
//
// An archive is as large as a disk, and the orchestrator has little memory,
// so nothing of it is held here: each write is sent on as it is made. A
// snapshot that fails before anything is written is answered as any request
// that failed; one that fails after the archive has begun streaming can no
// longer change its status, so it says why in another trailer, and the client
// takes an archive without the first trailer as one that failed.
func (s *Server) snapshot(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Trailer", wire.ArchiveTrailer+", "+wire.ErrorTrailer)

	archive := &archiveWriter{rw: rw}

	written, err := s.engine.Snapshot(r.Context(), r.PathValue("id"), archive)
	if err != nil {
		if !archive.started {
			rw.Header().Del("Trailer")
			s.fail(rw, r, err)

			return
		}

		_ = infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)

		failure, encodeErr := wire.EncodeHeader(wire.ErrorOf(err))
		if encodeErr != nil {
			return
		}

		rw.Header().Set(wire.ErrorTrailer, failure)

		return
	}

	// an engine that wrote nothing still wrote an archive: an empty one.
	archive.begin()

	encoded, err := wire.EncodeHeader(wire.NewArchive(written))
	if err != nil {
		failure, _ := wire.EncodeHeader(wire.ErrorOf(fmt.Errorf("what the archive is cannot be said: %w", err)))
		rw.Header().Set(wire.ErrorTrailer, failure)

		return
	}

	rw.Header().Set(wire.ArchiveTrailer, encoded)
}

// archiveWriter is a response whose status is sent with the first bytes of the
// archive, so that until then a failure can still be answered with one.
type archiveWriter struct {
	rw      http.ResponseWriter
	started bool
}

func (a *archiveWriter) begin() {
	if a.started {
		return
	}

	a.started = true

	a.rw.Header().Set("Content-Type", "application/octet-stream")
	a.rw.WriteHeader(http.StatusOK)
}

func (a *archiveWriter) Write(p []byte) (int, error) {
	a.begin()

	return a.rw.Write(p)
}

// restore replaces the VM the spec in wire.SpecHeader names, or creates it,
// from the archive the request's body streams.
func (s *Server) restore(rw http.ResponseWriter, r *http.Request) {
	header := r.Header.Get(wire.SpecHeader)
	if len(header) == 0 {
		s.fail(rw, r, fmt.Errorf("%w: a restore says what it restores in %s", wire.ErrInvalid, wire.SpecHeader))

		return
	}

	var spec wire.Spec
	if err := wire.DecodeHeader(header, &spec); err != nil {
		s.fail(rw, r, err)

		return
	}

	if len(spec.ID) == 0 {
		s.fail(rw, r, fmt.Errorf("%w: a vm is restored under an id", wire.ErrInvalid))

		return
	}

	instance, err := s.engine.Restore(lasting(r), spec.ToVM(), r.Body)
	if err != nil {
		s.fail(rw, r, err)

		return
	}

	respond(rw, http.StatusOK, wire.NewInstance(instance))
}
