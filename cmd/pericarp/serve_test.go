package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestServer wires the serve routes onto an httptest.Server confined to dir.
func newTestServer(t *testing.T, token, dir string) *httptest.Server {
	t.Helper()
	srv := &migrateServer{
		ctx:     context.Background(),
		token:   token,
		dataDir: dir,
		jobs:    newJobRegistry(),
	}
	ts := httptest.NewServer(srv.routes())
	t.Cleanup(ts.Close)
	return ts
}

func post(t *testing.T, ts *httptest.Server, token, path, body string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return do(t, ts, req)
}

func get(t *testing.T, ts *httptest.Server, token, path string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return do(t, ts, req)
}

func do(t *testing.T, ts *httptest.Server, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, data
}

// jobID extracts the "id" field from a 202 job-creation response.
func jobID(t *testing.T, data []byte) string {
	t.Helper()
	var j struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &j); err != nil || j.ID == "" {
		t.Fatalf("no job id in response %s (err %v)", data, err)
	}
	return j.ID
}

// waitForJob polls /jobs/{id} until the job leaves the running state, then
// returns its decoded body. Jobs run in-process, so this settles in millis.
func waitForJob(t *testing.T, ts *httptest.Server, token, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, data := get(t, ts, token, "/jobs/"+id)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /jobs/%s = %d: %s", id, resp.StatusCode, data)
		}
		var j map[string]any
		if err := json.Unmarshal(data, &j); err != nil {
			t.Fatal(err)
		}
		if j["state"] != "running" {
			return j
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s did not finish: %v", id, j)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

const seedJSONL = `{"pericarp_export":1}
{"id":"e1","aggregate_id":"a","event_type":"a.created","payload":{"v":"1"},"timestamp":"2026-01-01T00:00:00Z","sequence_no":1}
{"id":"e2","aggregate_id":"b","event_type":"b.created","payload":{"v":"2"},"timestamp":"2026-01-01T00:00:01Z","sequence_no":1}
{"id":"e3","aggregate_id":"a","event_type":"a.updated","payload":{"v":"3"},"timestamp":"2026-01-01T00:00:02Z","sequence_no":2}
`

// TestServeExportImportFlow drives the whole HTTP surface end to end against
// temp SQLite databases: seed via import, export, download, re-import.
func TestServeExportImportFlow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seed.jsonl"), []byte(seedJSONL), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t, "", dir)
	srcDSN := filepath.Join(dir, "src.db")
	dstDSN := filepath.Join(dir, "dst.db")

	// Seed src.db by importing the seed file.
	resp, data := post(t, ts, "", "/import", `{"backend":"sqlite","dsn":"`+srcDSN+`","input_path":"seed.jsonl"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("seed import = %d: %s", resp.StatusCode, data)
	}
	if j := waitForJob(t, ts, "", jobID(t, data)); j["count"] != float64(3) || j["state"] != "done" {
		t.Fatalf("seed import job = %v", j)
	}

	// Export src.db to a file inside the data dir.
	resp, data = post(t, ts, "", "/export", `{"backend":"sqlite","dsn":"`+srcDSN+`","output_path":"out.jsonl"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("export = %d: %s", resp.StatusCode, data)
	}
	exportID := jobID(t, data)
	if j := waitForJob(t, ts, "", exportID); j["count"] != float64(3) || j["last_position"] != float64(3) {
		t.Fatalf("export job = %v", j)
	}

	// Download the artifact and confirm it has three event lines (+header).
	resp, data = get(t, ts, "", "/export/"+exportID+"/download")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download = %d: %s", resp.StatusCode, data)
	}
	if n := countEventLines(string(data)); n != 3 {
		t.Fatalf("downloaded artifact has %d event lines, want 3", n)
	}

	// Re-import into dst.db.
	resp, data = post(t, ts, "", "/import", `{"backend":"sqlite","dsn":"`+dstDSN+`","input_path":"out.jsonl"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("re-import = %d: %s", resp.StatusCode, data)
	}
	if j := waitForJob(t, ts, "", jobID(t, data)); j["count"] != float64(3) {
		t.Fatalf("re-import job = %v", j)
	}
}

func TestServeAuth(t *testing.T) {
	ts := newTestServer(t, "secret", t.TempDir())

	// Health is always open.
	if resp, _ := get(t, ts, "", "/healthz"); resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", resp.StatusCode)
	}
	// No / wrong / prefix-less token → 401.
	for _, h := range []string{"", "Bearer wrong", "secret", "XXXXXXXsecret"} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/jobs/none", nil)
		if h != "" {
			req.Header.Set("Authorization", h)
		}
		resp, _ := do(t, ts, req)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("auth %q = %d, want 401", h, resp.StatusCode)
		}
	}
	// Correct token passes auth (job just doesn't exist → 404).
	if resp, _ := get(t, ts, "secret", "/jobs/none"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("authorized unknown job = %d, want 404", resp.StatusCode)
	}
}

func TestServeRejectsBadRequests(t *testing.T) {
	ts := newTestServer(t, "", t.TempDir())
	dsn := filepath.Join(t.TempDir(), "x.db")

	cases := []struct {
		name string
		path string
		body string
	}{
		{"path traversal", "/export", `{"backend":"sqlite","dsn":"` + dsn + `","output_path":"../escape.jsonl"}`},
		{"trailing data", "/export", `{"backend":"sqlite","dsn":"` + dsn + `","output_path":"a.jsonl"}{}`},
		{"unknown field", "/export", `{"backend":"sqlite","dsn":"` + dsn + `","output_path":"a.jsonl","bogus":1}`},
		{"dynamo export", "/export", `{"backend":"dynamo","table":"t","output_path":"a.jsonl"}`},
		{"missing output_path", "/export", `{"backend":"sqlite","dsn":"` + dsn + `"}`},
		{"missing input_path", "/import", `{"backend":"sqlite","dsn":"` + dsn + `"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, data := post(t, ts, "", c.path, c.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s = %d (%s), want 400", c.name, resp.StatusCode, data)
			}
		})
	}
}

func TestResolvePathConfinement(t *testing.T) {
	s := &migrateServer{dataDir: filepath.FromSlash("/data")}
	for _, p := range []string{"a.jsonl", "sub/b.jsonl"} {
		if _, err := s.resolvePath(p); err != nil {
			t.Errorf("resolvePath(%q) unexpected error: %v", p, err)
		}
	}
	for _, p := range []string{"../escape", "../../etc/passwd", "sub/../../escape"} {
		if _, err := s.resolvePath(p); err == nil {
			t.Errorf("resolvePath(%q) = nil error, want rejection", p)
		}
	}
}

// TestJobRegistryEvictsFinishedBeforeRunning fills the registry to capacity
// with one running job (oldest) and the rest finished, then creates another and
// asserts the running job survived — a finished job was evicted instead.
func TestJobRegistryEvictsFinishedBeforeRunning(t *testing.T) {
	reg := newJobRegistry()
	running := reg.create("export") // stays in the default running state
	for i := 1; i < maxRetainedJobs; i++ {
		j := reg.create("export")
		reg.update(j.ID, func(j *job) { j.State = jobDone })
	}
	// At capacity: 1 running (oldest) + (maxRetainedJobs-1) finished.
	reg.create("export")
	if _, ok := reg.snapshot(running.ID); !ok {
		t.Fatal("running job was evicted; a finished job should be evicted first")
	}
}

// countEventLines counts non-empty, non-header lines in an export.
func countEventLines(s string) int {
	n := 0
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, `"pericarp_export"`) {
			continue
		}
		n++
	}
	return n
}
