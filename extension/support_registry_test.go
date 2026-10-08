package extension

import "testing"

func TestSupportReplyActionRegistration(t *testing.T) {
	r := NewRegistry()
	if e := r.Register(Module{Key: "tickets", Label: "工单", LabelEN: "Tickets", Actions: []string{"read", "reply"}, Fields: []Field{{Key: "description", Label: "描述", LabelEN: "Description", Type: "text"}}}); e != nil {
		t.Fatal("core reply action rejected", e)
	}
}
func TestSupportReplyDoesNotWeakenCustomOperationRegistration(t *testing.T) {
	if e := NewRegistry().Register(Module{Key: "assets", Label: "资产", LabelEN: "Assets", Actions: []string{"reply"}, Fields: []Field{{Key: "description", Label: "描述", LabelEN: "Description", Type: "text"}}}); e == nil {
		t.Fatal("reply accepted as arbitrary extension action without callback")
	}
}
