# Pericarp CLI

`pericarp` is a command-line tool for operating on pericarp event stores. Its
first capability is **event-store migration** between app instances and
backends.

Build it with `make build-cli` (outputs `bin/pericarp`).

## Migrating data between instances

A pericarp app's event store is its source of truth — read models/projections
are derived. Migration copies the **events**; the destination app rebuilds its
own projections when it next runs. It is modelled as **export → portable file →
import**, so the source and destination never need to be reachable at once and
may use different backends (SQLite, Postgres, DynamoDB).

```bash
# Export the source feed to a backend-neutral JSONL file
pericarp export --backend postgres --dsn "$SRC_DSN" --out events.jsonl

# Import it into the destination (any backend)
pericarp import --backend sqlite --dsn ./dest.db --in events.jsonl
```

The file is newline-delimited JSON, one event envelope per line (preceded by a
version header). Export reads in global position order; import appends in file
order, so the destination's feed order matches the source. Payloads are copied
as-is (no schema upcasting).

### Store flags (`export`, `import`)

| Flag | Env | Applies to |
|------|-----|------------|
| `--backend` | `PERICARP_BACKEND` | all (`sqlite`\|`postgres`\|`dynamo`) |
| `--dsn` | `PERICARP_DSN` | sqlite, postgres (DSN / file path) |
| `--table` | `PERICARP_TABLE` | dynamo (table name) |
| `--endpoint` | `PERICARP_DYNAMO_ENDPOINT` | dynamo (optional) |
| `--region` | `AWS_REGION` | dynamo (optional) |

`export` also takes `--out` (default stdout), `--from-position` (resume), and
`--batch-size`. `import` takes `--in` (default stdin) and `--skip-existing`
(idempotent re-runs). A GORM destination self-provisions its schema; a DynamoDB
destination table and its event-id GSI must already exist.

## Migrating over HTTP

`pericarp serve` runs the same operations as asynchronous jobs, so a long
migration does not block the request:

```bash
pericarp serve --addr 127.0.0.1:8080     # env: PERICARP_MIGRATE_ADDR
```

| Method & path | Purpose |
|---------------|---------|
| `POST /export` | start an export job → `202 {"id": ...}` |
| `POST /import` | start an import job → `202 {"id": ...}` |
| `GET /jobs/{id}` | poll job state (`running`\|`done`\|`failed`), counts, error |
| `GET /export/{id}/download` | download a completed export's artifact |
| `GET /healthz` | health check |

Request bodies carry database credentials, so the server binds to loopback by
default. Set `PERICARP_MIGRATE_TOKEN` to require an
`Authorization: Bearer <token>` header on every route except `/healthz`.

```bash
curl -s -XPOST localhost:8080/export -H 'Authorization: Bearer '"$TOKEN" \
  -d '{"backend":"postgres","dsn":"'"$SRC_DSN"'","output_path":"/data/events.jsonl"}'
```

## Scope

The tool copies events only. Destination read models/projections are rebuilt by
the destination app's own subscribers; event schemas are not upcast; and tables
that are not derivable from the event feed are not carried.
