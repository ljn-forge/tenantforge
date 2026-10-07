package platform

import "testing"

func TestHTTPIsolationAuditAndVersions(t *testing.T) {
	f := newFixture(t)
	created := asMap(f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "actual"}}, 200)["data"])
	id := created["id"].(string)
	f.call("PATCH", "/resources/tickets/"+id, map[string]any{"version": 1, "data": map[string]any{"title": "changed"}}, 200)
	f.call("PATCH", "/resources/tickets/"+id, map[string]any{"version": 1, "data": map[string]any{"title": "stale"}}, 409)
	f.call("POST", "/resources/tickets/"+id+"/transition", map[string]any{"version": 2, "status": "closed"}, 422)
	if len(f.call("GET", "/audit", nil, 200)["data"].([]any)) < 3 {
		t.Fatal("transactional audit missing")
	}
	f.tenant = "11111111-1111-4111-8111-111111111111"
	f.call("GET", "/resources/tickets/"+id, nil, 403)
}
