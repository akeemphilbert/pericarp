package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/migration"
)

// TestOpenStoreSQLiteRoundTrip exercises the CLI's store path end to end:
// openStore builds real GORM/SQLite stores, and a full export→import between
// two of them preserves the events and their global order.
func TestOpenStoreSQLiteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	src, closeSrc, err := openStore(ctx, StoreSpec{Backend: "sqlite", DSN: filepath.Join(dir, "src.db")})
	if err != nil {
		t.Fatalf("openStore src: %v", err)
	}
	defer func() { _ = closeSrc() }()

	// Seed across two aggregates, interleaved, so global order is non-trivial.
	seed := func(agg, typ string, seq int, v string) {
		env := domain.NewEventEnvelope[any](map[string]any{"v": v}, agg, typ, seq)
		if err := src.Append(ctx, agg, seq-1, env); err != nil {
			t.Fatalf("append %s#%d: %v", agg, seq, err)
		}
	}
	seed("a", "a.created", 1, "1")
	seed("b", "b.created", 1, "2")
	seed("a", "a.updated", 2, "3")

	var buf bytes.Buffer
	exp, err := migration.Export(ctx, src, &buf, migration.ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if exp.Count != 3 {
		t.Fatalf("export Count = %d, want 3", exp.Count)
	}

	dst, closeDst, err := openStore(ctx, StoreSpec{Backend: "sqlite", DSN: filepath.Join(dir, "dst.db")})
	if err != nil {
		t.Fatalf("openStore dst: %v", err)
	}
	defer func() { _ = closeDst() }()

	imp, err := migration.Import(ctx, dst, &buf, migration.ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if imp.Count != 3 {
		t.Fatalf("import Count = %d, want 3", imp.Count)
	}

	got, err := dst.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("dst has %d events, want 3", len(got))
	}
	wantTypes := []string{"a.created", "b.created", "a.updated"}
	for i, w := range wantTypes {
		if got[i].EventType != w {
			t.Errorf("dst event[%d] type = %q, want %q", i, got[i].EventType, w)
		}
		if pos := int64(i + 1); got[i].Position != pos {
			t.Errorf("dst event[%d] position = %d, want %d", i, got[i].Position, pos)
		}
	}
}

func TestValidateAndExportable(t *testing.T) {
	cases := []struct {
		name          string
		spec          StoreSpec
		validErr      bool
		notExportable bool // validateExportable rejects even though validate passes
	}{
		{"sqlite ok", StoreSpec{Backend: "sqlite", DSN: "x.db"}, false, false},
		{"postgres ok", StoreSpec{Backend: "postgres", DSN: "postgres://h/db"}, false, false},
		{"dynamo valid but not exportable", StoreSpec{Backend: "dynamo", Table: "t"}, false, true},
		{"sqlite missing dsn", StoreSpec{Backend: "sqlite"}, true, true},
		{"dynamo missing table", StoreSpec{Backend: "dynamo"}, true, true},
		{"unknown backend", StoreSpec{Backend: "mysql", DSN: "x"}, true, true},
		{"empty", StoreSpec{}, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if gotErr := c.spec.validate() != nil; gotErr != c.validErr {
				t.Errorf("validate() error = %v, want error %v", c.spec.validate(), c.validErr)
			}
			if gotErr := c.spec.validateExportable() != nil; gotErr != c.notExportable {
				t.Errorf("validateExportable() error = %v, want error %v", c.spec.validateExportable(), c.notExportable)
			}
		})
	}
}
