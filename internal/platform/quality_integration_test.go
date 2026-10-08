package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ljn-forge/tenantforge/extension"
)

func TestResourceDepartmentNullAndOmission(t *testing.T) {
	f := newFixture(t)
	d := asMap(f.call("POST", "/departments", map[string]any{"name": "Operations"}, 200)["data"])["id"].(string)
	r := asMap(f.call("POST", "/resources/tickets", map[string]any{"department_id": d, "data": map[string]any{"title": "department semantics"}}, 200)["data"])
	id := r["id"].(string)
	r = asMap(f.call("PATCH", "/resources/tickets/"+id, map[string]any{"version": 1, "data": map[string]any{"title": "preserved"}}, 200)["data"])
	if r["department_id"] != d {
		t.Fatal("omission changed department")
	}
	r = asMap(f.call("PATCH", "/resources/tickets/"+id, map[string]any{"version": 2, "department_id": nil, "data": map[string]any{}}, 200)["data"])
	if r["department_id"] != "" {
		t.Fatal("explicit null did not clear department")
	}
}

type gatedTransferStorage struct {
	Storage
	started chan struct{}
	release chan struct{}
}

func (s *gatedTransferStorage) Put(ctx context.Context, k string, r io.Reader, n int64, ct string) error {
	close(s.started)
	select {
	case <-s.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.Storage.Put(ctx, k, r, n, ct)
}
func TestSlowUploadReleasesPoolAndSessionLocks(t *testing.T) {
	f := newFixtureWithConfig(t, func(c *Config) { c.PoolConfig.MaxConns = 1 })
	s := &gatedTransferStorage{Storage: f.app.storage(), started: make(chan struct{}), release: make(chan struct{})}
	f.app.setStorage(s)
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(s.release) })
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	p, _ := w.CreateFormFile("file", "slow.txt")
	p.Write([]byte("bytes"))
	w.Close()
	done := make(chan int, 1)
	go func() {
		r, _ := http.NewRequest("POST", f.server.URL+"/api/v1/files", buf)
		r.Header.Set("Content-Type", w.FormDataContentType())
		r.Header.Set("X-CSRF-Token", f.csrf)
		r.Header.Set("X-Tenant-ID", f.tenant)
		resp, e := f.client.Do(r)
		if e != nil {
			done <- 0
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		done <- resp.StatusCode
	}()
	select {
	case <-s.started:
	case <-time.After(time.Second):
		t.Fatal("upload did not reach object storage")
	}
	client := &http.Client{Jar: f.client.Jar, Timeout: 700 * time.Millisecond}
	r, _ := http.NewRequest("POST", f.server.URL+"/api/v1/resources/tickets", bytes.NewBufferString(`{"data":{"title":"unrelated mutation"}}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", f.csrf)
	r.Header.Set("X-Tenant-ID", f.tenant)
	resp, e := client.Do(r)
	if e != nil {
		t.Fatal("slow transfer holds database connection or session lock")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("unrelated mutation denied")
	}
	releaseOnce.Do(func() { close(s.release) })
	select {
	case status := <-done:
		if status != 200 {
			t.Fatalf("upload final status %d", status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upload failed to complete after unrelated mutation")
	}
}
func TestRowCapabilitiesUseUnfilteredConditions(t *testing.T) {
	f := newFixture(t)
	u, _, _ := f.addMember("scoped@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}, {Resource: "tickets", Action: "update", Scope: "self", WriteFields: []string{"title"}, Conditions: []Condition{{Field: "priority", Op: "eq", Value: "urgent"}}}}, "")
	f.call("POST", "/resources/tickets", map[string]any{"owner_id": u, "data": map[string]any{"title": "urgent", "priority": "urgent"}}, 200)
	f.call("POST", "/resources/tickets", map[string]any{"owner_id": u, "data": map[string]any{"title": "normal", "priority": "normal"}}, 200)
	m := f.loginAs("scoped@example.test")
	rows := m.call("GET", "/resources/tickets", nil, 200)["data"].([]any)
	for _, v := range rows {
		r := asMap(v)
		cap := asMap(r["capabilities"])
		if cap == nil {
			t.Fatal("missing server-authoritative row capabilities")
		}
		data := asMap(r["data"])
		if data["priority"] != nil {
			t.Fatal("capability computation leaked a hidden condition field")
		}
		b, _ := json.Marshal(cap["actions"])
		can := bytes.Contains(b, []byte(`"update"`))
		if can != (data["title"] == "urgent") {
			t.Fatal("capabilities did not evaluate original conditions")
		}
	}
}
func TestIndependentWorkerLanesDoNotStarveMailOrGC(t *testing.T) {
	f := newFixtureWithConfig(t, func(c *Config) { c.SMTPHost = "127.0.0.1"; c.SMTPPort = "1039"; c.SMTPFrom = "tests@localhost" })
	s := &gatedTransferStorage{Storage: f.app.storage(), started: make(chan struct{}), release: make(chan struct{})}
	f.app.setStorage(s)
	defer close(s.release)
	f.call("POST", "/resources/tickets/export", map[string]any{"columns": []string{"title"}}, 200)
	select {
	case <-s.started:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not reach storage")
	}
	f.call("POST", "/auth/reset/request", map[string]any{"email": "owner@example.test"}, 200)
	key := uuid.NewString()
	if e := s.Storage.Put(context.Background(), key, bytes.NewReader([]byte("gc")), 2, "text/plain"); e != nil {
		t.Fatal(e)
	}
	if e := f.app.reserveObject(context.Background(), key, f.tenant); e != nil {
		t.Fatal(e)
	}
	f.admin.Exec(context.Background(), "UPDATE storage_gc SET next_attempt_at=now() WHERE storage_key=$1", key)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var mails, gc int
		f.admin.QueryRow(context.Background(), "SELECT count(*) FROM mail_outbox WHERE status='sent'").Scan(&mails)
		f.admin.QueryRow(context.Background(), "SELECT count(*) FROM storage_gc WHERE storage_key=$1", key).Scan(&gc)
		if mails == 1 && gc == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("slow job starved mail or garbage collection lane")
}
func TestCustomOperationAndJobUseSharedAuthorization(t *testing.T) {
	old := moduleRegistry
	moduleRegistry = NewRegistry()
	t.Cleanup(func() { moduleRegistry = old })
	if e := moduleRegistry.Register(Module{Key: "inventory", Label: "Inventory", LabelEN: "Inventory", Fields: []Field{{Key: "name", Label: "Name", LabelEN: "Name", Type: "string"}, {Key: "active", Label: "Active", LabelEN: "Active", Type: "boolean"}}, Actions: []string{"read", "create", "update", "approve"}, Operations: []extension.Operation{{Key: "approve", Label: "Approve", LabelEN: "Approve", Apply: func(c extension.OperationContext) (map[string]any, error) { return map[string]any{"active": true}, nil }}}}); e != nil {
		t.Fatal(e)
	}
	if e := moduleRegistry.RegisterJob(extension.JobDefinition{Type: "inventory_count", Resource: "inventory", Action: "read", ResultFields: []Field{{Key: "count", Type: "number"}}, Run: func(c extension.JobContext) (map[string]any, error) {
		n := 0
		e := c.WalkRecords(func(r extension.Record) error { n++; return nil })
		return map[string]any{"count": float64(n)}, e
	}}); e != nil {
		t.Fatal(e)
	}
	f := newFixture(t)
	r := asMap(f.call("POST", "/resources/inventory", map[string]any{"data": map[string]any{"name": "asset", "active": false}}, 200)["data"])
	id := r["id"].(string)
	f.call("POST", "/resources/inventory/"+id+"/actions/approve", map[string]any{"version": 1, "data": map[string]any{}}, 200)
	f.call("POST", "/resources/inventory/"+id+"/actions/approve", map[string]any{"version": 1, "data": map[string]any{}}, 409)
	j := asMap(f.call("POST", "/jobs", map[string]any{"type": "inventory_count", "payload": map[string]any{}}, 200)["data"])
	f.awaitJob(j["id"].(string))
	j = asMap(f.call("GET", "/jobs/"+j["id"].(string), nil, 200)["data"])
	if asMap(j["result"])["count"] != float64(1) {
		t.Fatal("custom job result absent")
	}
}

func beginUpload(f *fixture, content string) <-chan int {
	done := make(chan int, 1)
	go func() {
		buf := &bytes.Buffer{}
		w := multipart.NewWriter(buf)
		part, _ := w.CreateFormFile("file", "gated.txt")
		part.Write([]byte(content))
		w.Close()
		r, _ := http.NewRequest("POST", f.server.URL+"/api/v1/files", buf)
		r.Header.Set("Content-Type", w.FormDataContentType())
		r.Header.Set("X-CSRF-Token", f.csrf)
		r.Header.Set("X-Tenant-ID", f.tenant)
		response, e := f.client.Do(r)
		if e != nil {
			done <- 0
			return
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		done <- response.StatusCode
	}()
	return done
}
func TestUploadPublicationRechecksQuotaAndRevocation(t *testing.T) {
	for _, scenario := range []string{"quota", "session", "role", "member"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			actor := f
			role := ""
			member := ""
			if scenario == "role" || scenario == "member" {
				_, member, role = f.addMember("uploader@example.test", []Grant{{Resource: "files", Action: "create", Scope: "self"}, {Resource: "files", Action: "read", Scope: "self"}}, "")
				actor = f.loginAs("uploader@example.test")
			}
			s := &gatedTransferStorage{Storage: f.app.storage(), started: make(chan struct{}), release: make(chan struct{})}
			f.app.setStorage(s)
			var once sync.Once
			defer once.Do(func() { close(s.release) })
			done := beginUpload(actor, "reservation")
			select {
			case <-s.started:
			case <-time.After(time.Second):
				t.Fatal("upload not started")
			}
			// Active reservations must not be reclaimed while external I/O is in flight.
			f.app.collectOneFile(context.Background())
			var active int
			f.admin.QueryRow(context.Background(), "SELECT count(*) FROM storage_gc WHERE reservation_bytes>0").Scan(&active)
			if active != 1 {
				t.Fatal("active reservation was collected")
			}
			expected := 403
			switch scenario {
			case "quota":
				f.admin.Exec(context.Background(), "UPDATE tenants SET quotas=jsonb_build_object('storage_bytes',1) WHERE id=$1", f.tenant)
				expected = 429
			case "session":
				f.admin.Exec(context.Background(), "DELETE FROM sessions WHERE user_id=$1", actor.user)
				expected = 401
			case "role":
				f.sql("UPDATE roles SET grants='[]' WHERE tenant_id=$1 AND id=$2", f.tenant, role)
			case "member":
				f.sql("UPDATE memberships SET status='suspended' WHERE tenant_id=$1 AND id=$2", f.tenant, member)
			}
			once.Do(func() { close(s.release) })
			select {
			case status := <-done:
				if status != expected {
					t.Fatalf("publication status %d, want %d", status, expected)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("publication blocked")
			}
			var n int
			f.admin.QueryRow(context.Background(), "SELECT count(*) FROM files").Scan(&n)
			if n != 0 {
				t.Fatal("revoked upload published metadata")
			}
		})
	}
}
func TestWorkloadRequiresFieldReadAuthority(t *testing.T) {
	f := newFixture(t)
	f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "hidden priority", "priority": "urgent"}}, 200)
	_, _, _ = f.addMember("title-reader@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title"}}}, "")
	reader := f.loginAs("title-reader@example.test")
	workload := asMap(asMap(reader.call("GET", "/dashboard", nil, 200)["data"])["workload"])
	if workload == nil {
		t.Fatal("missing workload")
	}
	if _, present := workload["open_tickets"]; present {
		t.Fatal("status count exposed without field authority")
	}
	if _, present := workload["urgent_tickets"]; present {
		t.Fatal("priority count exposed without field authority")
	}
	owner := asMap(asMap(f.call("GET", "/dashboard", nil, 200)["data"])["workload"])
	if owner["open_tickets"] != float64(1) || owner["urgent_tickets"] != float64(1) {
		t.Fatal("authorized workload incorrect")
	}
}

func TestDepartmentNullCannotEscapeResultingScope(t *testing.T) {
	f := newFixture(t)
	d := asMap(f.call("POST", "/departments", map[string]any{"name": "Scoped"}, 200)["data"])["id"].(string)
	user, _, _ := f.addMember("department-editor@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "department", ReadFields: []string{"title"}}, {Resource: "tickets", Action: "update", Scope: "department", WriteFields: []string{"title"}}, {Resource: "tickets", Action: "assign", Scope: "department"}}, d)
	record := asMap(f.call("POST", "/resources/tickets", map[string]any{"owner_id": user, "department_id": d, "data": map[string]any{"title": "scoped"}}, 200)["data"])
	editor := f.loginAs("department-editor@example.test")
	editor.call("PATCH", "/resources/tickets/"+record["id"].(string), map[string]any{"version": 1, "department_id": nil, "data": map[string]any{}}, 403)
}
func TestConcurrentControlMutationsDoNotUpgradeTenantLocks(t *testing.T) {
	f := newFixture(t)
	d := asMap(f.call("POST", "/departments", map[string]any{"name": "Before"}, 200)["data"])["id"].(string)
	_, member, _ := f.addMember("concurrent-member@example.test", []Grant{}, "")
	start := make(chan struct{})
	done := make(chan int, 2)
	for _, item := range []struct{ path, body string }{{"/departments/" + d, `{"name":"After"}`}, {"/members/" + member, `{"position":"Engineer"}`}} {
		go func() {
			<-start
			r, _ := http.NewRequest("PATCH", f.server.URL+"/api/v1"+item.path, bytes.NewBufferString(item.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-CSRF-Token", f.csrf)
			r.Header.Set("X-Tenant-ID", f.tenant)
			client := &http.Client{Jar: f.client.Jar, Timeout: 2 * time.Second}
			response, e := client.Do(r)
			if e != nil {
				done <- 0
				return
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			done <- response.StatusCode
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		select {
		case status := <-done:
			if status != 200 {
				t.Fatalf("concurrent control mutation status %d", status)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("control mutation deadlocked")
		}
	}
}

func TestCompletedJobResultIsWithheldAfterAuthorityNarrows(t *testing.T) {
	old := moduleRegistry
	moduleRegistry = NewRegistry()
	t.Cleanup(func() { moduleRegistry = old })
	if e := moduleRegistry.RegisterJob(extension.JobDefinition{Type: "ticket_count", Resource: "tickets", Action: "read", ResultFields: []Field{{Key: "count", Type: "number"}}, Run: func(c extension.JobContext) (map[string]any, error) {
		n := 0
		e := c.WalkRecords(func(Record) error { n++; return nil })
		return map[string]any{"count": float64(n)}, e
	}}); e != nil {
		t.Fatal(e)
	}
	f := newFixture(t)
	_, _, role := f.addMember("job-reader@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title"}}, {Resource: "jobs", Action: "create", Scope: "self"}, {Resource: "jobs", Action: "read", Scope: "self"}}, "")
	f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "foreign owner"}}, 200)
	actor := f.loginAs("job-reader@example.test")
	job := asMap(actor.call("POST", "/jobs", map[string]any{"type": "ticket_count", "payload": map[string]any{}}, 200)["data"])
	id := job["id"].(string)
	f.awaitJob(id)
	before := asMap(actor.call("GET", "/jobs/"+id, nil, 200)["data"])
	if asMap(before["result"])["count"] != float64(1) {
		t.Fatal("authorized result missing")
	}
	f.call("PATCH", "/roles/"+role, map[string]any{"name": "narrowed", "grants": []Grant{{Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}, {Resource: "jobs", Action: "read", Scope: "self"}}}, 200)
	detail := asMap(actor.call("GET", "/jobs/"+id, nil, 200)["data"])
	if _, ok := detail["result"]; ok {
		t.Fatal("detail disclosed stale aggregate")
	}
	list := actor.call("GET", "/jobs", nil, 200)["data"].([]any)
	for _, item := range list {
		row := asMap(item)
		for _, key := range []string{"result", "authority_hash", "authority_resource", "authority_action", "actor_id", "department_id"} {
			if _, ok := row[key]; ok {
				t.Fatalf("list disclosed %s", key)
			}
		}
	}
}

func TestConcurrentUploadReservationsCannotExceedQuota(t *testing.T) {
	f := newFixture(t)
	f.call("PATCH", "/platform/tenants/"+f.tenant, map[string]any{"quotas": map[string]int{"storage_bytes": 5}}, 200)
	s := &gatedTransferStorage{Storage: f.app.storage(), started: make(chan struct{}), release: make(chan struct{})}
	f.app.setStorage(s)
	var once sync.Once
	defer once.Do(func() { close(s.release) })
	first := beginUpload(f, "12345")
	select {
	case <-s.started:
	case <-time.After(time.Second):
		t.Fatal("first reservation missing")
	}
	second := beginUpload(f, "67890")
	select {
	case status := <-second:
		if status != 429 {
			t.Fatalf("concurrent reservation status %d", status)
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent reservation blocked behind external I/O")
	}
	once.Do(func() { close(s.release) })
	select {
	case status := <-first:
		if status != 200 {
			t.Fatalf("reservation conversion status %d", status)
		}
	case <-time.After(time.Second):
		t.Fatal("publication blocked")
	}
	var bytes int64
	f.admin.QueryRow(context.Background(), "SELECT (SELECT coalesce(sum(size),0) FROM files)+(SELECT coalesce(sum(reservation_bytes),0) FROM storage_gc)").Scan(&bytes)
	if bytes != 5 {
		t.Fatalf("reservation converted incorrectly: %d", bytes)
	}
}

type tailGateReader struct {
	data             []byte
	offset           int
	started, release chan struct{}
	once             sync.Once
}

func (r *tailGateReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	gate := len(r.data) - 128
	if r.offset >= gate {
		r.once.Do(func() { close(r.started); <-r.release })
	}
	limit := len(r.data)
	if r.offset < gate {
		limit = gate
	}
	n := copy(p, r.data[r.offset:limit])
	r.offset += n
	return n, nil
}
func TestMultipartSpillIsRemovedWhenReauthorizationFails(t *testing.T) {
	f := newFixture(t)
	spill := t.TempDir()
	t.Setenv("TMPDIR", spill)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "spill.txt")
	part.Write(bytes.Repeat([]byte("x"), 2<<20))
	writer.Close()
	reader := &tailGateReader{data: body.Bytes(), started: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	defer once.Do(func() { close(reader.release) })
	done := make(chan int, 1)
	go func() {
		r, _ := http.NewRequest("POST", f.server.URL+"/api/v1/files", reader)
		r.Header.Set("Content-Type", writer.FormDataContentType())
		r.Header.Set("X-CSRF-Token", f.csrf)
		r.Header.Set("X-Tenant-ID", f.tenant)
		response, e := f.client.Do(r)
		if e != nil {
			done <- 0
			return
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		done <- response.StatusCode
	}()
	<-reader.started
	deadline := time.Now().Add(time.Second)
	found := false
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(spill)
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "multipart-") {
				found = true
			}
		}
		if found {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !found {
		t.Fatal("test did not spill multipart data")
	}
	f.admin.Exec(context.Background(), "DELETE FROM sessions WHERE user_id=$1", f.user)
	once.Do(func() { close(reader.release) })
	select {
	case status := <-done:
		if status != 401 {
			t.Fatalf("revoked body status %d", status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("body reauthorization blocked")
	}
	entries, _ := os.ReadDir(spill)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "multipart-") {
			t.Fatal("denied multipart request leaked temporary file")
		}
	}
}
func TestMetricsUsePrivateRegistryAndBoundedRouteLabels(t *testing.T) {
	f := newFixture(t)
	record := asMap(f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "metrics"}}, 200)["data"])
	id := record["id"].(string)
	f.call("GET", "/resources/tickets/"+id, nil, 200)
	response, e := f.client.Get(f.server.URL + "/metrics")
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if !bytes.Contains(body, []byte("tenantforge_http_request_duration_seconds_bucket")) {
		t.Fatal("missing Prometheus histogram")
	}
	if bytes.Contains(body, []byte(id)) {
		t.Fatal("record id used as metric label")
	}
	if bytes.Contains(body, []byte("go_goroutines")) {
		t.Fatal("global collectors leaked into private registry")
	}
	// A second independent registry must never panic with duplicate collectors.
	_ = newMetrics(&App{})
}

func TestRunningJobRejectsNarrowedAuthorityAtPublication(t *testing.T) {
	old := moduleRegistry
	moduleRegistry = NewRegistry()
	t.Cleanup(func() { moduleRegistry = old })
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	if e := moduleRegistry.RegisterJob(extension.JobDefinition{Type: "gated_ticket_count", Resource: "tickets", Action: "read", ResultFields: []Field{{Key: "count", Type: "number"}}, Run: func(c extension.JobContext) (map[string]any, error) {
		n := 0
		e := c.WalkRecords(func(Record) error { n++; return nil })
		close(started)
		select {
		case <-release:
		case <-c.Context().Done():
			return nil, c.Context().Err()
		}
		return map[string]any{"count": float64(n)}, e
	}}); e != nil {
		t.Fatal(e)
	}
	f := newFixture(t)
	_, _, role := f.addMember("gated-reader@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title", "priority"}}, {Resource: "jobs", Action: "create", Scope: "self"}, {Resource: "jobs", Action: "read", Scope: "self"}}, "")
	f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "other owner"}}, 200)
	actor := f.loginAs("gated-reader@example.test")
	job := asMap(actor.call("POST", "/jobs", map[string]any{"type": "gated_ticket_count", "payload": map[string]any{}}, 200)["data"])
	id := job["id"].(string)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("job handler did not reach gate")
	}
	f.call("PATCH", "/roles/"+role, map[string]any{"name": "narrowed during handler", "grants": []Grant{{Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}, {Resource: "jobs", Action: "read", Scope: "self"}}}, 200)
	once.Do(func() { close(release) })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		row := asMap(actor.call("GET", "/jobs/"+id, nil, 200)["data"])
		if row["status"] == "failed" {
			if row["error_code"] != "permission_revoked" || row["error_stage"] != "authorize" {
				t.Fatal("missing safe publication diagnostic")
			}
			if len(asMap(row["result"])) != 0 {
				t.Fatal("unauthorized aggregate published")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("narrowed authority job did not fail")
}

func asyncJSON(f *fixture, method, path string, body map[string]any) <-chan int {
	done := make(chan int, 1)
	go func() {
		r, _ := http.NewRequest(method, f.server.URL+"/api/v1"+path, bytes.NewReader(jsonBytes(body)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", f.csrf)
		r.Header.Set("X-Tenant-ID", f.tenant)
		response, e := f.client.Do(r)
		if e != nil {
			done <- 0
			return
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		done <- response.StatusCode
	}()
	return done
}
func TestDepartmentTreeRevocationIsFencedWithRecordPublication(t *testing.T) {
	f := newFixture(t)
	a := asMap(f.call("POST", "/departments", map[string]any{"name": "A"}, 200)["data"])["id"].(string)
	b := asMap(f.call("POST", "/departments", map[string]any{"name": "B", "parent_id": a}, 200)["data"])["id"].(string)
	user, _, _ := f.addMember("tree-editor@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "department_tree", ReadFields: []string{"title"}}, {Resource: "tickets", Action: "update", Scope: "department_tree", WriteFields: []string{"title"}}}, a)
	row := asMap(f.call("POST", "/resources/tickets", map[string]any{"owner_id": user, "department_id": b, "data": map[string]any{"title": "before"}}, 200)["data"])
	id := row["id"].(string)
	editor := f.loginAs("tree-editor@example.test")
	lock, e := f.admin.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Rollback(context.Background())
	if _, e = lock.Exec(context.Background(), "SELECT id FROM resources WHERE id=$1 FOR UPDATE", id); e != nil {
		t.Fatal(e)
	}
	patch := asyncJSON(editor, "PATCH", "/resources/tickets/"+id, map[string]any{"version": 1, "data": map[string]any{"title": "authorized before move"}})
	deadline := time.Now().Add(time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		f.admin.QueryRow(context.Background(), "SELECT EXISTS(SELECT FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE '%FROM resources%' AND query LIKE '%FOR UPDATE%')").Scan(&blocked)
		if blocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("record update did not reach deterministic lock gate")
	}
	move := asyncJSON(f, "PATCH", "/departments/"+b, map[string]any{"parent_id": nil})
	select {
	case <-move:
		t.Fatal("hierarchy changed before record's authority fence released")
	case <-time.After(50 * time.Millisecond):
	}
	lock.Commit(context.Background())
	for _, done := range []<-chan int{patch, move} {
		select {
		case status := <-done:
			if status != 200 {
				t.Fatalf("ordered mutation status %d", status)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("hierarchy fence deadlocked")
		}
	}
	editor.call("PATCH", "/resources/tickets/"+id, map[string]any{"version": 2, "data": map[string]any{"title": "must now deny"}}, 403)
}

func TestCustomJobUpdatesAreIdempotentAndPayloadCannotCrossTenants(t *testing.T) {
	old := moduleRegistry
	moduleRegistry = NewRegistry()
	t.Cleanup(func() { moduleRegistry = old })
	if e := moduleRegistry.RegisterJob(extension.JobDefinition{Type: "ticket_rename", Resource: "tickets", Action: "update", ResultFields: []Field{{Key: "updated", Type: "number"}}, Run: func(c extension.JobContext) (map[string]any, error) {
		var payload struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(c.Payload(), &payload) != nil {
			return nil, &extension.Error{Code: "validation_failed", Permanent: true}
		}
		for i := 0; i < 2; i++ {
			if _, e := c.UpdateRecord(payload.ID, 1, map[string]any{"title": "job update"}); e != nil {
				return nil, &extension.Error{Code: "permission_revoked", Permanent: true}
			}
		}
		return map[string]any{"updated": float64(1)}, nil
	}}); e != nil {
		t.Fatal(e)
	}
	f := newFixture(t)
	user, _, _ := f.addMember("job-editor@example.test", []Grant{{Resource: "tickets", Action: "update", Scope: "self", WriteFields: []string{"title"}}, {Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}, {Resource: "jobs", Action: "create", Scope: "self"}, {Resource: "jobs", Action: "read", Scope: "self"}}, "")
	record := asMap(f.call("POST", "/resources/tickets", map[string]any{"owner_id": user, "data": map[string]any{"title": "initial"}}, 200)["data"])
	id := record["id"].(string)
	actor := f.loginAs("job-editor@example.test")
	job := asMap(actor.call("POST", "/jobs", map[string]any{"type": "ticket_rename", "payload": map[string]any{"id": id, "owner_id": f.user, "department_id": "11111111-1111-4111-8111-111111111111"}}, 200)["data"])
	f.awaitJob(job["id"].(string))
	changed := asMap(actor.call("GET", "/resources/tickets/"+id, nil, 200)["data"])
	if changed["version"] != float64(2) {
		t.Fatal("repeated stable job effect incremented version more than once")
	}
	original := f.tenant
	other := asMap(f.call("POST", "/platform/tenants", map[string]any{"name": "Foreign", "owner_email": "owner@example.test"}, 200)["data"])["id"].(string)
	f.tenant = other
	foreign := asMap(f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "foreign"}}, 200)["data"])
	f.tenant = original
	denied := asMap(actor.call("POST", "/jobs", map[string]any{"type": "ticket_rename", "payload": map[string]any{"id": foreign["id"], "tenant_id": other}}, 200)["data"])
	deniedID := denied["id"].(string)
	deadline := time.Now().Add(2 * time.Second)
	failed := false
	for time.Now().Before(deadline) {
		row := asMap(actor.call("GET", "/jobs/"+deniedID, nil, 200)["data"])
		if row["status"] == "failed" {
			failed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !failed {
		t.Fatal("foreign job target was not rejected")
	}
	f.tenant = other
	unchanged := asMap(f.call("GET", "/resources/tickets/"+foreign["id"].(string), nil, 200)["data"])
	if unchanged["version"] != float64(1) {
		t.Fatal("job payload crossed tenant boundary")
	}
}

type gatedReadBody struct {
	io.ReadCloser
	started, release chan struct{}
	once             sync.Once
	ctx              context.Context
}

func (b *gatedReadBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.release:
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
	return b.ReadCloser.Read(p)
}

type gatedDownloadStorage struct {
	Storage
	started, release chan struct{}
}

func (s *gatedDownloadStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	body, e := s.Storage.Open(ctx, key)
	if e != nil {
		return nil, e
	}
	return &gatedReadBody{ReadCloser: body, ctx: ctx, started: s.started, release: s.release}, nil
}
func TestSlowDownloadReleasesPoolBeforeResponseCopy(t *testing.T) {
	f := newFixtureWithConfig(t, func(c *Config) { c.PoolConfig.MaxConns = 1 })
	file := asMap(f.multipart("/files", "read.txt", "slow response body", nil, 200)["data"])
	storage := &gatedDownloadStorage{Storage: f.app.storage(), started: make(chan struct{}), release: make(chan struct{})}
	f.app.setStorage(storage)
	var once sync.Once
	defer once.Do(func() { close(storage.release) })
	done := make(chan int, 1)
	go func() {
		request, _ := http.NewRequest("GET", f.server.URL+"/api/v1/files/"+file["id"].(string)+"/download", nil)
		request.Header.Set("X-Tenant-ID", f.tenant)
		response, e := f.client.Do(request)
		if e != nil {
			done <- 0
			return
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		done <- response.StatusCode
	}()
	select {
	case <-storage.started:
	case <-time.After(time.Second):
		t.Fatal("download did not reach response copy")
	}
	mutation := asyncJSON(f, "POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "while downloading"}})
	select {
	case status := <-mutation:
		if status != 200 {
			t.Fatal("unrelated mutation denied")
		}
	case <-time.After(time.Second):
		t.Fatal("response copy retained pool connection")
	}
	once.Do(func() { close(storage.release) })
	select {
	case status := <-done:
		if status != 200 {
			t.Fatal("download did not complete")
		}
	case <-time.After(time.Second):
		t.Fatal("download blocked")
	}
}
func TestRejectedMetricCountsEarlyAuthenticationAndCSRF(t *testing.T) {
	f := newFixture(t)
	before := f.app.failures.Load()
	request, _ := http.NewRequest("GET", f.server.URL+"/api/v1/resources/tickets", nil)
	request.Header.Set("X-Tenant-ID", f.tenant)
	response, e := http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("expected unauthenticated request")
	}
	request, _ = http.NewRequest("POST", f.server.URL+"/api/v1/resources/tickets", bytes.NewBufferString(`{"data":{"title":"denied"}}`))
	request.Header.Set("X-Tenant-ID", f.tenant)
	request.Header.Set("X-CSRF-Token", "incorrect")
	response, e = f.client.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("expected invalid CSRF denial")
	}
	if f.app.failures.Load()-before != 2 {
		t.Fatal("early rejections were not counted exactly once")
	}
}

func TestExportPublicationRechecksLoweredStorageQuota(t *testing.T) {
	f := newFixture(t)
	f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "reserved export"}}, 200)
	storage := &gatedTransferStorage{Storage: f.app.storage(), started: make(chan struct{}), release: make(chan struct{})}
	f.app.setStorage(storage)
	var once sync.Once
	defer once.Do(func() { close(storage.release) })
	job := asMap(f.call("POST", "/resources/tickets/export", map[string]any{"columns": []string{"title"}}, 200)["data"])
	id := job["id"].(string)
	select {
	case <-storage.started:
	case <-time.After(2 * time.Second):
		t.Fatal("export did not reach storage")
	}
	f.call("PATCH", "/platform/tenants/"+f.tenant, map[string]any{"quotas": map[string]int{"storage_bytes": 1}}, 200)
	once.Do(func() { close(storage.release) })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		row := asMap(f.call("GET", "/jobs/"+id, nil, 200)["data"])
		if row["error_code"] == "quota_exceeded" {
			if row["status"] == "succeeded" {
				t.Fatal("export published above lowered quota")
			}
			var size int64
			var key string
			f.admin.QueryRow(context.Background(), "SELECT result_bytes,result_key FROM jobs WHERE id=$1", id).Scan(&size, &key)
			if size != 0 || key != "" {
				t.Fatal("rejected export retained artifact metadata")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("lowered export quota not rechecked")
}

func TestCancelledJobCannotPublishRecordAfterWaitingForRowLock(t *testing.T) {
	f := newFixture(t)
	row := asMap(f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "before cancellation"}}, 200)["data"])
	recordID := row["id"].(string)
	jobID := uuid.NewString()
	claim := digest(randomToken())
	f.sql("INSERT INTO jobs(id,tenant_id,actor_id,type,status) VALUES($1,$2,$3,'export','running')", jobID, f.tenant, f.user)
	f.sql("INSERT INTO job_leases(job_id,tenant_id,claim_token,lease_until) VALUES($1,$2,$3,now()+interval '1 minute')", jobID, f.tenant, claim)
	lock, e := f.admin.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Rollback(context.Background())
	lock.Exec(context.Background(), "SELECT id FROM resources WHERE id=$1 FOR UPDATE", recordID)
	service := &jobService{app: f.app, ctx: context.Background(), id: jobID, tenant: f.tenant, actor: f.user, claim: claim, resource: "tickets", action: "update"}
	done := make(chan error, 1)
	go func() {
		_, e := service.UpdateRecord(recordID, 1, map[string]any{"title": "must not publish"})
		done <- e
	}()
	deadline := time.Now().Add(time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		f.admin.QueryRow(context.Background(), "SELECT EXISTS(SELECT FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE '%FROM resources%' AND query LIKE '%FOR UPDATE%')").Scan(&blocked)
		if blocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("record update did not reach lock gate")
	}
	f.call("POST", "/jobs/"+jobID+"/cancel", map[string]any{}, 200)
	lock.Commit(context.Background())
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancelled job wrote after cancellation committed")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled record publication blocked")
	}
	after := asMap(f.call("GET", "/resources/tickets/"+recordID, nil, 200)["data"])
	if after["version"] != float64(1) {
		t.Fatal("cancelled job incremented record version")
	}
}
