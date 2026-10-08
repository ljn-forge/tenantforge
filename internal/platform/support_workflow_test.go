package platform

import (
	"slices"
	"testing"
)

func TestSupportResolutionRequiresFreshSummary(t *testing.T) {
	old := Record{Data: map[string]any{"status": "in_progress", "resolution": "previous"}}
	for _, delta := range []map[string]any{{"status": "resolved"}, {"status": "resolved", "resolution": " \n "}} {
		if e := policies["tickets"].Change(old, delta); e == nil {
			t.Fatal("resolution accepted without fresh nonblank input")
		}
	}
	if e := policies["tickets"].Change(old, map[string]any{"status": "resolved", "resolution": "fixed"}); e != nil {
		t.Fatal(e)
	}
}
func TestSupportResolutionCapabilitiesNeedBothFields(t *testing.T) {
	r := Record{TenantID: "t", OwnerID: "u", Data: map[string]any{"status": "in_progress"}}
	p := Principal{ID: "u", TenantID: "t", Grants: []Grant{{Resource: "tickets", Action: "update", Scope: "self", WriteFields: []string{"status"}}}}
	if slices.Contains(capabilities(p, "tickets", r).Transitions, "resolved") {
		t.Fatal("resolve advertised without resolution write")
	}
}

func TestSupportReplyCapabilitiesRespectCurrentFieldsAndScope(t *testing.T) {
	r := Record{TenantID: "t", OwnerID: "u", Data: map[string]any{"status": "open"}}
	base := []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"description", "internal_notes"}}, {Resource: "tickets", Action: "reply", Scope: "self", WriteFields: []string{"description", "internal_notes"}}}
	p := Principal{ID: "u", TenantID: "t", Grants: base}
	if kinds := ticketReplyKinds(p, r); !slices.Equal(kinds, []string{"reply"}) {
		t.Fatalf("masked note composer advertised: %v", kinds)
	}
	p.Grants[0].UnmaskFields = []string{"internal_notes"}
	if kinds := ticketReplyKinds(p, r); !slices.Equal(kinds, []string{"reply", "note"}) {
		t.Fatalf("explicit kinds missing: %v", kinds)
	}
	r.OwnerID = "other"
	if len(ticketReplyKinds(p, r)) != 0 {
		t.Fatal("reply scope inherited from read")
	}
	r.OwnerID = "u"
	r.Data["status"] = "closed"
	if len(ticketReplyKinds(p, r)) != 0 {
		t.Fatal("closed composer advertised")
	}
}
