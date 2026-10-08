package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"strings"
	"testing"
)

func supportTicket(f *fixture) (string, string) {
	f.t.Helper()
	r := asMap(f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "Support", "description": "initial", "internal_notes": "legacy", "customer_name": "Customer"}}, 200)["data"])
	id := r["id"].(string)
	return id, "/resources/tickets/" + id
}
func TestSupportResolutionHTTPAndHistory(t *testing.T) {
	f := newFixture(t)
	_, path := supportTicket(f)
	f.call("POST", path+"/transition", map[string]any{"version": 1, "status": "in_progress"}, 200)
	f.call("PATCH", path, map[string]any{"version": 2, "data": map[string]any{"status": "resolved"}}, 422)
	f.call("POST", path+"/transition", map[string]any{"version": 2, "status": "resolved", "resolution": "  "}, 422)
	f.call("POST", path+"/transition", map[string]any{"version": 2, "status": "resolved", "resolution": "Fixed"}, 200)
	f.call("POST", path+"/transition", map[string]any{"version": 3, "status": "in_progress"}, 200)
	f.call("POST", path+"/transition", map[string]any{"version": 4, "status": "resolved"}, 422)
	activity := f.call("GET", path+"/activity?page_size=2", nil, 200)
	if asMap(activity["meta"])["total"] != float64(4) || len(activity["data"].([]any)) != 2 {
		t.Fatal("history count/paging mismatch")
	}
	events := activity["data"].([]any)
	if asMap(events[1])["body"] != "Fixed" || asMap(events[0])["to_status"] != "in_progress" {
		t.Fatal("resolution history missing")
	}
}
func TestSupportReplyPermissionsVersionsAndClosed(t *testing.T) {
	f := newFixture(t)
	_, path := supportTicket(f)
	for _, body := range []string{" \n", strings.Repeat("x", 4001), strings.Repeat("界", 3000)} {
		f.call("POST", path+"/replies", map[string]any{"version": 1, "kind": "reply", "body": body}, 422)
	}
	f.call("POST", path+"/replies", map[string]any{"version": 1, "kind": "other", "body": "x"}, 422)
	f.call("POST", path+"/replies", map[string]any{"version": 1, "kind": "reply", "body": "x", "unknown": true}, 422)
	reply := asMap(f.call("POST", path+"/replies", map[string]any{"version": 1, "kind": "reply", "body": "Conversation"}, 200)["data"])
	if asMap(reply["record"])["version"] != float64(2) || asMap(reply["event"])["body"] != "Conversation" {
		t.Fatal("reply return mismatch")
	}
	f.call("POST", path+"/replies", map[string]any{"version": 1, "kind": "reply", "body": "duplicate"}, 409)
	f.call("POST", path+"/replies", map[string]any{"version": 2, "kind": "note", "body": "Private"}, 200)
	r := asMap(f.call("GET", path, nil, 200)["data"])
	if asMap(r["data"])["description"] != "initial" || asMap(r["data"])["internal_notes"] != "legacy" {
		t.Fatal("reply rewrote original data")
	}
	gs := []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title", "status", "description", "internal_notes"}}, {Resource: "tickets", Action: "reply", Scope: "tenant", WriteFields: []string{"description", "internal_notes"}}}
	_, _, role := f.addMember("reply@example.test", gs, "")
	m := f.loginAs("reply@example.test")
	activity := m.call("GET", path+"/activity", nil, 200)
	if asMap(activity["meta"])["total"] != float64(2) {
		t.Fatal("masked note contributed to history/count")
	}
	m.call("POST", path+"/replies", map[string]any{"version": 3, "kind": "note", "body": "denied"}, 403)
	m.call("POST", path+"/replies", map[string]any{"version": 3, "kind": "reply", "body": "allowed"}, 200)
	f.call("PATCH", "/roles/"+role, map[string]any{"name": "Revoked description", "grants": []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title"}}, {Resource: "tickets", Action: "reply", Scope: "tenant", WriteFields: []string{"description"}}}}, 200)
	m.call("POST", path+"/replies", map[string]any{"version": 4, "kind": "reply", "body": "denied"}, 403)
	activity = m.call("GET", path+"/activity", nil, 200)
	if asMap(activity["meta"])["total"] != float64(1) {
		t.Fatal("revoked replies visible")
	}
	f.call("POST", path+"/transition", map[string]any{"version": 4, "status": "in_progress"}, 200)
	f.call("POST", path+"/transition", map[string]any{"version": 5, "status": "resolved", "resolution": "done"}, 200)
	f.call("POST", path+"/transition", map[string]any{"version": 6, "status": "closed"}, 200)
	f.call("POST", path+"/replies", map[string]any{"version": 7, "kind": "reply", "body": "closed"}, 422)
}
func TestSupportAssignmentWithoutUpdateAndQueues(t *testing.T) {
	f := newFixture(t)
	_, path := supportTicket(f)
	dep := asMap(f.call("POST", "/departments", map[string]any{"name": "Support"}, 200)["data"])["id"].(string)
	target, _, _ := f.addMember("agent@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"*"}}}, dep)
	_, _, _ = f.addMember("assign@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title"}}, {Resource: "tickets", Action: "assign", Scope: "tenant"}}, "")
	m := f.loginAs("assign@example.test")
	result := asMap(m.call("POST", path+"/assignment", map[string]any{"version": 1, "owner_id": target}, 200)["data"])
	if result["owner_id"] != target || result["department_id"] != dep || result["version"] != float64(2) {
		t.Fatal("assignment/default department mismatch")
	}
	m.call("POST", path+"/assignment", map[string]any{"version": 1, "owner_id": target}, 409)
	m.call("POST", path+"/assignment", map[string]any{"version": 2, "owner_id": target, "department_id": nil}, 200)
	for _, queue := range []string{"mine", "active", "resolved", "closed", "all"} {
		result = f.call("GET", "/resources/tickets?queue="+queue+"&page_size=1", nil, 200)
		want := float64(1)
		if queue == "mine" || queue == "resolved" || queue == "closed" {
			want = 0
		}
		if asMap(result["meta"])["total"] != want {
			t.Fatalf("queue %s incorrect", queue)
		}
	}
	f.call("GET", "/resources/tickets?queue=invalid", nil, 422)
	f.call("GET", "/resources/settings?queue=all", nil, 422)
	agent := f.loginAs("agent@example.test")
	if asMap(agent.call("GET", "/resources/tickets?queue=mine", nil, 200)["meta"])["total"] != float64(1) {
		t.Fatal("mine omitted assigned record")
	}
}
func TestSupportHistoryAuditRollbackAndImport(t *testing.T) {
	f := newFixture(t)
	id, path := supportTicket(f)
	if _, e := f.admin.Exec(context.Background(), `CREATE FUNCTION support_reject_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.resource='tickets' AND NEW.action='reply' THEN RAISE EXCEPTION 'test failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER support_reject_audit BEFORE INSERT ON audit FOR EACH ROW EXECUTE FUNCTION support_reject_audit()`); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		f.admin.Exec(context.Background(), `DROP TRIGGER IF EXISTS support_reject_audit ON audit; DROP FUNCTION IF EXISTS support_reject_audit()`)
	})
	f.call("POST", path+"/replies", map[string]any{"version": 1, "kind": "reply", "body": "rollback"}, 500)
	r := asMap(f.call("GET", path, nil, 200)["data"])
	if r["version"] != float64(1) {
		t.Fatal("failed audit committed parent")
	}
	activity := f.call("GET", path+"/activity", nil, 200)
	if asMap(activity["meta"])["total"] != float64(1) {
		t.Fatal("failed audit committed history")
	}
	f.multipart("/resources/tickets/import", "batch.csv", "title\nimported\n", nil, 200)
	var count int
	if e := f.admin.QueryRow(context.Background(), "SELECT count(*) FROM ticket_events WHERE ticket_id<>$1", id).Scan(&count); e != nil || count != 1 {
		t.Fatal("import omitted create event", e)
	}
}

func TestSupportHistoryCurrentScopeFieldMetadataAndTransfer(t *testing.T) {
	f := newFixture(t)
	id, path := supportTicket(f)
	user, member, role := f.addMember("scoped@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}}, "")
	f.call("PATCH", path, map[string]any{"version": 1, "owner_id": user, "data": map[string]any{"title": "changed", "customer_email": "must-hide@example.test"}}, 200)
	f.call("POST", path+"/replies", map[string]any{"version": 2, "kind": "note", "body": "secret note"}, 200)
	f.call("POST", path+"/transition", map[string]any{"version": 3, "status": "in_progress"}, 200)
	f.call("POST", path+"/transition", map[string]any{"version": 4, "status": "resolved", "resolution": "secret resolution"}, 200)
	m := f.loginAs("scoped@example.test")
	activity := m.call("GET", path+"/activity", nil, 200)
	if asMap(activity["meta"])["total"] != float64(5) {
		t.Fatal("hidden note count mismatch")
	}
	for _, raw := range activity["data"].([]any) {
		event := asMap(raw)
		if event["body"] != nil || event["from_status"] != nil || event["to_status"] != nil {
			t.Fatal("system event leaked hidden fields")
		}
		if fields, ok := event["fields"].([]any); ok {
			for _, field := range fields {
				if field != "title" {
					t.Fatal("event leaked unreadable field name")
				}
			}
		}
	}
	for _, queue := range []string{"mine", "active", "resolved", "closed"} {
		if asMap(m.call("GET", "/resources/tickets?queue="+queue, nil, 200)["meta"])["total"] != float64(0) {
			t.Fatal("queue leaked unreadable status")
		}
	}
	if asMap(m.call("GET", "/resources/tickets?queue=all", nil, 200)["meta"])["total"] != float64(1) {
		t.Fatal("all lost readable record")
	}
	f.call("POST", "/members/"+member+"/transfer", map[string]any{"owner_id": f.user}, 200)
	m.call("GET", path+"/activity", nil, 404)
	var count int
	if e := f.admin.QueryRow(context.Background(), "SELECT count(*) FROM ticket_events WHERE ticket_id=$1 AND kind='assigned'", id).Scan(&count); e != nil || count != 2 {
		t.Fatal("member transfer omitted activity", e)
	}
	f.call("PATCH", "/roles/"+role, map[string]any{"name": "Revoked", "grants": []Grant{}}, 200)
	m.call("GET", path+"/activity", nil, 404)
	other := uuid.NewString()
	if _, e := f.admin.Exec(context.Background(), "INSERT INTO tenants(id,name,slug) VALUES($1,'Other',$2)", other, other); e != nil {
		t.Fatal(e)
	}
	if _, e := f.admin.Exec(context.Background(), "INSERT INTO memberships(id,tenant_id,user_id) VALUES($1,$2,$3)", uuid.NewString(), other, user); e != nil {
		t.Fatal(e)
	}
	m.tenant = other
	m.call("GET", path+"/activity", nil, 404)
	f.call("GET", "/resources/settings/"+id+"/activity", nil, 404)
}
func TestSupportEventsForcedRLSReferencesAndAppendOnly(t *testing.T) {
	f := newFixture(t)
	id, path := supportTicket(f)
	other, foreignActor, foreignTicket := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, op := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO tenants(id,name,slug) VALUES($1::uuid,'Other',$1::uuid::text)", []any{other}},
		{"INSERT INTO users(id,email,display_name,username) VALUES($1,$2,'Foreign',$3)", []any{foreignActor, foreignActor + "@example.test", userUsername(foreignActor)}},
		{"INSERT INTO memberships(id,tenant_id,user_id) VALUES($1,$2,$3)", []any{uuid.NewString(), other, foreignActor}},
		{"INSERT INTO resources(id,tenant_id,module,owner_id,data) VALUES($1,$2,'tickets',$3,'{\"title\":\"Foreign\"}')", []any{foreignTicket, other, foreignActor}},
	} {
		if _, e := f.admin.Exec(context.Background(), op.sql, op.args...); e != nil {
			t.Fatal(e)
		}
	}
	checkReject := func(query string, args []any, code string) {
		t.Helper()
		tx, e := f.app.DB.Begin(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(context.Background())
		if _, e = tx.Exec(context.Background(), "SELECT set_config('app.tenant_id',$1,true)", f.tenant); e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(context.Background(), query, args...)
		var pg *pgconn.PgError
		if !errors.As(e, &pg) || pg.Code != code {
			t.Fatalf("expected database guard %s", code)
		}
	}
	insert := `INSERT INTO ticket_events(id,tenant_id,ticket_id,kind,actor_id) VALUES($1,$2,$3,'created',$4)`
	checkReject(insert, []any{uuid.NewString(), f.tenant, foreignTicket, f.user}, "23503")
	checkReject(insert, []any{uuid.NewString(), f.tenant, id, foreignActor}, "23503")
	settings := asMap(f.call("POST", "/resources/settings", map[string]any{"data": map[string]any{"key": "a"}}, 200)["data"])["id"].(string)
	checkReject(insert, []any{uuid.NewString(), f.tenant, settings, f.user}, "23503")
	checkReject(insert, []any{uuid.NewString(), other, foreignTicket, foreignActor}, "42501")
	checkReject("UPDATE ticket_events SET body='overwrite' WHERE ticket_id=$1", []any{id}, "42501")
	checkReject("DELETE FROM ticket_events WHERE ticket_id=$1", []any{id}, "42501")
	var forced bool
	if e := f.admin.QueryRow(context.Background(), "SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='ticket_events'::regclass").Scan(&forced); e != nil || !forced {
		t.Fatal("history RLS not forced", e)
	}
	tx, e := f.app.DB.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(context.Background(), "SELECT set_config('app.tenant_id',$1,true)", other); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = tx.QueryRow(context.Background(), "SELECT count(*) FROM ticket_events WHERE ticket_id=$1", id).Scan(&count); e != nil || count != 0 {
		t.Fatal("cross tenant events visible", e)
	}
	actor, member, _ := f.addMember("historical@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"description"}}, {Resource: "tickets", Action: "reply", Scope: "tenant", WriteFields: []string{"description"}}}, "")
	m := f.loginAs("historical@example.test")
	m.call("POST", path+"/replies", map[string]any{"version": 1, "kind": "reply", "body": "retained"}, 200)
	f.sql("DELETE FROM member_roles WHERE member_id=$1", member)
	f.sql("DELETE FROM memberships WHERE tenant_id=$1 AND user_id=$2", f.tenant, actor)
	activity := f.call("GET", path+"/activity", nil, 200)
	event := asMap(activity["data"].([]any)[0])
	if event["actor_id"] != nil || event["actor_name"] != "historical@example.test" || event["body"] != "retained" {
		t.Fatal("actor deletion lost historical event")
	}
}
func TestSupportConcurrentRepliesCommitOnce(t *testing.T) {
	f := newFixture(t)
	_, path := supportTicket(f)
	results := make(chan int, 2)
	for range 2 {
		go func() {
			body, _ := json.Marshal(map[string]any{"version": 1, "kind": "reply", "body": "once"})
			resp := f.request("POST", path+"/replies", bytes.NewReader(body), "application/json")
			resp.Body.Close()
			results <- resp.StatusCode
		}()
	}
	a, b := <-results, <-results
	if !((a == http.StatusOK && b == http.StatusConflict) || (b == http.StatusOK && a == http.StatusConflict)) {
		t.Fatalf("concurrent statuses %d/%d", a, b)
	}
	activity := f.call("GET", path+"/activity", nil, 200)
	if asMap(activity["meta"])["total"] != float64(2) {
		t.Fatal("concurrent retry duplicated history")
	}
}
func TestSupportJobCommonWriteHistoryAndIdempotency(t *testing.T) {
	f := newFixture(t)
	id, path := supportTicket(f)
	job := uuid.NewString()
	claim := uuid.NewString()
	f.sql("INSERT INTO jobs(id,tenant_id,actor_id,type,status) VALUES($1,$2,$3,'ticket_rename','running')", job, f.tenant, f.user)
	f.sql("INSERT INTO job_leases(job_id,tenant_id,claim_token,lease_until) VALUES($1,$2,$3,now()+interval '1 hour')", job, f.tenant, claim)
	s := &jobService{app: f.app, ctx: context.Background(), id: job, tenant: f.tenant, actor: f.user, claim: claim, resource: "tickets", action: "update"}
	for range 2 {
		if _, e := s.UpdateRecord(id, 1, map[string]any{"title": "Job change"}); e != nil {
			t.Fatal(e)
		}
	}
	activity := f.call("GET", path+"/activity", nil, 200)
	if asMap(activity["meta"])["total"] != float64(2) || asMap(activity["data"].([]any)[0])["kind"] != "updated" {
		t.Fatal("job history absent or duplicated")
	}
}

func TestSupportActivityFencesParentVisibilityUntilReadCompletes(t *testing.T) {
	f := newFixture(t)
	id, path := supportTicket(f)
	tx, e := f.app.DB.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(context.Background(), "SELECT set_config('app.tenant_id',$1,true)", f.tenant); e != nil {
		t.Fatal(e)
	}
	r, _ := http.NewRequest("GET", "http://fixture/api/v1"+path+"/activity", nil)
	r.SetPathValue("module", "tickets")
	r.SetPathValue("id", id)
	q := &request{app: f.app, tx: tx, r: r, p: Principal{ID: f.user, TenantID: f.tenant, Grants: ownerGrants()}}
	if _, e = q.ticketParent(false); e != nil {
		t.Fatal(e)
	}
	competitor, e := f.admin.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer competitor.Rollback(context.Background())
	if _, e = competitor.Exec(context.Background(), "SET LOCAL lock_timeout='100ms'"); e != nil {
		t.Fatal(e)
	}
	_, e = competitor.Exec(context.Background(), "UPDATE resources SET version=version+1 WHERE id=$1", id)
	var pg *pgconn.PgError
	if !errors.As(e, &pg) || pg.Code != "55P03" {
		t.Fatal("parent visibility can change between authorization and activity paging")
	}
}

func TestSupportCustomJobCannotBypassStatusUpdateAuthority(t *testing.T) {
	f := newFixture(t)
	id, path := supportTicket(f)
	f.call("POST", path+"/transition", map[string]any{"version": 1, "status": "in_progress"}, 200)
	actor, _, _ := f.addMember("nonupdate-job@example.test", []Grant{{Resource: "tickets", Action: "reply", Scope: "tenant", WriteFields: []string{"status", "resolution"}}}, "")
	job := uuid.NewString()
	claim := uuid.NewString()
	f.sql("INSERT INTO jobs(id,tenant_id,actor_id,type,status) VALUES($1,$2,$3,'nonupdate_ticket_action','running')", job, f.tenant, actor)
	f.sql("INSERT INTO job_leases(job_id,tenant_id,claim_token,lease_until) VALUES($1,$2,$3,now()+interval '1 hour')", job, f.tenant, claim)
	s := &jobService{app: f.app, ctx: context.Background(), id: job, tenant: f.tenant, actor: actor, claim: claim, resource: "tickets", action: "reply"}
	_, e := s.UpdateRecord(id, 2, map[string]any{"status": "resolved", "resolution": "bypass"})
	var denied *APIError
	if !errors.As(e, &denied) || denied.Status != 403 {
		t.Fatal("non-update job bypassed ticket status authority")
	}
	current := asMap(f.call("GET", path, nil, 200)["data"])
	if current["version"] != float64(2) || asMap(current["data"])["status"] != "in_progress" {
		t.Fatal("denied job mutated ticket")
	}
	activity := f.call("GET", path+"/activity", nil, 200)
	if asMap(activity["meta"])["total"] != float64(2) {
		t.Fatal("denied job inserted history")
	}
	var count int
	if e = f.admin.QueryRow(context.Background(), "SELECT count(*) FROM audit WHERE resource='tickets' AND action='reply'").Scan(&count); e != nil || count != 0 {
		t.Fatal("denied job inserted audit", e)
	}
}

func TestSupportNonUpdateJobCannotStartWork(t *testing.T) {
	f := newFixture(t)
	id, path := supportTicket(f)
	actor, _, _ := f.addMember("assign-job@example.test", []Grant{{Resource: "tickets", Action: "assign", Scope: "tenant", WriteFields: []string{"status"}}}, "")
	job, claim := uuid.NewString(), uuid.NewString()
	f.sql("INSERT INTO jobs(id,tenant_id,actor_id,type,status) VALUES($1,$2,$3,'nonupdate_assign','running')", job, f.tenant, actor)
	f.sql("INSERT INTO job_leases(job_id,tenant_id,claim_token,lease_until) VALUES($1,$2,$3,now()+interval '1 hour')", job, f.tenant, claim)
	s := &jobService{app: f.app, ctx: context.Background(), id: job, tenant: f.tenant, actor: actor, claim: claim, resource: "tickets", action: "assign"}
	_, e := s.UpdateRecord(id, 1, map[string]any{"status": "in_progress"})
	var denied *APIError
	if !errors.As(e, &denied) || denied.Status != 403 {
		t.Fatal("assign job bypassed start-work update authority")
	}
	record := asMap(f.call("GET", path, nil, 200)["data"])
	if record["version"] != float64(1) || asMap(record["data"])["status"] != "open" {
		t.Fatal("denied start-work job committed state")
	}
}
