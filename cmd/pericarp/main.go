// Command pericarp is a CLI for operating on pericarp event stores.
//
// It currently provides event-store migration between app instances and
// backends, as export → portable file → import:
//
//	pericarp export --backend postgres --dsn "$SRC_DSN" --out events.jsonl
//	pericarp import --backend sqlite  --dsn dst.db      --in  events.jsonl
//	pericarp serve  --addr 127.0.0.1:8080   # same operations as async HTTP jobs
//
// All database-driver dependencies (SQLite, Postgres, DynamoDB) live in this
// command so the core library packages stay driver-free.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	// Ctrl-C / SIGTERM cancels the active operation (and running serve jobs).
	// SIGTERM matters for container runtimes (e.g. Kubernetes) that signal it
	// on shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "export":
		err = runExport(ctx, os.Args[2:])
	case "import":
		err = runImport(ctx, os.Args[2:])
	case "serve":
		err = runServe(ctx, os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "pericarp: unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "pericarp: error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `pericarp — event-store migration tool

Usage:
  pericarp export [flags]   Export a store's event feed to a JSONL file (or stdout)
  pericarp import [flags]   Import a JSONL event file into a store (or from stdin)
  pericarp serve  [flags]   Run export/import as async HTTP jobs

Store flags (export, import):
  --backend   sqlite | postgres | dynamo   (env PERICARP_BACKEND)
  --dsn       DSN / sqlite file path        (env PERICARP_DSN;    sqlite, postgres)
  --table     table name                    (env PERICARP_TABLE;  dynamo)
  --endpoint  custom endpoint               (env PERICARP_DYNAMO_ENDPOINT; dynamo, optional)
  --region    AWS region                    (env AWS_REGION;      dynamo, optional)

Run "pericarp <subcommand> -h" for subcommand-specific flags.
`)
}
