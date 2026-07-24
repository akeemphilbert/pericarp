package migration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Export streams src's global event feed to w as newline-delimited JSON, one
// EventEnvelope per line, preceded by a version header line. Events are read in
// ascending global Position order (domain.EventStore.ReadAfter) so a later
// Import can preserve the feed order on the destination.
//
// Export is resumable: pass the previous run's ExportReport.LastPosition as
// ExportOptions.FromPosition to continue after an interruption. It honors ctx
// cancellation between batches and returns a partial report with the error.
//
// Only committed, visible events are exported; a store that withholds events
// behind an in-flight writer (e.g. Postgres) will not export them, so quiesce
// writes on the source for a complete migration.
func Export(ctx context.Context, src domain.EventStore, w io.Writer, opts ExportOptions) (ExportReport, error) {
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	report := ExportReport{LastPosition: opts.FromPosition}
	bw := bufio.NewWriter(w)

	if err := writeJSONLine(bw, &header{PericarpExport: FormatVersion}); err != nil {
		return report, fmt.Errorf("write header: %w", err)
	}

	cursor := opts.FromPosition
	for {
		if err := ctx.Err(); err != nil {
			return report, err
		}

		events, err := src.ReadAfter(ctx, cursor, batchSize)
		if err != nil {
			return report, fmt.Errorf("read after position %d: %w", cursor, err)
		}
		if len(events) == 0 {
			break
		}

		for i := range events {
			ev := events[i]
			if err := writeJSONLine(bw, &ev); err != nil {
				return report, fmt.Errorf("write event %s (position %d): %w", ev.ID, ev.Position, err)
			}
			cursor = ev.Position
			report.Count++
			report.LastPosition = ev.Position
		}
		if opts.Progress != nil {
			opts.Progress(report)
		}
	}

	if err := bw.Flush(); err != nil {
		return report, fmt.Errorf("flush export: %w", err)
	}
	return report, nil
}

// writeJSONLine marshals v and writes it followed by a newline. Envelopes must
// be passed by pointer so EventEnvelope's pointer-receiver MarshalJSON is used.
func writeJSONLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	_, err = w.Write([]byte{'\n'})
	return err
}
