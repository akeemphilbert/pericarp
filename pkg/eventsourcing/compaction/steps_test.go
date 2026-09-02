package compaction_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/compaction"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

const dateLayout = "2006-01-02"

func (w *world) registerSteps(sc *godog.ScenarioContext) {
	// Background and setup
	sc.Step(`^a compaction-capable event store$`, w.aCompactionCapableStore)
	sc.Step(`^a state provider that returns the current full state of any aggregate$`, w.noop)
	sc.Step(`^compaction events are recorded with the type "([^"]*)"$`, w.compactionEventsUseType)
	sc.Step(`^an archive file that can be fsynced$`, w.noop)
	sc.Step(`^the event store is empty$`, w.noop)
	sc.Step(`^a SQLite-backed event store$`, w.aSQLiteStore)
	sc.Step(`^a (file-backed|DynamoDB) event store$`, w.aStoreOfKind)
	sc.Step(`^the event store (?:holds|then holds):$`, w.theEventStoreHolds)
	sc.Step(`^the "([^"]*)" event at position (\d+) tagged "([^"]*)" as "([^"]*)"$`, w.theEventAtPositionTagged)
	sc.Step(`^the provider reports "([^"]*)" as named "([^"]*)" and tagged "([^"]*)"$`, w.theProviderReports)
	sc.Step(`^a state provider that fails for "([^"]*)"$`, w.aProviderThatFailsFor)
	sc.Step(`^a delete is recognised as any event type ending in "([^"]*)"$`, w.aDeleteIsRecognisedBySuffix)
	sc.Step(`^compaction processes (\d+) events per batch$`, w.compactionProcessesPerBatch)
	sc.Step(`^the store fails to delete the second batch$`, w.theStoreFailsToDeleteTheSecondBatch)
	sc.Step(`^Retain keeps events of type "([^"]*)"$`, w.retainKeepsType)
	sc.Step(`^Retain keeps events created on or after "([^"]*)"$`, w.retainKeepsFrom)
	sc.Step(`^the store has already been compacted up to position (\d+)$`, w.theStoreHasAlreadyBeenCompacted)

	// Archive fixtures
	sc.Step(`^an archive that records the order of its writes, syncs and the store's deletes$`, w.aRecordingArchive)
	sc.Step(`^an archive that fails on its first write$`, w.anArchiveThatFailsToWrite)
	sc.Step(`^an archive whose fsync fails$`, w.anArchiveWhoseFsyncFails)
	sc.Step(`^an archive destination that cannot be fsynced$`, w.anArchiveThatCannotBeFsynced)
	sc.Step(`^an archive that already holds the events up to position (\d+)$`, w.anArchiveAlreadyHolding)

	// Actions
	sc.Step(`^the store is compacted up to position (\d+)$`, w.theStoreIsCompactedUpTo)
	sc.Step(`^the store is compacted up to position (\d+) again$`, w.theStoreIsCompactedUpTo)
	sc.Step(`^the store is compacted from position (\d+) up to position (\d+)$`, w.theStoreIsCompactedFromUpTo)
	sc.Step(`^"([^"]*)" records a "([^"]*)" event$`, w.aggregateRecordsAnEvent)
	sc.Step(`^"([^"]*)" is rebuilt from the events in the store$`, w.aggregateIsRebuilt)
	sc.Step(`^a rebuilt aggregate that has already applied a compaction event at sequence_no (\d+)$`, w.aRebuiltAggregateAtSequence)
	sc.Step(`^an event at sequence_no (\d+) is applied to it$`, w.anEventAtSequenceIsApplied)

	// Outcomes: the run itself
	sc.Step(`^compaction succeeds$`, w.compactionSucceeds)
	sc.Step(`^compaction reports the archive failure$`, w.compactionReportsArchiveFailure)
	sc.Step(`^compaction reports the delete failure$`, w.compactionReportsDeleteFailure)
	sc.Step(`^compaction reports the provider failure$`, w.compactionReportsProviderFailure)
	sc.Step(`^compaction is refused because the archive cannot be fsynced$`, w.compactionRefusedUnsyncableArchive)
	sc.Step(`^compaction is refused because that store does not support it$`, w.compactionRefusedUnsupportedStore)
	sc.Step(`^no event was read for archiving$`, w.noEventWasRead)

	// Outcomes: the store
	sc.Step(`^"([^"]*)" has exactly one event left, of type "([^"]*)"$`, w.aggregateHasOneEventOfType)
	sc.Step(`^"([^"]*)" has no events left in the store$`, w.aggregateHasNoEvents)
	sc.Step(`^"([^"]*)" has (\d+) events left$`, w.aggregateHasNEvents)
	sc.Step(`^"([^"]*)" still has its original (\d+) events$`, w.aggregateStillHasOriginalEvents)
	sc.Step(`^all (\d+) original events are still in the store$`, w.allOriginalEventsStillPresent)
	sc.Step(`^no compaction event was appended$`, w.noCompactionEventAnywhere)
	sc.Step(`^no further compaction event was appended$`, w.noFurtherCompactionEvent)
	sc.Step(`^no compaction event was appended for "([^"]*)"$`, w.noCompactionEventFor)
	sc.Step(`^the compaction event for "([^"]*)" has sequence_no (\d+)$`, w.compactionEventSequenceNo)
	sc.Step(`^the compaction event for "([^"]*)" carries the name "([^"]*)" and the tag "([^"]*)"$`, w.compactionEventCarriesNameAndTag)
	sc.Step(`^the compaction event for "([^"]*)" carries the tag "([^"]*)"$`, w.compactionEventCarriesTag)
	sc.Step(`^the compaction event for "([^"]*)" records compacted_from (\d+) and compacted_to (\d+)$`, w.compactionEventRecordsSpan)
	sc.Step(`^the compaction event for "([^"]*)" sits above it, at a higher position$`, w.compactionEventSitsAbove)
	sc.Step(`^the surviving "([^"]*)" event is still at position (\d+)$`, w.survivingEventStillAtPosition)
	sc.Step(`^the "([^"]*)" event is still in the store at position (\d+)$`, w.survivingEventStillAtPosition)
	sc.Step(`^the event at position (\d+) is still in the store$`, w.eventAtPositionStillPresent)
	sc.Step(`^the events at positions (\d+) and (\d+) are still in the store$`, w.eventsAtPositionsStillPresent)
	sc.Step(`^the events at positions (\d+) and (\d+) are gone$`, w.eventsAtPositionsGone)
	sc.Step(`^the retained event comes before the compaction event in position order$`, w.retainedEventComesFirst)

	// Outcomes: the archive
	sc.Step(`^the archive holds the (\d+) retired events$`, w.archiveHoldsNEvents)
	sc.Step(`^the archive holds the (\d+) retired events for "([^"]*)"$`, w.archiveHoldsNEventsFor)
	sc.Step(`^the archive holds the (\d+) events at positions (\d+) and (\d+)$`, w.archiveHoldsAddedPositions)
	sc.Step(`^the archive holds only the (\d+) events at positions (\d+) and (\d+)$`, w.archiveHoldsExactlyPositions)
	sc.Step(`^the archive holds only the events at positions (\d+) and (\d+)$`, w.archiveHoldsOnlyPositions)
	sc.Step(`^the archive holds only the event at position (\d+)$`, w.archiveHoldsOnlyPosition)
	sc.Step(`^the archive does not hold any event for "([^"]*)"$`, w.archiveHoldsNothingFor)
	sc.Step(`^the archive does not hold the "([^"]*)" event$`, w.archiveHoldsNoEventOfType)
	sc.Step(`^the archive does not repeat any event at or below position (\d+)$`, w.archiveDoesNotRepeatBelow)
	sc.Step(`^nothing (?:further )?was written to the archive$`, w.nothingWasWrittenToTheArchive)
	sc.Step(`^the archive is newline-delimited JSON with an export version header$`, w.archiveIsJSONLWithHeader)
	sc.Step(`^each archived line carries the event's identifier, aggregate, type, payload, sequence_no and position$`, w.eachArchivedLineIsComplete)
	sc.Step(`^the archived events are in ascending position order$`, w.archivedEventsAscending)
	sc.Step(`^the archive was fsynced before the first event was deleted$`, w.archiveFsyncedBeforeDelete)
	sc.Step(`^the compaction events were appended before the first event was deleted$`, w.appendedBeforeDelete)

	// Outcomes: the manifests
	sc.Step(`^no compaction was recorded$`, w.noCompactionRecorded)
	sc.Step(`^exactly one compaction is recorded$`, w.exactlyOneCompactionRecorded)
	sc.Step(`^(\d+) compactions are recorded$`, w.nCompactionsRecorded)
	sc.Step(`^(?:a|exactly one) compaction is recorded,? covering positions (\d+) through (\d+)$`, w.compactionRecordedCovering)
	sc.Step(`^that record reports (\d+) archived events and the sha256 checksum of the archive segment$`, w.recordReportsCountAndChecksum)
	sc.Step(`^a rerun compacts only the events at positions (\d+) and (\d+)$`, w.aRerunCompactsOnly)

	// Outcomes: reading a compacted store
	sc.Step(`^reading the history of "([^"]*)" returns (\d+) events in sequence order$`, w.readingHistoryReturns)
	sc.Step(`^reading the history of "([^"]*)" from sequence_no (\d+) returns just the compaction event$`, w.readingHistoryFromReturnsCompactionEvent)
	sc.Step(`^the current version of "([^"]*)" is (\d+)$`, w.currentVersionIs)
	sc.Step(`^the global feed returns (\d+) events, both compaction events$`, w.feedReturnsOnlyCompactionEvents)
	sc.Step(`^their positions are above (\d+) and strictly increasing$`, w.feedPositionsAboveAndIncreasing)
	sc.Step(`^positions (\d+) through (\d+) no longer appear in the feed$`, w.positionsGoneFromFeed)
	sc.Step(`^the new event's position is above every compaction event's position$`, w.newEventIsAboveCompactionEvents)
	sc.Step(`^no two events in the store share a position$`, w.noSharedPositions)
	sc.Step(`^the rebuilt aggregate is named "([^"]*)" and tagged "([^"]*)"$`, w.rebuiltAggregateNamedAndTagged)
	sc.Step(`^the rebuilt aggregate is at version (\d+)$`, w.rebuiltAggregateAtVersion)
	sc.Step(`^the event is refused because a sequence number was skipped$`, w.eventRefusedForSkippedSequence)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (w *world) noop() error { return nil }

// base is the real store, unwrapped. Assertions read through it so that the
// suite's own reads never show up in what a scenario claims compaction did.
func (w *world) base() domain.EventStore {
	if w.observed != nil {
		return w.observed.CompactableEventStore
	}
	return w.store
}

func (w *world) recorded(ctx context.Context) ([]domain.CompactionManifest, error) {
	store, ok := w.base().(domain.CompactableEventStore)
	if !ok {
		return nil, fmt.Errorf("the %s store records no compactions", w.kind)
	}
	return store.Compactions(ctx)
}

func (w *world) feed(ctx context.Context) ([]domain.EventEnvelope[any], error) {
	return w.base().ReadAfter(ctx, 0, 0)
}

func (w *world) compactionEventFor(ctx context.Context, aggregateID string) (domain.EventEnvelope[any], error) {
	events, err := w.base().GetEvents(ctx, aggregateID)
	if err != nil {
		return domain.EventEnvelope[any]{}, err
	}
	var found []domain.EventEnvelope[any]
	for _, event := range events {
		if event.EventType == w.eventType {
			found = append(found, event)
		}
	}
	if len(found) != 1 {
		return domain.EventEnvelope[any]{}, fmt.Errorf("expected exactly one %s event for %s, found %d",
			w.eventType, aggregateID, len(found))
	}
	return found[0], nil
}

func payloadString(event domain.EventEnvelope[any], key string) (string, bool) {
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := payload[key].(string)
	return v, ok
}

// metadataInt64 reads a numeric metadata value. An envelope that came back
// through a SQL store arrived as JSON, where an integer is a float64, so the
// concrete type depends on which store answered.
func metadataInt64(metadata map[string]any, key string) (int64, bool) {
	switch v := metadata[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

// archiveContents is a parsed archive file.
type archiveContents struct {
	hasHeader bool
	version   int
	lines     [][]byte
	events    []domain.EventEnvelope[any]
}

func (w *world) readArchive() (archiveContents, error) {
	var parsed archiveContents

	raw, err := os.ReadFile(w.archivePath)
	if err != nil {
		return parsed, fmt.Errorf("read archive: %w", err)
	}
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var header struct {
			PericarpExport int `json:"pericarp_export"`
		}
		if err := json.Unmarshal(line, &header); err == nil && header.PericarpExport != 0 {
			parsed.hasHeader = true
			parsed.version = header.PericarpExport
			continue
		}

		var event domain.EventEnvelope[any]
		if err := json.Unmarshal(line, &event); err != nil {
			return parsed, fmt.Errorf("archive line is not an event envelope: %w", err)
		}
		parsed.lines = append(parsed.lines, slices.Clone(line))
		parsed.events = append(parsed.events, event)
	}
	return parsed, nil
}

func (w *world) archivePositions() ([]int64, error) {
	parsed, err := w.readArchive()
	if err != nil {
		return nil, err
	}
	positions := make([]int64, len(parsed.events))
	for i, event := range parsed.events {
		positions[i] = event.Position
	}
	return positions, nil
}

func expectPositions(got []int64, want ...int64) error {
	if !slices.Equal(got, want) {
		return fmt.Errorf("expected the archive to hold positions %v, got %v", want, got)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Setup steps
// ---------------------------------------------------------------------------

func (w *world) aCompactionCapableStore() error {
	if _, ok := w.base().(domain.CompactableEventStore); !ok {
		return fmt.Errorf("the %s store is not compaction-capable", w.kind)
	}
	return nil
}

func (w *world) compactionEventsUseType(eventType string) error {
	w.eventType = eventType
	return nil
}

func (w *world) aSQLiteStore(ctx context.Context) error {
	return w.useStore(ctx, storeSQLite)
}

func (w *world) aStoreOfKind(ctx context.Context, kind string) error {
	err := w.useStore(ctx, kind)
	if errors.Is(err, errNoDocker) {
		godog.T(ctx).Skipf("skipping the DynamoDB row: %v", err)
		godog.T(ctx).SkipNow()
	}
	return err
}

func (w *world) theEventStoreHolds(table *godog.Table) error {
	columns := make(map[string]int, len(table.Rows[0].Cells))
	for i, cell := range table.Rows[0].Cells {
		columns[strings.TrimSpace(cell.Value)] = i
	}
	for _, required := range []string{"aggregate", "event type", "position"} {
		if _, ok := columns[required]; !ok {
			return fmt.Errorf("the event table needs a %q column", required)
		}
	}

	for _, row := range table.Rows[1:] {
		cell := func(name string) string {
			i, ok := columns[name]
			if !ok || i >= len(row.Cells) {
				return ""
			}
			return strings.TrimSpace(row.Cells[i].Value)
		}

		var position int64
		if _, err := fmt.Sscanf(cell("position"), "%d", &position); err != nil {
			return fmt.Errorf("unreadable position %q: %w", cell("position"), err)
		}

		created := time.Now().UTC()
		if raw := cell("created"); raw != "" {
			parsed, err := time.Parse(dateLayout, raw)
			if err != nil {
				return fmt.Errorf("unreadable created date %q: %w", raw, err)
			}
			created = parsed
		}

		w.pending = append(w.pending, eventRow{
			aggregate: cell("aggregate"),
			eventType: cell("event type"),
			position:  position,
			created:   created,
			payload:   defaultPayload(cell("aggregate"), cell("event type")),
		})
	}
	return nil
}

// defaultPayload gives each seeded event something to say about the resource,
// so folding an aggregate's events produces a state worth snapshotting.
func defaultPayload(aggregate, eventType string) map[string]any {
	switch {
	case strings.HasSuffix(eventType, "Created"):
		return map[string]any{"name": aggregate}
	case strings.HasSuffix(eventType, "Renamed"):
		return map[string]any{"name": aggregate + " renamed"}
	case strings.HasSuffix(eventType, "Tagged"):
		return map[string]any{"tag": "tagged"}
	default:
		return map[string]any{}
	}
}

func (w *world) theEventAtPositionTagged(eventType string, position int64, aggregate, tag string) error {
	for i := range w.pending {
		row := &w.pending[i]
		if row.position == position && row.eventType == eventType && row.aggregate == aggregate {
			row.payload["tag"] = tag
			return nil
		}
	}
	return fmt.Errorf("no %s event for %s at position %d was declared", eventType, aggregate, position)
}

func (w *world) theProviderReports(aggregate, name, tag string) error {
	w.stateOverrides[aggregate] = map[string]any{"name": name, "tag": tag}
	return nil
}

func (w *world) aProviderThatFailsFor(aggregate string) error {
	w.providerFails[aggregate] = true
	return nil
}

func (w *world) aDeleteIsRecognisedBySuffix(suffix string) error {
	w.isDelete = func(event domain.EventEnvelope[any]) bool {
		return strings.HasSuffix(event.EventType, suffix)
	}
	return nil
}

func (w *world) compactionProcessesPerBatch(size int) error {
	w.batchSize = size
	return nil
}

func (w *world) theStoreFailsToDeleteTheSecondBatch() error {
	if w.observed == nil {
		return fmt.Errorf("the %s store cannot be made to fail a delete", w.kind)
	}
	w.observed.failDeleteOn = 2
	return nil
}

func (w *world) retainKeepsType(eventType string) error {
	w.retain.EventTypes = append(w.retain.EventTypes, eventType)
	return nil
}

func (w *world) retainKeepsFrom(date string) error {
	parsed, err := time.Parse(dateLayout, date)
	if err != nil {
		return fmt.Errorf("unreadable retention date %q: %w", date, err)
	}
	w.retain.NotBefore = parsed
	return nil
}

// theStoreHasAlreadyBeenCompacted runs a complete earlier compaction into an
// archive of its own, so the scenario's archive stays empty and can show what
// the run under test wrote — or did not write.
func (w *world) theStoreHasAlreadyBeenCompacted(ctx context.Context, watermark int64) error {
	if err := w.ensureSeeded(ctx); err != nil {
		return err
	}

	prior, err := os.Create(filepath.Join(w.dir, fmt.Sprintf("prior-%d.jsonl", watermark)))
	if err != nil {
		return fmt.Errorf("open the earlier run's archive: %w", err)
	}
	defer func() { _ = prior.Close() }()

	opts := w.options(watermark, 0)
	opts.Archive = prior
	if _, err := compaction.Compact(ctx, w.store, opts); err != nil {
		return fmt.Errorf("the earlier compaction failed: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Archive fixtures
// ---------------------------------------------------------------------------

func (w *world) aRecordingArchive() error {
	w.journal = []string{}
	return nil
}

func (w *world) anArchiveThatFailsToWrite() error {
	w.archive.failWriteOn = 1
	return nil
}

func (w *world) anArchiveWhoseFsyncFails() error {
	w.archive.failSync = true
	return nil
}

func (w *world) anArchiveThatCannotBeFsynced() error {
	w.plainArchive = &plainWriter{w: w.archive.file}
	return nil
}

// anArchiveAlreadyHolding pre-fills the archive the way an interrupted run
// would have left it: a version header and every event up to that position.
func (w *world) anArchiveAlreadyHolding(ctx context.Context, position int64) error {
	if _, err := fmt.Fprintf(w.archive.file, "{\"pericarp_export\":%d}\n", 1); err != nil {
		return fmt.Errorf("write the existing archive's header: %w", err)
	}
	for pos := int64(1); pos <= position; pos++ {
		event := domain.EventEnvelope[any]{
			ID:          fmt.Sprintf("already-archived-%d", pos),
			AggregateID: "resource-0",
			EventType:   "Resource.Created",
			Payload:     map[string]any{"name": "resource-0"},
			Created:     time.Now().UTC(),
			SequenceNo:  int(pos),
			Metadata:    map[string]any{},
			Position:    pos,
		}
		line, err := json.Marshal(&event)
		if err != nil {
			return fmt.Errorf("marshal an already-archived event: %w", err)
		}
		if _, err := w.archive.file.Write(append(line, '\n')); err != nil {
			return fmt.Errorf("write an already-archived event: %w", err)
		}
	}
	return w.archive.file.Sync()
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func (w *world) theStoreIsCompactedUpTo(ctx context.Context, watermark int64) error {
	return w.compact(ctx, watermark, 0)
}

func (w *world) theStoreIsCompactedFromUpTo(ctx context.Context, from, watermark int64) error {
	return w.compact(ctx, watermark, from)
}

func (w *world) aggregateRecordsAnEvent(ctx context.Context, aggregate, eventType string) error {
	version, err := w.base().GetCurrentVersion(ctx, aggregate)
	if err != nil {
		return err
	}
	event := domain.NewEventEnvelope[any](map[string]any{"name": aggregate}, aggregate, eventType, version+1)
	return w.base().Append(ctx, aggregate, version, event)
}

func (w *world) aggregateIsRebuilt(ctx context.Context, aggregate string) error {
	events, err := w.base().GetEvents(ctx, aggregate)
	if err != nil {
		return err
	}
	slices.SortFunc(events, func(a, b domain.EventEnvelope[any]) int { return a.SequenceNo - b.SequenceNo })

	w.rebuilt = newResource(aggregate)
	for _, event := range events {
		if err := w.rebuilt.Apply(ctx, event); err != nil {
			return fmt.Errorf("replay %s at sequence %d: %w", event.EventType, event.SequenceNo, err)
		}
	}
	return nil
}

func (w *world) aRebuiltAggregateAtSequence(ctx context.Context, sequenceNo int) error {
	w.rebuilt = newResource("resource-1")
	event := domain.NewEventEnvelope[any](map[string]any{"name": "resource-1"}, "resource-1", w.eventType, sequenceNo)
	return w.rebuilt.Apply(ctx, event)
}

func (w *world) anEventAtSequenceIsApplied(ctx context.Context, sequenceNo int) error {
	event := domain.NewEventEnvelope[any](map[string]any{"name": "later"}, w.rebuilt.GetID(), "Resource.Renamed", sequenceNo)
	w.applyErr = w.rebuilt.Apply(ctx, event)
	return nil
}

// ---------------------------------------------------------------------------
// Outcomes: the run
// ---------------------------------------------------------------------------

func (w *world) compactionSucceeds() error {
	return w.err
}

func (w *world) claimFailure(want error, description string) error {
	w.errConsumed = true
	if w.err == nil {
		return fmt.Errorf("expected compaction to report %s, but it succeeded", description)
	}
	if want != nil && !errors.Is(w.err, want) {
		return fmt.Errorf("expected %s, got: %w", description, w.err)
	}
	return nil
}

func (w *world) compactionReportsArchiveFailure() error {
	return w.claimFailure(errArchiveIO, "an archive failure")
}

func (w *world) compactionReportsDeleteFailure() error {
	return w.claimFailure(errDeleteFailed, "a delete failure")
}

func (w *world) compactionReportsProviderFailure() error {
	if err := w.claimFailure(nil, "a state provider failure"); err != nil {
		return err
	}
	if !strings.Contains(w.err.Error(), "state provider") {
		return fmt.Errorf("expected a state provider failure, got: %w", w.err)
	}
	return nil
}

func (w *world) compactionRefusedUnsyncableArchive() error {
	return w.claimFailure(compaction.ErrArchiveNotSyncable, "a refusal to use an archive that cannot be fsynced")
}

func (w *world) compactionRefusedUnsupportedStore() error {
	return w.claimFailure(domain.ErrCompactionNotSupported, "a refusal to compact a store that cannot retire events")
}

func (w *world) noEventWasRead() error {
	if w.observed == nil {
		return fmt.Errorf("the %s store does not observe reads", w.kind)
	}
	if w.observed.reads != 0 {
		return fmt.Errorf("expected the feed never to be read, it was read %d times", w.observed.reads)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Outcomes: the store
// ---------------------------------------------------------------------------

func (w *world) aggregateHasOneEventOfType(ctx context.Context, aggregate, eventType string) error {
	events, err := w.base().GetEvents(ctx, aggregate)
	if err != nil {
		return err
	}
	if len(events) != 1 {
		return fmt.Errorf("expected %s to have exactly one event left, it has %d", aggregate, len(events))
	}
	if events[0].EventType != eventType {
		return fmt.Errorf("expected %s's remaining event to be %s, it is %s", aggregate, eventType, events[0].EventType)
	}
	return nil
}

func (w *world) aggregateHasNoEvents(ctx context.Context, aggregate string) error {
	events, err := w.base().GetEvents(ctx, aggregate)
	if err != nil {
		return err
	}
	if len(events) != 0 {
		return fmt.Errorf("expected %s to have no events left, it has %d", aggregate, len(events))
	}
	return nil
}

func (w *world) aggregateHasNEvents(ctx context.Context, aggregate string, want int) error {
	events, err := w.base().GetEvents(ctx, aggregate)
	if err != nil {
		return err
	}
	if len(events) != want {
		return fmt.Errorf("expected %s to have %d events left, it has %d", aggregate, want, len(events))
	}
	return nil
}

func (w *world) aggregateStillHasOriginalEvents(ctx context.Context, aggregate string, want int) error {
	events, err := w.base().GetEvents(ctx, aggregate)
	if err != nil {
		return err
	}
	if len(events) != want {
		return fmt.Errorf("expected %s to still have its %d events, it has %d", aggregate, want, len(events))
	}
	for _, event := range events {
		if event.EventType == w.eventType {
			return fmt.Errorf("%s was compacted after all: it holds a %s event", aggregate, w.eventType)
		}
	}
	return nil
}

func (w *world) allOriginalEventsStillPresent(ctx context.Context, want int) error {
	events, err := w.feed(ctx)
	if err != nil {
		return err
	}
	if len(events) != want {
		return fmt.Errorf("expected all %d original events to survive, the store holds %d", want, len(events))
	}
	for _, event := range events {
		if event.EventType == w.eventType {
			return fmt.Errorf("the store holds a %s event, so it was not left untouched", w.eventType)
		}
	}
	return nil
}

func (w *world) countCompactionEvents(ctx context.Context) (int, error) {
	events, err := w.feed(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, event := range events {
		if event.EventType == w.eventType {
			n++
		}
	}
	return n, nil
}

func (w *world) noCompactionEventAnywhere(ctx context.Context) error {
	n, err := w.countCompactionEvents(ctx)
	if err != nil {
		return err
	}
	if n != 0 {
		return fmt.Errorf("expected no compaction event, the store holds %d", n)
	}
	return nil
}

// noFurtherCompactionEvent holds when the run under test appended nothing on
// top of what the earlier run left: one compaction event per aggregate, no more.
func (w *world) noFurtherCompactionEvent(ctx context.Context) error {
	if w.report.CompactionEvents != 0 {
		return fmt.Errorf("the run appended %d compaction events, expected none", w.report.CompactionEvents)
	}
	n, err := w.countCompactionEvents(ctx)
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("expected the store to still hold the one compaction event, it holds %d", n)
	}
	return nil
}

func (w *world) noCompactionEventFor(ctx context.Context, aggregate string) error {
	events, err := w.base().GetEvents(ctx, aggregate)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.EventType == w.eventType {
			return fmt.Errorf("%s has a %s event at sequence %d, expected none",
				aggregate, w.eventType, event.SequenceNo)
		}
	}
	return nil
}

func (w *world) compactionEventSequenceNo(ctx context.Context, aggregate string, want int) error {
	event, err := w.compactionEventFor(ctx, aggregate)
	if err != nil {
		return err
	}
	if event.SequenceNo != want {
		return fmt.Errorf("expected the compaction event for %s at sequence_no %d, got %d",
			aggregate, want, event.SequenceNo)
	}
	return nil
}

func (w *world) compactionEventCarriesNameAndTag(ctx context.Context, aggregate, name, tag string) error {
	if err := w.compactionEventCarriesField(ctx, aggregate, "name", name); err != nil {
		return err
	}
	return w.compactionEventCarriesField(ctx, aggregate, "tag", tag)
}

func (w *world) compactionEventCarriesTag(ctx context.Context, aggregate, tag string) error {
	return w.compactionEventCarriesField(ctx, aggregate, "tag", tag)
}

func (w *world) compactionEventCarriesField(ctx context.Context, aggregate, field, want string) error {
	event, err := w.compactionEventFor(ctx, aggregate)
	if err != nil {
		return err
	}
	got, ok := payloadString(event, field)
	if !ok {
		return fmt.Errorf("the compaction event for %s carries no %s: payload is %#v", aggregate, field, event.Payload)
	}
	if got != want {
		return fmt.Errorf("expected the compaction event for %s to carry %s %q, got %q", aggregate, field, want, got)
	}
	return nil
}

func (w *world) compactionEventRecordsSpan(ctx context.Context, aggregate string, from, to int64) error {
	event, err := w.compactionEventFor(ctx, aggregate)
	if err != nil {
		return err
	}
	gotFrom, ok := metadataInt64(event.Metadata, compaction.MetadataCompactedFrom)
	if !ok {
		return fmt.Errorf("the compaction event for %s records no %s: metadata is %#v",
			aggregate, compaction.MetadataCompactedFrom, event.Metadata)
	}
	gotTo, ok := metadataInt64(event.Metadata, compaction.MetadataCompactedTo)
	if !ok {
		return fmt.Errorf("the compaction event for %s records no %s: metadata is %#v",
			aggregate, compaction.MetadataCompactedTo, event.Metadata)
	}
	if gotFrom != from || gotTo != to {
		return fmt.Errorf("expected the compaction event for %s to span %d..%d, it spans %d..%d",
			aggregate, from, to, gotFrom, gotTo)
	}
	return nil
}

func (w *world) compactionEventSitsAbove(ctx context.Context, aggregate string) error {
	event, err := w.compactionEventFor(ctx, aggregate)
	if err != nil {
		return err
	}
	if event.Position <= w.referenced {
		return fmt.Errorf("expected the compaction event for %s above position %d, it is at %d",
			aggregate, w.referenced, event.Position)
	}
	return nil
}

func (w *world) survivingEventStillAtPosition(ctx context.Context, eventType string, position int64) error {
	events, err := w.feed(ctx)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.EventType == eventType && event.Position == position {
			w.referenced = position
			return nil
		}
	}
	return fmt.Errorf("no surviving %s event sits at position %d", eventType, position)
}

func (w *world) positionsInStore(ctx context.Context) (map[int64]bool, error) {
	events, err := w.feed(ctx)
	if err != nil {
		return nil, err
	}
	present := make(map[int64]bool, len(events))
	for _, event := range events {
		present[event.Position] = true
	}
	return present, nil
}

func (w *world) eventAtPositionStillPresent(ctx context.Context, position int64) error {
	return w.eventsAtPositionsStillPresent(ctx, position, position)
}

func (w *world) eventsAtPositionsStillPresent(ctx context.Context, first, second int64) error {
	present, err := w.positionsInStore(ctx)
	if err != nil {
		return err
	}
	for _, position := range []int64{first, second} {
		if !present[position] {
			return fmt.Errorf("expected the event at position %d to still be in the store", position)
		}
	}
	return nil
}

func (w *world) eventsAtPositionsGone(ctx context.Context, first, second int64) error {
	present, err := w.positionsInStore(ctx)
	if err != nil {
		return err
	}
	for _, position := range []int64{first, second} {
		if present[position] {
			return fmt.Errorf("expected the event at position %d to be gone", position)
		}
	}
	return nil
}

func (w *world) retainedEventComesFirst(ctx context.Context) error {
	if len(w.retain.EventTypes) == 0 {
		return errors.New("this scenario declared no retained event type")
	}
	events, err := w.feed(ctx)
	if err != nil {
		return err
	}

	var retained, compacted int64
	for _, event := range events {
		if slices.Contains(w.retain.EventTypes, event.EventType) {
			retained = event.Position
		}
		if event.EventType == w.eventType {
			compacted = event.Position
		}
	}
	if retained == 0 || compacted == 0 {
		return fmt.Errorf("expected both a retained event and a compaction event, got positions %d and %d",
			retained, compacted)
	}
	if retained >= compacted {
		return fmt.Errorf("expected the retained event (position %d) below the compaction event (position %d)",
			retained, compacted)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Outcomes: the archive
// ---------------------------------------------------------------------------

func (w *world) archiveHoldsNEvents(want int) error {
	parsed, err := w.readArchive()
	if err != nil {
		return err
	}
	if len(parsed.events) != want {
		return fmt.Errorf("expected %d archived events, found %d", want, len(parsed.events))
	}
	return nil
}

func (w *world) archiveHoldsNEventsFor(want int, aggregate string) error {
	parsed, err := w.readArchive()
	if err != nil {
		return err
	}
	found := 0
	for _, event := range parsed.events {
		if event.AggregateID == aggregate {
			found++
		}
	}
	if found != want {
		return fmt.Errorf("expected %d archived events for %s, found %d", want, aggregate, found)
	}
	return nil
}

func (w *world) archiveHoldsExactlyPositions(want int, first, second int64) error {
	positions, err := w.archivePositions()
	if err != nil {
		return err
	}
	if len(positions) != want {
		return fmt.Errorf("expected %d archived events, found %d at positions %v", want, len(positions), positions)
	}
	return expectPositions(positions, first, second)
}

// archiveHoldsAddedPositions checks what the run under test contributed,
// ignoring anything the archive already held when the run started resuming
// into it. The "does not repeat" step is what guards the older half.
func (w *world) archiveHoldsAddedPositions(want int, first, second int64) error {
	positions, err := w.archivePositions()
	if err != nil {
		return err
	}
	added := make([]int64, 0, len(positions))
	for _, position := range positions {
		if position > w.lastFrom {
			added = append(added, position)
		}
	}
	if len(added) != want {
		return fmt.Errorf("expected the run to archive %d events, it archived %d at positions %v",
			want, len(added), added)
	}
	return expectPositions(added, first, second)
}

func (w *world) archiveHoldsOnlyPositions(first, second int64) error {
	positions, err := w.archivePositions()
	if err != nil {
		return err
	}
	return expectPositions(positions, first, second)
}

func (w *world) archiveHoldsOnlyPosition(position int64) error {
	positions, err := w.archivePositions()
	if err != nil {
		return err
	}
	return expectPositions(positions, position)
}

func (w *world) archiveHoldsNothingFor(aggregate string) error {
	parsed, err := w.readArchive()
	if err != nil {
		return err
	}
	for _, event := range parsed.events {
		if event.AggregateID == aggregate {
			return fmt.Errorf("the archive holds %s's event at position %d", aggregate, event.Position)
		}
	}
	return nil
}

func (w *world) archiveHoldsNoEventOfType(eventType string) error {
	parsed, err := w.readArchive()
	if err != nil {
		return err
	}
	for _, event := range parsed.events {
		if event.EventType == eventType {
			return fmt.Errorf("the archive holds a %s event at position %d", eventType, event.Position)
		}
	}
	return nil
}

func (w *world) archiveDoesNotRepeatBelow(position int64) error {
	positions, err := w.archivePositions()
	if err != nil {
		return err
	}
	seen := map[int64]int{}
	for _, p := range positions {
		seen[p]++
	}
	for p, count := range seen {
		if count > 1 {
			return fmt.Errorf("the archive holds position %d %d times", p, count)
		}
	}
	// The resumed run must have started above the cursor, not re-read from
	// the beginning: anything it added at or below the cursor is a repeat of
	// history the archive already held.
	added := 0
	for _, p := range positions {
		if p <= position {
			added++
		}
	}
	if added != int(position) {
		return fmt.Errorf("expected the %d events at or below position %d to appear once each, found %d",
			position, position, added)
	}
	return nil
}

func (w *world) nothingWasWrittenToTheArchive() error {
	info, err := os.Stat(w.archivePath)
	if err != nil {
		return fmt.Errorf("stat archive: %w", err)
	}
	if info.Size() != 0 {
		contents, _ := os.ReadFile(w.archivePath)
		return fmt.Errorf("expected an empty archive, it holds %d bytes: %s", info.Size(), contents)
	}
	return nil
}

func (w *world) archiveIsJSONLWithHeader() error {
	parsed, err := w.readArchive()
	if err != nil {
		return err
	}
	if !parsed.hasHeader {
		return errors.New("the archive has no export version header")
	}
	if parsed.version != 1 {
		return fmt.Errorf("expected export format version 1, got %d", parsed.version)
	}
	if len(parsed.events) == 0 {
		return errors.New("the archive holds no events")
	}
	return nil
}

func (w *world) eachArchivedLineIsComplete() error {
	parsed, err := w.readArchive()
	if err != nil {
		return err
	}
	for i, line := range parsed.lines {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(line, &fields); err != nil {
			return fmt.Errorf("archive line %d is not JSON: %w", i+1, err)
		}
		for _, field := range []string{"id", "aggregate_id", "event_type", "payload", "sequence_no", "position"} {
			if _, ok := fields[field]; !ok {
				return fmt.Errorf("archive line %d carries no %s: %s", i+1, field, line)
			}
		}
		event := parsed.events[i]
		if event.ID == "" || event.AggregateID == "" || event.EventType == "" ||
			event.SequenceNo == 0 || event.Position == 0 || event.Payload == nil {
			return fmt.Errorf("archive line %d is incomplete: %s", i+1, line)
		}
	}
	return nil
}

func (w *world) archivedEventsAscending() error {
	positions, err := w.archivePositions()
	if err != nil {
		return err
	}
	if !slices.IsSorted(positions) {
		return fmt.Errorf("archived positions are not ascending: %v", positions)
	}
	return nil
}

func (w *world) indexIn(journal []string, what string) int {
	return slices.Index(journal, what)
}

func (w *world) archiveFsyncedBeforeDelete() error {
	sync, del := w.indexIn(w.journal, "sync"), w.indexIn(w.journal, "delete")
	if sync < 0 || del < 0 {
		return fmt.Errorf("expected both an fsync and a delete, the run did %v", w.journal)
	}
	if sync > del {
		return fmt.Errorf("the archive was fsynced after the first delete: %v", w.journal)
	}
	return nil
}

func (w *world) appendedBeforeDelete() error {
	appended, del := w.indexIn(w.journal, "append"), w.indexIn(w.journal, "delete")
	if appended < 0 || del < 0 {
		return fmt.Errorf("expected both an append and a delete, the run did %v", w.journal)
	}
	if appended > del {
		return fmt.Errorf("compaction events were appended after the first delete: %v", w.journal)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Outcomes: the manifests
// ---------------------------------------------------------------------------

func (w *world) noCompactionRecorded(ctx context.Context) error {
	return w.nCompactionsRecorded(ctx, 0)
}

func (w *world) exactlyOneCompactionRecorded(ctx context.Context) error {
	return w.nCompactionsRecorded(ctx, 1)
}

func (w *world) nCompactionsRecorded(ctx context.Context, want int) error {
	manifests, err := w.recorded(ctx)
	if err != nil {
		return err
	}
	if len(manifests) != want {
		return fmt.Errorf("expected %d recorded compactions, found %d", want, len(manifests))
	}
	return nil
}

func (w *world) compactionRecordedCovering(ctx context.Context, from, to int64) error {
	manifests, err := w.recorded(ctx)
	if err != nil {
		return err
	}
	if len(manifests) != 1 {
		return fmt.Errorf("expected exactly one recorded compaction, found %d", len(manifests))
	}
	if manifests[0].FromPosition != from || manifests[0].ToPosition != to {
		return fmt.Errorf("expected the record to cover positions %d through %d, it covers %d through %d",
			from, to, manifests[0].FromPosition, manifests[0].ToPosition)
	}
	return nil
}

// recordReportsCountAndChecksum verifies the manifest against the archive
// itself: the checksum has to be the SHA-256 of the very bytes the segment
// occupies in the file, otherwise it proves nothing about what was deleted.
func (w *world) recordReportsCountAndChecksum(ctx context.Context, want int) error {
	manifests, err := w.recorded(ctx)
	if err != nil {
		return err
	}
	if len(manifests) != 1 {
		return fmt.Errorf("expected exactly one recorded compaction, found %d", len(manifests))
	}
	manifest := manifests[0]

	if manifest.EventCount != want {
		return fmt.Errorf("expected the record to report %d archived events, it reports %d", want, manifest.EventCount)
	}

	parsed, err := w.readArchive()
	if err != nil {
		return err
	}
	var segment []byte
	for i, event := range parsed.events {
		if event.Position < manifest.FromPosition || event.Position > manifest.ToPosition {
			continue
		}
		segment = append(segment, parsed.lines[i]...)
		segment = append(segment, '\n')
	}
	sum := sha256.Sum256(segment)
	if got := hex.EncodeToString(sum[:]); got != manifest.Checksum {
		return fmt.Errorf("the record's checksum %s does not match the archive segment's %s", manifest.Checksum, got)
	}
	return nil
}

// aRerunCompactsOnly proves the resume: a second run over the same watermark
// picks up exactly the batch the failed one never recorded, and nothing else.
func (w *world) aRerunCompactsOnly(ctx context.Context, first, second int64) error {
	watermark := w.report.Manifests[len(w.report.Manifests)-1].Watermark
	if w.observed != nil {
		w.observed.failDeleteOn = 0
	}
	if err := w.useArchive("rerun.jsonl"); err != nil {
		return err
	}

	report, err := compaction.Compact(ctx, w.store, w.options(watermark, 0))
	if err != nil {
		return fmt.Errorf("the rerun failed: %w", err)
	}
	w.report, w.err, w.errConsumed = report, nil, false

	if err := w.archiveHoldsOnlyPositions(first, second); err != nil {
		return err
	}
	return w.eventsAtPositionsGone(ctx, first, second)
}

// ---------------------------------------------------------------------------
// Outcomes: reading a compacted store
// ---------------------------------------------------------------------------

func (w *world) readingHistoryReturns(ctx context.Context, aggregate string, want int) error {
	events, err := w.base().GetEvents(ctx, aggregate)
	if err != nil {
		return err
	}
	if len(events) != want {
		return fmt.Errorf("expected %d events for %s, got %d", want, aggregate, len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i-1].SequenceNo >= events[i].SequenceNo {
			return fmt.Errorf("%s's history is not in sequence order: %d then %d",
				aggregate, events[i-1].SequenceNo, events[i].SequenceNo)
		}
	}
	return nil
}

func (w *world) readingHistoryFromReturnsCompactionEvent(ctx context.Context, aggregate string, fromVersion int) error {
	events, err := w.base().GetEventsFromVersion(ctx, aggregate, fromVersion)
	if err != nil {
		return err
	}
	if len(events) != 1 {
		return fmt.Errorf("expected one event from sequence_no %d, got %d", fromVersion, len(events))
	}
	if events[0].EventType != w.eventType {
		return fmt.Errorf("expected the compaction event, got %s", events[0].EventType)
	}
	return nil
}

func (w *world) currentVersionIs(ctx context.Context, aggregate string, want int) error {
	version, err := w.base().GetCurrentVersion(ctx, aggregate)
	if err != nil {
		return err
	}
	if version != want {
		return fmt.Errorf("expected %s at version %d, it is at %d", aggregate, want, version)
	}
	return nil
}

func (w *world) feedReturnsOnlyCompactionEvents(ctx context.Context, want int) error {
	events, err := w.feed(ctx)
	if err != nil {
		return err
	}
	if len(events) != want {
		return fmt.Errorf("expected %d events in the feed, got %d", want, len(events))
	}
	for _, event := range events {
		if event.EventType != w.eventType {
			return fmt.Errorf("the feed holds a %s event at position %d, expected only %s events",
				event.EventType, event.Position, w.eventType)
		}
	}
	return nil
}

func (w *world) feedPositionsAboveAndIncreasing(ctx context.Context, above int64) error {
	events, err := w.feed(ctx)
	if err != nil {
		return err
	}
	previous := above
	for _, event := range events {
		if event.Position <= previous {
			return fmt.Errorf("position %d does not sit above %d", event.Position, previous)
		}
		previous = event.Position
	}
	return nil
}

func (w *world) positionsGoneFromFeed(ctx context.Context, from, to int64) error {
	present, err := w.positionsInStore(ctx)
	if err != nil {
		return err
	}
	for position := from; position <= to; position++ {
		if present[position] {
			return fmt.Errorf("position %d still appears in the feed", position)
		}
	}
	return nil
}

func (w *world) newEventIsAboveCompactionEvents(ctx context.Context) error {
	events, err := w.feed(ctx)
	if err != nil {
		return err
	}

	var highestCompaction, newest int64
	var newestType string
	for _, event := range events {
		if event.EventType == w.eventType && event.Position > highestCompaction {
			highestCompaction = event.Position
		}
		if event.Position > newest {
			newest, newestType = event.Position, event.EventType
		}
	}
	if highestCompaction == 0 {
		return errors.New("the store holds no compaction event")
	}
	if newestType == w.eventType {
		return fmt.Errorf("the newest event at position %d is a compaction event, so nothing was appended after", newest)
	}
	if newest <= highestCompaction {
		return fmt.Errorf("the new event at position %d does not sit above the compaction event at %d",
			newest, highestCompaction)
	}
	return nil
}

func (w *world) noSharedPositions(ctx context.Context) error {
	events, err := w.feed(ctx)
	if err != nil {
		return err
	}
	seen := map[int64]string{}
	for _, event := range events {
		if other, clash := seen[event.Position]; clash {
			return fmt.Errorf("events %s and %s share position %d", other, event.ID, event.Position)
		}
		seen[event.Position] = event.ID
	}
	return nil
}

func (w *world) rebuiltAggregateNamedAndTagged(name, tag string) error {
	if w.rebuilt.name != name || w.rebuilt.tag != tag {
		return fmt.Errorf("expected the rebuilt aggregate named %q and tagged %q, got %q and %q",
			name, tag, w.rebuilt.name, w.rebuilt.tag)
	}
	return nil
}

func (w *world) rebuiltAggregateAtVersion(want int) error {
	if got := w.rebuilt.GetSequenceNo(); got != want {
		return fmt.Errorf("expected the rebuilt aggregate at version %d, it is at %d", want, got)
	}
	return nil
}

func (w *world) eventRefusedForSkippedSequence() error {
	if w.applyErr == nil {
		return errors.New("expected the event to be refused, it was applied")
	}
	if !errors.Is(w.applyErr, ddd.ErrInvalidEventSequenceNo) {
		return fmt.Errorf("expected a sequence number refusal, got: %w", w.applyErr)
	}
	return nil
}
