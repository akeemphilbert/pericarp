package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/migration"
)

// runExport implements `pericarp export`: read a store's event feed and write
// it as JSONL to a file (--out) or stdout.
func runExport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	var spec StoreSpec
	addStoreFlags(fs, &spec)
	out := fs.String("out", "", "output file path (default: stdout)")
	fromPosition := fs.Int64("from-position", 0, "resume export after this global position")
	batchSize := fs.Int("batch-size", 0, "events read per batch (default 500)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, closeStore, err := openStore(ctx, spec)
	if err != nil {
		return err
	}
	defer func() { _ = closeStore() }()

	w := io.Writer(os.Stdout)
	closeOut := func() error { return nil }
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		w = f
		closeOut = f.Close
	}

	report, err := migration.Export(ctx, store, w, migration.ExportOptions{
		FromPosition: *fromPosition,
		BatchSize:    *batchSize,
	})
	// A failed Close on the output file can mask a partial write, so surface it
	// (unless an export error already takes precedence).
	if cerr := closeOut(); cerr != nil && err == nil {
		err = fmt.Errorf("close output file: %w", cerr)
	}
	if err != nil {
		return err
	}
	// Progress/summary go to stderr so stdout stays clean when piping JSONL.
	fmt.Fprintf(os.Stderr, "exported %d events (last position %d)\n", report.Count, report.LastPosition)
	return nil
}

// runImport implements `pericarp import`: read a JSONL event file (--in or
// stdin) and append its events to a store.
func runImport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	var spec StoreSpec
	addStoreFlags(fs, &spec)
	in := fs.String("in", "", "input file path (default: stdin)")
	skipExisting := fs.Bool("skip-existing", false, "skip events already present (idempotent re-runs)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, closeStore, err := openStore(ctx, spec)
	if err != nil {
		return err
	}
	defer func() { _ = closeStore() }()

	r := io.Reader(os.Stdin)
	if *in != "" {
		f, err := os.Open(*in)
		if err != nil {
			return fmt.Errorf("open input file: %w", err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}

	report, err := migration.Import(ctx, store, r, migration.ImportOptions{
		SkipExisting: *skipExisting,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "imported %d events (%d skipped)\n", report.Count, report.Skipped)
	return nil
}
