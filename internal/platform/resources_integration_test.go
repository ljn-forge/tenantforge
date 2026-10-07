package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"runtime"
	"sync"
	"testing"
	"time"
)

func (f *fixture) multipart(path, name, contents string, values map[string]string, want int) map[string]any {
	f.t.Helper()
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	part, e := writer.CreateFormFile("file", name)
	if e != nil {
		f.t.Fatal(e)
	}
	part.Write([]byte(contents))
	for k, v := range values {
		writer.WriteField(k, v)
	}
	writer.Close()
	resp := f.request("POST", path, buf, writer.FormDataContentType())
	defer resp.Body.Close()
	out := map[string]any{}
	json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != want {
		f.t.Fatalf("multipart %s got %d want %d (%s)", path, resp.StatusCode, want, stringValue(asMap(out["error"])["code"]))
	}
	return out
}
func (f *fixture) awaitJob(id string) {
	f.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j := asMap(f.call("GET", "/jobs/"+id, nil, 200)["data"])
		if j["status"] == "succeeded" {
			return
		}
		if j["status"] == "failed" {
			f.t.Fatal("job failed")
		}
		time.Sleep(30 * time.Millisecond)
	}
	f.t.Fatal("job completion timeout")
}
func TestFilesImportExportAndAtomicAudit(t *testing.T) {
	f := newFixture(t)
	uploaded := asMap(f.multipart("/files", "../../report.txt", "actual bytes", nil, 200)["data"])
	id := uploaded["id"].(string)
	if uploaded["name"] != "report.txt" {
		t.Fatal("unsafe filename")
	}
	resp := f.request("GET", "/files/"+id+"/download", nil, "")
	b, e := io.ReadAll(resp.Body)
	resp.Body.Close()
	if e != nil || resp.StatusCode != 200 || string(b) != "actual bytes" {
		t.Fatal("file download mismatch")
	}
	if resp.Header.Get("Content-Disposition") == "" {
		t.Fatal("attachment header")
	}
	f.multipart("/resources/tickets/import", "batch.csv", "title,status\nvalid,open\ninvalid,closed\n", nil, 422)
	list := f.call("GET", "/resources/tickets", nil, 200)
	if asMap(list["meta"])["total"].(float64) != 0 {
		t.Fatal("failed import partially committed")
	}
	f.multipart("/resources/tickets/import", "batch.csv", "title\n=SUM(1)\nsecond\n", nil, 200)
	job := asMap(f.call("POST", "/resources/tickets/export", map[string]any{"columns": []string{"title"}}, 200)["data"])
	f.awaitJob(job["id"].(string))
	resp = f.request("GET", "/jobs/"+job["id"].(string)+"/download", nil, "")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !bytes.Contains(b, []byte("'=SUM(1)")) {
		t.Fatal("safe CSV download missing")
	}
	job = asMap(f.call("POST", "/audit/export", map[string]any{"columns": []string{"action", "resource", "changes"}}, 200)["data"])
	f.awaitJob(job["id"].(string))
	resp = f.request("GET", "/jobs/"+job["id"].(string)+"/download", nil, "")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || bytes.Contains(b, []byte("actual bytes")) {
		t.Fatal("audit CSV leaked business data")
	}
	f.call("DELETE", "/files/"+id, nil, 200)
	f.call("GET", "/files/"+id+"/download", nil, 404)
}
func TestExportRevalidatesCurrentActionAndFields(t *testing.T) {
	f := newFixture(t)
	user, _, role := f.addMember("exporter@example.test", []Grant{{Resource: "tickets", Action: "export", Scope: "self", ReadFields: []string{"title"}}, {Resource: "jobs", Action: "read", Scope: "self"}}, "")
	f.call("POST", "/resources/tickets", map[string]any{"owner_id": user, "data": map[string]any{"title": "public", "customer_email": "secret@example.test"}}, 200)
	m := f.loginAs("exporter@example.test")
	job := asMap(m.call("POST", "/resources/tickets/export", map[string]any{"columns": []string{"title", "customer_email"}}, 200)["data"])
	m.awaitJob(job["id"].(string))
	resp := m.request("GET", "/jobs/"+job["id"].(string)+"/download", nil, "")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || bytes.Contains(b, []byte("secret@example.test")) {
		t.Fatal("export leaked field")
	}
	f.call("PATCH", "/roles/"+role, map[string]any{"name": "revoked export", "description": "", "grants": []Grant{{Resource: "jobs", Action: "read", Scope: "self"}}}, 200)
	m.call("GET", "/jobs/"+job["id"].(string)+"/download", nil, 403)
}
func TestConcurrentCreatesAndQuotaAreSerialized(t *testing.T) {
	f := newFixture(t)
	jar, _ := cookiejar.New(nil)
	other := &fixture{t: t, app: f.app, admin: f.admin, server: f.server, client: &http.Client{Jar: jar}, tenant: f.tenant}
	other.refreshCSRF()
	other.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "LongPassword!123"}, 200)
	other.refreshCSRF()
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, c := range []*fixture{f, other} {
		wg.Add(1)
		go func(c *fixture) {
			defer wg.Done()
			r := c.request("POST", "/resources/tickets", bytes.NewBufferString(`{"data":{"title":"parallel"}}`), "application/json")
			io.Copy(io.Discard, r.Body)
			r.Body.Close()
			statuses <- r.StatusCode
		}(c)
	}
	wg.Wait()
	close(statuses)
	for s := range statuses {
		if s != 200 {
			t.Fatalf("concurrent create got %d", s)
		}
	}
	f.call("PATCH", "/platform/tenants/"+f.tenant, map[string]any{"quotas": map[string]int64{"records": 2}}, 200)
	f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "quota blocked"}}, 429)
	var n int
	if e := f.app.DB.QueryRow(context.Background(), "SELECT count(*) FROM security_events").Scan(&n); e != nil || n == 0 {
		t.Fatal("identity audit absent")
	}
}
func TestFileAssignmentUsesActualOwnerMemberDepartment(t *testing.T) {
	f := newFixture(t)
	a := asMap(f.call("POST", "/departments", map[string]any{"name": "A"}, 200)["data"])["id"].(string)
	b := asMap(f.call("POST", "/departments", map[string]any{"name": "B"}, 200)["data"])["id"].(string)
	target, _, _ := f.addMember("target-dept@example.test", []Grant{}, b)
	f.addMember("filemanager@example.test", []Grant{{Resource: "files", Action: "create", Scope: "department"}, {Resource: "members", Action: "assign", Scope: "department"}}, a)
	m := f.loginAs("filemanager@example.test")
	m.multipart("/files", "file.txt", "content", map[string]string{"owner_id": target, "department_id": a}, 403)
}
func TestExportStreamsSelectedFieldsWithinAllocationBudget(t *testing.T) {
	f := newFixture(t)
	f.sql("INSERT INTO resources(id,tenant_id,module,owner_id,data) SELECT gen_random_uuid(),$1::uuid,'tickets',$2::uuid,jsonb_build_object('title','small','description',repeat('x',20000)) FROM generate_series(1,3000)", f.tenant, f.user)
	tx, e := f.app.DB.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	r, _ := http.NewRequest("GET", "http://worker", nil)
	q := &request{app: f.app, tx: tx, r: r, p: Principal{ID: f.user}}
	if e = q.loadTenant(f.tenant); e != nil {
		t.Fatal(e)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	csv, e := q.exportCSV(exportPayload{"tickets", []string{"title"}})
	runtime.ReadMemStats(&after)
	if e != nil || bytes.Count(csv, []byte("small")) != 3000 {
		t.Fatal("stream export row count")
	}
	if after.TotalAlloc-before.TotalAlloc > 48<<20 {
		t.Fatal("export materialized unrequested large fields")
	}
}
