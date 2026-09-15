@compaction
Feature: Compacting an event store to one full-state event per live aggregate

  An event store that has run for a long time carries history nobody reads any
  more: a resource that changed five hundred times still replays five hundred
  events to answer one question. Compaction trades that history for a single
  full-state event per surviving aggregate, and moves everything it retires
  into a portable archive file so nothing is actually lost.

  The operator supplies three things: a watermark (a global position — history
  at or below it is a candidate), a state provider that can produce an
  aggregate's full state, and an archive destination. Compact does the rest.

  The order of work is the whole safety argument. The archive is written and
  fsynced first, the compaction events are appended second, and only then is
  anything deleted. Every failure before the delete therefore leaves the store
  exactly as it was, and every delete happens with the archive already durable.
  Because deletes come last, positions are never reused: a retired position
  becomes a permanent gap in the global feed rather than a number a later event
  might claim.

  Deleting is transactional per batch and each batch's archive segment is
  recorded as a manifest, so an interrupted run resumes at the first batch it
  never recorded instead of archiving and compacting the same history twice.

  Two things stay out of scope here. What to retain is policy, and this feature
  only proves the hook exists (Retain) and is honoured. What the state of an
  aggregate actually is belongs to the caller, which is why the state provider
  is a parameter rather than something compaction works out for itself.

  Background:
    Given a compaction-capable event store
    # The suite binds this step once per supported store kind — the in-memory
    # store and the SQLite store — so every scenario below is proved against
    # both. Scenarios that turn on transactional behaviour name their store.
    And a state provider that returns the current full state of any aggregate
    And compaction events are recorded with the type "Resource.Compacted"
    And an archive file that can be fsynced

  Rule: History at or below the watermark collapses into one full-state event per aggregate

    Scenario: Every aggregate below the watermark keeps a single compaction event
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-2 | Resource.Created | 3        |
        | resource-2 | Resource.Renamed | 4        |
      When the store is compacted up to position 4
      Then "resource-1" has exactly one event left, of type "Resource.Compacted"
      And "resource-2" has exactly one event left, of type "Resource.Compacted"
      And the archive holds the 4 retired events

    Scenario: The compaction event carries the full state the provider supplied
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
      And the provider reports "resource-1" as named "final name" and tagged "archived"
      When the store is compacted up to position 2
      Then the compaction event for "resource-1" carries the name "final name" and the tag "archived"

    Scenario: The compaction event records the span of history it replaces
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-2 | Resource.Created | 3        |
      When the store is compacted up to position 5
      Then the compaction event for "resource-1" records compacted_from 1 and compacted_to 5
      And the compaction event for "resource-2" records compacted_from 3 and compacted_to 5

    Scenario: The compaction event continues the aggregate's sequence numbering
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-1 | Resource.Tagged  | 3        |
      When the store is compacted up to position 3
      Then the compaction event for "resource-1" has sequence_no 4

    Scenario: An aggregate entirely above the watermark is left alone
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-2 | Resource.Created | 5        |
        | resource-2 | Resource.Renamed | 6        |
      When the store is compacted up to position 3
      Then "resource-2" still has its original 2 events
      And no compaction event was appended for "resource-2"
      And the archive does not hold any event for "resource-2"

    Scenario: An aggregate straddling the watermark keeps the events above it
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-1 | Resource.Tagged  | 5        |
      When the store is compacted up to position 3
      Then "resource-1" has 2 events left
      And the surviving "Resource.Tagged" event is still at position 5
      And the compaction event for "resource-1" sits above it, at a higher position
      And the archive holds only the 2 events at positions 1 and 2

    @decision
    Scenario: A straddling aggregate is snapshotted as it stands, not as it stood at the watermark
      # DECISION: the provider is asked for the aggregate's CURRENT full state,
      # including the events above the watermark. The compaction event is
      # appended at sequence_no max + 1, so it is the last thing replay applies;
      # a snapshot of the state as at the watermark would sit above the surviving
      # events and silently undo them on every rehydration.
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-1 | Resource.Tagged  | 5        |
      And the "Resource.Tagged" event at position 5 tagged "resource-1" as "urgent"
      When the store is compacted up to position 3
      Then the compaction event for "resource-1" carries the tag "urgent"

  Rule: A deleted aggregate leaves nothing behind but its archive

    Scenario: An aggregate whose last event is a delete gets no compaction event
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Deleted | 2        |
      When the store is compacted up to position 2
      Then "resource-1" has no events left in the store
      And no compaction event was appended for "resource-1"
      And the archive holds the 2 retired events for "resource-1"

    Scenario: An aggregate deleted and then recreated is compacted like any other
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Deleted | 2        |
        | resource-1 | Resource.Created | 3        |
      When the store is compacted up to position 3
      Then "resource-1" has exactly one event left, of type "Resource.Compacted"

    @decision
    Scenario: The caller can say what counts as a delete
      # DECISION: "the last event is a delete" defaults to event types ending in
      # "Deleted", because that is the convention the library's own event types
      # follow. A caller whose domain retires things differently supplies its own
      # predicate rather than renaming its events to suit compaction.
      Given a delete is recognised as any event type ending in "Retired"
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Retired | 2        |
        | resource-2 | Resource.Created | 3        |
        | resource-2 | Resource.Deleted | 4        |
      When the store is compacted up to position 4
      Then "resource-1" has no events left in the store
      And "resource-2" has exactly one event left, of type "Resource.Compacted"

  Rule: The archive is durable before anything is deleted

    Scenario: Retired events are archived in the same shape the exporter writes
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-2 | Resource.Created | 2        |
      When the store is compacted up to position 2
      Then the archive is newline-delimited JSON with an export version header
      And each archived line carries the event's identifier, aggregate, type, payload, sequence_no and position
      And the archived events are in ascending position order

    Scenario: Nothing is deleted until the archive has been fsynced
      Given an archive that records the order of its writes, syncs and the store's deletes
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
      When the store is compacted up to position 2
      Then the archive was fsynced before the first event was deleted
      And the compaction events were appended before the first event was deleted

    Scenario: An archive that fails to write leaves the store untouched
      Given an archive that fails on its first write
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
      When the store is compacted up to position 2
      Then compaction reports the archive failure
      And all 2 original events are still in the store
      And no compaction event was appended
      And no compaction was recorded

    Scenario: An archive that fails to fsync leaves the store untouched
      Given an archive whose fsync fails
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
      When the store is compacted up to position 1
      Then compaction reports the archive failure
      And all 1 original events are still in the store
      And no compaction event was appended

    Scenario: An archive destination that cannot be fsynced is refused up front
      # A plain writer would report a clean success while the archive sat in a
      # page cache, so compaction refuses it rather than deleting behind it.
      Given an archive destination that cannot be fsynced
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
      When the store is compacted up to position 1
      Then compaction is refused because the archive cannot be fsynced
      And no event was read for archiving
      And all 1 original events are still in the store

    Scenario: A resumed archive continues after the last position it already holds
      Given an archive that already holds the events up to position 4
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 5        |
        | resource-1 | Resource.Renamed | 6        |
      When the store is compacted from position 4 up to position 6
      Then the archive holds the 2 events at positions 5 and 6
      And the archive does not repeat any event at or below position 4

  Rule: Each deleted batch is recorded, so a second run resumes rather than repeats

    Scenario: The archive segment for a batch is recorded with the delete
      Given a SQLite-backed event store
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-2 | Resource.Created | 3        |
      When the store is compacted up to position 3
      Then a compaction is recorded covering positions 1 through 3
      And that record reports 3 archived events and the sha256 checksum of the archive segment

    Scenario: A second run over the same watermark changes nothing
      Given a SQLite-backed event store
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
      And the store has already been compacted up to position 2
      When the store is compacted up to position 2 again
      Then no further compaction event was appended
      And nothing further was written to the archive
      And exactly one compaction is recorded

    Scenario: A later run compacts only the history no recorded compaction covers
      Given a SQLite-backed event store
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
      And the store has already been compacted up to position 2
      And the event store then holds:
        | aggregate  | event type       | position |
        | resource-2 | Resource.Created | 7        |
        | resource-2 | Resource.Renamed | 8        |
      When the store is compacted up to position 8
      Then the archive holds only the events at positions 7 and 8
      And "resource-2" has exactly one event left, of type "Resource.Compacted"
      And 2 compactions are recorded

    Scenario: Two runs appending to the same archive leave one importable file
      # The second run is told nothing about where to resume; it works that out
      # from the compactions the first run recorded. Resuming that way still
      # continues an export that already exists, so the file keeps the one
      # header it started with — a header anywhere but the first line is a file
      # no importer can read, which would strand every event in it.
      Given a SQLite-backed event store
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
      When the store is compacted up to position 2
      And the event store then holds:
        | aggregate  | event type       | position |
        | resource-2 | Resource.Created | 7        |
        | resource-2 | Resource.Renamed | 8        |
      And the store is compacted up to position 8
      Then the archive holds the 4 retired events
      And the archive has exactly one export version header, on its first line
      And the archived events are still at their original positions 1, 2, 7 and 8
      And the archive imports cleanly into a fresh store
      And every archived event is in that store with its original identifier, aggregate and sequence_no
      # The original positions live in the archive file, which is what a resumed
      # run reads them back from. Importing is a replay, not a restore of the
      # global feed: the destination assigns positions of its own, so the ones
      # the archive carries are asserted there and not in the imported store.

    Scenario: A batch that fails to delete records nothing and leaves earlier batches recorded
      Given a SQLite-backed event store
      And compaction processes 2 events per batch
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-2 | Resource.Created | 3        |
        | resource-2 | Resource.Renamed | 4        |
      And the store fails to delete the second batch
      When the store is compacted up to position 4
      Then compaction reports the delete failure
      And the events at positions 1 and 2 are gone
      And the events at positions 3 and 4 are still in the store
      And exactly one compaction is recorded, covering positions 1 through 2
      And a rerun compacts only the events at positions 3 and 4

    Scenario: A run interrupted between the archive fsync and the delete does not archive the same batch twice
      # The window the ordering opens on purpose: the segment is already durable
      # when the delete that would have recorded its manifest never commits, so
      # the rerun meets a segment in the archive that nothing accounts for. It
      # may recognise that segment or work around it; what it may not do is
      # leave the aggregate with two snapshots or the archive unrestorable.
      Given a SQLite-backed event store
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-2 | Resource.Created | 3        |
      And the store fails to delete the first batch
      When the store is compacted up to position 3
      And the store is compacted up to position 3 again
      Then compaction succeeds
      And "resource-1" has exactly one event left, of type "Resource.Compacted"
      And "resource-2" has exactly one event left, of type "Resource.Compacted"
      And no event identifier appears twice in the archive
      And the archive imports cleanly into a fresh store

  Rule: Retention holds chosen events back from both compaction and the archive

    Scenario: Events of a retained type stay in the live store
      Given Retain keeps events of type "Resource.Audited"
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Audited | 2        |
        | resource-1 | Resource.Renamed | 3        |
      When the store is compacted up to position 3
      Then the "Resource.Audited" event is still in the store at position 2
      And the archive does not hold the "Resource.Audited" event
      And the archive holds the 2 events at positions 1 and 3

    Scenario: A retained event in the middle of history still rehydrates the aggregate
      # The retained event survives with history compacted away on both sides of
      # it, so the live story is the retained event followed by the compaction
      # event. Replay accepts a snapshot as its first event but refuses a hole
      # after one, so those two have to meet. Nothing here says which of the two
      # gives ground, so nothing here asserts a version.
      Given Retain keeps events of type "Resource.Audited"
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Audited | 2        |
        | resource-1 | Resource.Renamed | 3        |
      And the provider reports "resource-1" as named "final name" and tagged "archived"
      When the store is compacted up to position 3
      And "resource-1" is rebuilt from the events in the store
      Then the rebuilt aggregate is named "final name" and tagged "archived"
      And the "Resource.Audited" event is still in the store at position 2
      And the archive does not hold the "Resource.Audited" event

    Scenario: A retained event stays below the compaction event
      Given Retain keeps events of type "Resource.Audited"
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Audited | 2        |
      When the store is compacted up to position 2
      Then "resource-1" has 2 events left
      And the retained event comes before the compaction event in position order

    Scenario: An aggregate whose newest event is retained still rehydrates
      Given Retain keeps events of type "Resource.Audited"
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Audited | 2        |
      And the provider reports "resource-1" as named "final name" and tagged "archived"
      When the store is compacted up to position 2
      And "resource-1" is rebuilt from the events in the store
      Then the rebuilt aggregate is named "final name" and tagged "archived"
      And the rebuilt aggregate is at version 3
      And the "Resource.Audited" event is still in the store at position 2
      And the archive does not hold the "Resource.Audited" event

    Scenario: Events newer than the retention time stay in the live store
      Given Retain keeps events created on or after "2026-08-01"
      And the event store holds:
        | aggregate  | event type       | position | created    |
        | resource-1 | Resource.Created | 1        | 2026-06-01 |
        | resource-1 | Resource.Renamed | 2        | 2026-08-15 |
      When the store is compacted up to position 2
      Then the event at position 2 is still in the store
      And the archive holds only the event at position 1

    Scenario: An aggregate holding an event retained by its date still rehydrates
      Given Retain keeps events created on or after "2026-08-01"
      And the event store holds:
        | aggregate  | event type       | position | created    |
        | resource-1 | Resource.Created | 1        | 2026-06-01 |
        | resource-1 | Resource.Renamed | 2        | 2026-08-15 |
      And the provider reports "resource-1" as named "final name" and tagged "archived"
      When the store is compacted up to position 2
      And "resource-1" is rebuilt from the events in the store
      Then the rebuilt aggregate is named "final name" and tagged "archived"
      And the rebuilt aggregate is at version 3
      And the event at position 2 is still in the store

    Scenario: An aggregate whose every event is retained is not compacted at all
      Given Retain keeps events of type "Resource.Audited"
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Audited | 1        |
        | resource-1 | Resource.Audited | 2        |
      When the store is compacted up to position 2
      Then "resource-1" still has its original 2 events
      And no compaction event was appended for "resource-1"
      And nothing was written to the archive

    Scenario: A compacted aggregate that kept a retained event goes on recording new history
      # Compaction is not the end of the aggregate's life. Whatever version the
      # run leaves it at has to be the version the next event builds on, or the
      # first thing the aggregate does after being compacted breaks its replay.
      # The rebuilt name comes from the new event and the tag from the
      # compaction event, so both are proved to have been applied, in order.
      Given Retain keeps events of type "Resource.Audited"
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-1 | Resource.Audited | 3        |
      And the provider reports "resource-1" as named "final name" and tagged "archived"
      When the store is compacted up to position 3
      And "resource-1" records a "Resource.Renamed" event
      And "resource-1" is rebuilt from the events in the store
      Then the rebuilt aggregate is named "resource-1" and tagged "archived"
      And the rebuilt aggregate is at version 5
      And the "Resource.Audited" event is still in the store at position 3

  Rule: A compacted store reads back like any other store

    Scenario: Reading an aggregate's history returns the compaction event and what survived it
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-1 | Resource.Tagged  | 6        |
      When the store is compacted up to position 3
      Then reading the history of "resource-1" returns 2 events in sequence order
      And the current version of "resource-1" is 4

    Scenario: Reading from a sequence number that was compacted away starts at the compaction event
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-1 | Resource.Tagged  | 3        |
      When the store is compacted up to position 3
      Then reading the history of "resource-1" from sequence_no 2 returns just the compaction event

    Scenario: The global feed returns the survivors in position order with gaps where events were retired
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-2 | Resource.Created | 3        |
        | resource-2 | Resource.Renamed | 4        |
      When the store is compacted up to position 4
      Then the global feed returns 2 events, both compaction events
      And their positions are above 4 and strictly increasing
      And positions 1 through 4 no longer appear in the feed

    Scenario: Events appended after a compaction keep the feed monotonic
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
      When the store is compacted up to position 2
      And "resource-2" records a "Resource.Created" event
      Then the new event's position is above every compaction event's position
      And no two events in the store share a position

    Scenario: A compacted aggregate rehydrates to its full state from the compaction event alone
      # The compaction event is the aggregate's first surviving event and its
      # sequence_no is well above 1, so replay has to accept a snapshot start.
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-1 | Resource.Renamed | 2        |
        | resource-1 | Resource.Tagged  | 3        |
      And the provider reports "resource-1" as named "final name" and tagged "archived"
      When the store is compacted up to position 3
      And "resource-1" is rebuilt from the events in the store
      Then the rebuilt aggregate is named "final name" and tagged "archived"
      And the rebuilt aggregate is at version 4

    @decision
    Scenario: A gap in the middle of a replay is still refused
      # DECISION: accepting any first sequence number applies only to an
      # aggregate with no state yet. Loosening it further would turn a genuinely
      # lost event into a silently wrong aggregate, which is the failure event
      # sourcing exists to prevent.
      Given a rebuilt aggregate that has already applied a compaction event at sequence_no 4
      When an event at sequence_no 6 is applied to it
      Then the event is refused because a sequence number was skipped

  Rule: A run with nothing to do leaves the store as it found it

    Scenario: Compacting an empty store does nothing
      Given the event store is empty
      When the store is compacted up to position 100
      Then compaction succeeds
      And nothing was written to the archive
      And no compaction was recorded

    Scenario: A watermark above the head of the store compacts everything
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-2 | Resource.Created | 2        |
      When the store is compacted up to position 1000
      Then "resource-1" has exactly one event left, of type "Resource.Compacted"
      And "resource-2" has exactly one event left, of type "Resource.Compacted"
      And the archive holds the 2 retired events

    Scenario: A watermark below every event compacts nothing
      Given the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 4        |
        | resource-2 | Resource.Created | 5        |
      When the store is compacted up to position 3
      Then all 2 original events are still in the store
      And no compaction event was appended
      And nothing was written to the archive
      And no compaction was recorded

  Rule: Compaction refuses what it cannot do safely

    Scenario: A state provider failure aborts the run and leaves the store untouched
      Given a state provider that fails for "resource-2"
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
        | resource-2 | Resource.Created | 2        |
      When the store is compacted up to position 2
      Then compaction reports the provider failure
      And all 2 original events are still in the store
      And no compaction event was appended
      And no compaction was recorded

    Scenario Outline: A store that cannot retire events refuses compaction
      Given a <kind> event store
      And the event store holds:
        | aggregate  | event type       | position |
        | resource-1 | Resource.Created | 1        |
      When the store is compacted up to position 1
      Then compaction is refused because that store does not support it
      And all 1 original events are still in the store
      And nothing was written to the archive

      Examples:
        | kind        |
        | file-backed |
        | DynamoDB    |
