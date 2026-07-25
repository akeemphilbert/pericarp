package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/ksuid"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/migration"
)

// runServe implements `pericarp serve`: expose export/import as asynchronous
// HTTP jobs so a long migration does not block a request.
//
//	POST /export            {backend,dsn,table,...,output_path,from_position,batch_size} -> 202 {job}
//	POST /import            {backend,dsn,table,...,input_path,skip_existing}             -> 202 {job}
//	GET  /jobs/{id}                                                                      -> job status
//	GET  /export/{id}/download                                                           -> the export artifact
//	GET  /healthz
//
// Request bodies carry database credentials, so the server binds to loopback by
// default and, when PERICARP_MIGRATE_TOKEN is set, requires a matching
// "Authorization: Bearer <token>" header.
func runServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", envOr("PERICARP_MIGRATE_ADDR", "127.0.0.1:8080"), "listen address")
	dataDir := fs.String("data-dir", envOr("PERICARP_MIGRATE_DATA_DIR", "."), "directory export/import files are read from and written to")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Confine all job file I/O to this directory so a request body cannot make
	// the server read or clobber arbitrary paths (client paths are resolved
	// relative to it and may not escape it).
	absDir, err := filepath.Abs(*dataDir)
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	srv := &migrateServer{
		ctx:     ctx,
		token:   os.Getenv("PERICARP_MIGRATE_TOKEN"),
		dataDir: absDir,
		jobs:    newJobRegistry(),
	}
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Shut down gracefully when the process is signalled.
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutCtx)
	}()

	fmt.Fprintf(os.Stderr, "pericarp migrate server listening on %s (data dir %s)\n", *addr, absDir)
	if srv.token == "" {
		fmt.Fprintln(os.Stderr, "warning: PERICARP_MIGRATE_TOKEN not set — endpoint is unauthenticated; keep it bound to localhost")
	}
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// migrateServer holds the shared state for the serve endpoints.
type migrateServer struct {
	ctx     context.Context // base context; cancelled on shutdown to stop running jobs
	token   string
	dataDir string // absolute directory job files are confined to
	jobs    *jobRegistry
}

func (s *migrateServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /export", s.handleExport)
	mux.HandleFunc("POST /import", s.handleImport)
	mux.HandleFunc("GET /jobs/{id}", s.handleJob)
	mux.HandleFunc("GET /export/{id}/download", s.handleDownload)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return s.authenticated(mux)
}

// authenticated enforces the bearer token (when configured) on every route
// except the health check.
func (s *migrateServer) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && r.URL.Path != "/healthz" {
			// The header must actually be "Bearer <token>": CutPrefix guards
			// against a value that merely happens to share the token's suffix.
			presented, hasPrefix := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !hasPrefix || subtle.ConstantTimeCompare([]byte(presented), []byte(s.token)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *migrateServer) handleExport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StoreSpec
		OutputPath   string `json:"output_path"`
		FromPosition int64  `json:"from_position"`
		BatchSize    int    `json:"batch_size"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.OutputPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "output_path is required"})
		return
	}
	outPath, err := s.resolvePath(req.OutputPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := req.validateExportable(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	j := s.jobs.create("export")
	opts := migration.ExportOptions{FromPosition: req.FromPosition, BatchSize: req.BatchSize}
	go s.runExportJob(j.ID, req.StoreSpec, outPath, opts)
	writeJSON(w, http.StatusAccepted, j)
}

func (s *migrateServer) handleImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StoreSpec
		InputPath    string `json:"input_path"`
		SkipExisting bool   `json:"skip_existing"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.InputPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input_path is required"})
		return
	}
	inPath, err := s.resolvePath(req.InputPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := req.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	j := s.jobs.create("import")
	go s.runImportJob(j.ID, req.StoreSpec, inPath, req.SkipExisting)
	writeJSON(w, http.StatusAccepted, j)
}

func (s *migrateServer) handleJob(w http.ResponseWriter, r *http.Request) {
	j, ok := s.jobs.snapshot(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *migrateServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	j, ok := s.jobs.snapshot(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if j.Kind != "export" || j.Output == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job has no export artifact"})
		return
	}
	if j.State != jobDone {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "job is " + string(j.State)})
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	http.ServeFile(w, r, j.Output)
}

// resolvePath resolves a client-supplied path within the server's data dir and
// rejects anything that would escape it. filepath.Join cleans the result, so a
// traversal like "../../etc/passwd" lands outside dataDir and is caught by the
// relative-path check.
func (s *migrateServer) resolvePath(p string) (string, error) {
	full := filepath.Join(s.dataDir, p)
	rel, err := filepath.Rel(s.dataDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the server data directory", p)
	}
	return full, nil
}

func (s *migrateServer) runExportJob(id string, spec StoreSpec, outPath string, opts migration.ExportOptions) {
	store, closeStore, err := openStore(s.ctx, spec)
	if err != nil {
		s.jobs.fail(id, err)
		return
	}
	defer func() { _ = closeStore() }()

	f, err := os.Create(outPath)
	if err != nil {
		s.jobs.fail(id, fmt.Errorf("create output file: %w", err))
		return
	}
	defer func() { _ = f.Close() }()

	opts.Progress = func(rep migration.ExportReport) {
		s.jobs.update(id, func(j *job) { j.Count = rep.Count; j.LastPosition = rep.LastPosition })
	}
	report, err := migration.Export(s.ctx, store, f, opts)
	if err != nil {
		s.jobs.fail(id, err)
		return
	}
	s.jobs.update(id, func(j *job) {
		j.State = jobDone
		j.Count = report.Count
		j.LastPosition = report.LastPosition
		j.Output = outPath
	})
}

func (s *migrateServer) runImportJob(id string, spec StoreSpec, inPath string, skipExisting bool) {
	store, closeStore, err := openStore(s.ctx, spec)
	if err != nil {
		s.jobs.fail(id, err)
		return
	}
	defer func() { _ = closeStore() }()

	f, err := os.Open(inPath)
	if err != nil {
		s.jobs.fail(id, fmt.Errorf("open input file: %w", err))
		return
	}
	defer func() { _ = f.Close() }()

	opts := migration.ImportOptions{
		SkipExisting: skipExisting,
		Progress: func(rep migration.ImportReport) {
			s.jobs.update(id, func(j *job) { j.Count = rep.Count; j.Skipped = rep.Skipped })
		},
	}
	report, err := migration.Import(s.ctx, store, f, opts)
	if err != nil {
		s.jobs.fail(id, err)
		return
	}
	s.jobs.update(id, func(j *job) {
		j.State = jobDone
		j.Count = report.Count
		j.Skipped = report.Skipped
	})
}

// --- job registry ---

type jobState string

const (
	jobRunning jobState = "running"
	jobDone    jobState = "done"
	jobFailed  jobState = "failed"
)

// job is the observable state of an async migration. It is only ever read or
// written while holding jobRegistry.mu; handlers receive value snapshots.
type job struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"` // export | import
	State        jobState `json:"state"`
	Count        int64    `json:"count"`
	LastPosition int64    `json:"last_position,omitempty"`
	Skipped      int64    `json:"skipped,omitempty"`
	Output       string   `json:"output,omitempty"` // export artifact path
	Error        string   `json:"error,omitempty"`
}

// maxRetainedJobs bounds the in-memory job history so a long-lived server does
// not grow without limit. When exceeded, the oldest jobs are evicted (FIFO).
const maxRetainedJobs = 1000

type jobRegistry struct {
	mu    sync.Mutex
	jobs  map[string]*job
	order []string // job IDs in creation order, for FIFO eviction
}

func newJobRegistry() *jobRegistry {
	return &jobRegistry{jobs: make(map[string]*job)}
}

func (reg *jobRegistry) create(kind string) job {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if len(reg.order) >= maxRetainedJobs {
		reg.evictOne()
	}
	j := &job{ID: ksuid.New().String(), Kind: kind, State: jobRunning}
	reg.jobs[j.ID] = j
	reg.order = append(reg.order, j.ID)
	return *j
}

// evictOne removes one job to make room, preferring the oldest finished job so
// a still-running job's live status is not dropped (update ignores unknown IDs,
// and /jobs/{id} would 404 mid-run). Only when every retained job is still
// running — pathological — does it evict the oldest, to keep memory bounded.
func (reg *jobRegistry) evictOne() {
	for i, id := range reg.order {
		if j, ok := reg.jobs[id]; ok && j.State != jobRunning {
			delete(reg.jobs, id)
			reg.order = append(reg.order[:i], reg.order[i+1:]...)
			return
		}
	}
	oldest := reg.order[0]
	reg.order = reg.order[1:]
	delete(reg.jobs, oldest)
}

// update mutates the named job under the registry lock. Unknown ids are ignored
// (a job is always created before its goroutine runs).
func (reg *jobRegistry) update(id string, fn func(*job)) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if j, ok := reg.jobs[id]; ok {
		fn(j)
	}
}

func (reg *jobRegistry) fail(id string, err error) {
	reg.update(id, func(j *job) {
		j.State = jobFailed
		j.Error = err.Error()
	})
}

func (reg *jobRegistry) snapshot(id string) (job, bool) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	j, ok := reg.jobs[id]
	if !ok {
		return job{}, false
	}
	return *j, true
}

// --- shared helpers ---

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeJSON decodes a size-limited JSON request body. On failure it writes a
// 400 and returns false so the caller can simply return.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return false
	}
	// Require the body to be exactly one JSON value — reject trailing data.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must contain a single JSON object"})
		return false
	}
	return true
}
