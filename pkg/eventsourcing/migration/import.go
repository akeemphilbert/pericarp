package migration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Import reads an export produced by Export from r and appends its events to
// dst. Events are appended in file order with expectedVersion -1 (no optimistic
// concurrency check — this is a bulk load of known-good history), which
// preserves the source's global feed order on the destination.
//
// A leading version header line is consumed if present; a file without one is
// still importable. ctx cancellation is honored between events. On any error
// Import stops and returns a partial report; re-running with the same file and
// ImportOptions.SkipExisting resumes, since already-imported events are skipped.
//
// Import copies events only. Destination read models/projections are rebuilt by
// the destination app when it runs its own subscribers.
func Import(ctx context.Context, dst domain.EventStore, r io.Reader, opts ImportOptions) (ImportReport, error) {
	p := importer{dst: dst, opts: opts}
	br := bufio.NewReader(r)
	lineNo := 0

	for {
		raw, readErr := br.ReadBytes('\n')
		// Count every physical line so an error's "line N" matches the file even
		// across blank lines; only non-empty lines are processed.
		if len(raw) > 0 {
			lineNo++
			if line := bytes.TrimSpace(raw); len(line) > 0 {
				if err := p.process(ctx, line, lineNo); err != nil {
					return p.report, err
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return p.report, nil
			}
			return p.report, fmt.Errorf("read: %w", readErr)
		}
	}
}

// importer carries the per-run state Import threads through each line.
type importer struct {
	dst           domain.EventStore
	opts          ImportOptions
	report        ImportReport
	headerChecked bool
}

// process handles one non-empty line: the optional header on the first line,
// otherwise an event envelope to append.
func (p *importer) process(ctx context.Context, line []byte, lineNo int) error {
	if !p.headerChecked {
		p.headerChecked = true
		consumed, err := tryHeader(line)
		if err != nil {
			return err
		}
		if consumed {
			return nil
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	var ev domain.EventEnvelope[any]
	if err := json.Unmarshal(line, &ev); err != nil {
		return fmt.Errorf("line %d: parse event: %w", lineNo, err)
	}
	if ev.ID == "" || ev.AggregateID == "" {
		return fmt.Errorf("line %d: event missing id or aggregate_id", lineNo)
	}

	if p.opts.SkipExisting {
		switch _, err := p.dst.GetEventByID(ctx, ev.ID); {
		case err == nil:
			p.report.Skipped++
			p.maybeProgress()
			return nil
		case errors.Is(err, domain.ErrEventNotFound):
			// Not present — fall through to append.
		default:
			return fmt.Errorf("line %d: check existing event %s: %w", lineNo, ev.ID, err)
		}
	}

	if err := p.dst.Append(ctx, ev.AggregateID, -1, ev); err != nil {
		return fmt.Errorf("line %d: append event %s: %w", lineNo, ev.ID, err)
	}
	p.report.Count++
	p.maybeProgress()
	return nil
}

// importProgressInterval is how often (in events handled) Import invokes the
// progress callback, if one is set.
const importProgressInterval = 1000

// maybeProgress invokes the progress callback on interval boundaries.
func (p *importer) maybeProgress() {
	if p.opts.Progress == nil {
		return
	}
	if (p.report.Count+p.report.Skipped)%importProgressInterval == 0 {
		p.opts.Progress(p.report)
	}
}

// tryHeader reports whether line is a format header (and thus consumed). It
// errors if the header declares a format newer than this build supports; a
// non-header line returns (false, nil) so it is processed as an event.
func tryHeader(line []byte) (bool, error) {
	var h header
	if err := json.Unmarshal(line, &h); err != nil || h.PericarpExport == 0 {
		return false, nil
	}
	if h.PericarpExport > FormatVersion {
		return true, fmt.Errorf("unsupported export format version %d (this build supports up to %d)", h.PericarpExport, FormatVersion)
	}
	return true, nil
}
