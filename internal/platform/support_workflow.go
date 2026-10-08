package platform

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// All history visibility is evaluated against the current parent and current grants.
func ticketFieldReadable(p Principal, r Record, key string) bool {
	m, _ := Descriptor("tickets")
	for _, f := range m.Fields {
		if f.Key == key {
			for _, g := range matching(p, "tickets", "read", r) {
				if fieldAllowed(g.ReadFields, f) && (!f.Sensitive || slices.Contains(g.UnmaskFields, key)) {
					return true
				}
			}
		}
	}
	return false
}
func ticketReplyKinds(p Principal, r Record) []string {
	out := []string{}
	if stringValue(r.Data["status"]) == "closed" || !Allowed(p, "tickets", "read", r) {
		return out
	}
	for _, kind := range []string{"reply", "note"} {
		field := "description"
		if kind == "note" {
			field = "internal_notes"
		}
		if ticketFieldReadable(p, r, field) && CanWrite(p, "tickets", "reply", r, map[string]any{field: ""}) {
			out = append(out, kind)
		}
	}
	return out
}
func ticketQueue(q *request, m string, b *predicate, where *string) error {
	queues, present := q.r.URL.Query()["queue"]
	if !present {
		return nil
	}
	if m != "tickets" || len(queues) != 1 {
		return validation("Invalid ticket queue")
	}
	queue := queues[0]
	if queue == "all" {
		return nil
	}
	if !slices.Contains([]string{"mine", "active", "resolved", "closed"}, queue) {
		return validation("Invalid ticket queue")
	}
	module, _ := Descriptor("tickets")
	for _, f := range module.Fields {
		if f.Key == "status" {
			*where += " AND " + b.readField(q.p, m, "read", "r", "r.owner_id", "r.department_id", "r.data", f, true)
		}
	}
	if queue == "mine" {
		*where += " AND r.owner_id=" + b.arg(q.p.ID) + "::uuid"
	}
	if queue == "mine" || queue == "active" {
		*where += " AND r.data->>'status' IN ('open','in_progress')"
	} else {
		*where += " AND r.data->>'status'=" + b.arg(queue)
	}
	return nil
}
func (q *request) ticketParent(lock bool) (Record, error) {
	m, e := q.module()
	if e != nil {
		return Record{}, e
	}
	if m != "tickets" {
		return Record{}, notFound()
	}
	id, e := q.id()
	if e != nil {
		return Record{}, e
	}
	clause := " FOR SHARE"
	if lock {
		clause = " FOR UPDATE"
	}
	r, e := q.recordLocked(m, id, clause)
	if e != nil {
		return Record{}, e
	}
	if !Allowed(q.p, m, "read", r) {
		return Record{}, notFound()
	}
	return r, nil
}
func (q *request) insertTicketEvent(ticket, kind, body string, metadata map[string]any) (map[string]any, error) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	var name string
	if e := q.tx.QueryRow(q.ctx(), "SELECT display_name FROM users WHERE id=$1", q.p.ID).Scan(&name); e != nil {
		return nil, e
	}
	id := uuid.NewString()
	if _, e := q.tx.Exec(q.ctx(), `INSERT INTO ticket_events(id,tenant_id,ticket_id,kind,actor_id,actor_name,body,metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, q.p.TenantID, ticket, kind, q.p.ID, name, nullable(body), jsonBytes(metadata)); e != nil {
		return nil, e
	}
	return q.one(ticketEventSelect+" FROM ticket_events e WHERE e.tenant_id=$1 AND e.id=$2", q.p.TenantID, id)
}
func (q *request) ticketChangeEvents(old, next Record, delta map[string]any) error {
	if old.OwnerID != next.OwnerID || old.DepartmentID != next.DepartmentID {
		var department any
		if next.DepartmentID != "" {
			department = next.DepartmentID
		}
		if _, e := q.insertTicketEvent(old.ID, "assigned", "", map[string]any{"owner_id": next.OwnerID, "department_id": department}); e != nil {
			return e
		}
	}
	if old.Data["status"] != next.Data["status"] {
		body := ""
		if next.Data["status"] == "resolved" {
			body = stringValue(next.Data["resolution"])
		}
		if _, e := q.insertTicketEvent(old.ID, "status_changed", body, map[string]any{"from_status": old.Data["status"], "to_status": next.Data["status"]}); e != nil {
			return e
		}
	}
	fields := []string{}
	for _, key := range safeFieldKeys("tickets", delta) {
		if key != "status" && !(key == "resolution" && old.Data["status"] != next.Data["status"] && next.Data["status"] == "resolved") && !equalJSON(old.Data[key], next.Data[key]) {
			fields = append(fields, key)
		}
	}
	if len(fields) > 0 {
		if _, e := q.insertTicketEvent(old.ID, "updated", "", map[string]any{"fields": fields}); e != nil {
			return e
		}
	}
	return nil
}

const ticketEventSelect = `SELECT e.id::text,e.ticket_id::text,e.kind,e.actor_id::text,e.actor_name,e.created_at,e.body,e.metadata`

func publicTicketEvent(p Principal, r Record, row map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"id", "ticket_id", "kind", "actor_id", "actor_name", "created_at"} {
		out[key] = row[key]
	}
	metadata := asJSONMap(row["metadata"])
	kind := stringValue(row["kind"])
	if kind == "reply" || kind == "note" || (kind == "status_changed" && ticketFieldReadable(p, r, "resolution")) {
		if body, ok := row["body"].(string); ok && body != "" {
			out["body"] = body
		}
	}
	if kind == "status_changed" && ticketFieldReadable(p, r, "status") {
		out["from_status"] = metadata["from_status"]
		out["to_status"] = metadata["to_status"]
	}
	if kind == "assigned" {
		if stringValue(metadata["owner_id"]) == r.OwnerID {
			out["owner_id"] = r.OwnerID
		}
		if stringValue(metadata["department_id"]) == r.DepartmentID {
			out["department_id"] = metadata["department_id"]
		}
	}
	if kind == "updated" {
		fields := []string{}
		if keys, ok := metadata["fields"].([]any); ok {
			for _, key := range keys {
				if s, ok := key.(string); ok && ticketFieldReadable(p, r, s) {
					fields = append(fields, s)
				}
			}
		}
		out["fields"] = fields
	}
	return out
}
func asJSONMap(v any) map[string]any {
	var m map[string]any
	json.Unmarshal(jsonBytes(v), &m)
	return m
}
func ticketActivity(q *request) (any, error) {
	r, e := q.ticketParent(false)
	if e != nil {
		return nil, e
	}
	b := &predicate{}
	where := "e.tenant_id=" + b.arg(q.p.TenantID) + "::uuid AND e.ticket_id=" + b.arg(r.ID) + "::uuid"
	hidden := []string{}
	if !ticketFieldReadable(q.p, r, "description") {
		hidden = append(hidden, "reply")
	}
	if !ticketFieldReadable(q.p, r, "internal_notes") {
		hidden = append(hidden, "note")
	}
	if len(hidden) > 0 {
		where += " AND NOT(e.kind=ANY(" + b.arg(hidden) + "::text[]))"
	}
	page, e := q.sqlPage(ticketEventSelect, "FROM ticket_events e", where, "e.created_at DESC,e.id DESC", b)
	if e != nil {
		return nil, e
	}
	rows := page.Data.([]map[string]any)
	for i, row := range rows {
		rows[i] = publicTicketEvent(q.p, r, row)
	}
	page.Data = rows
	return page, nil
}
func replyTicket(q *request) (any, error) {
	var in struct {
		Version int    `json:"version"`
		Kind    string `json:"kind"`
		Body    string `json:"body"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	if !slices.Contains([]string{"reply", "note"}, in.Kind) || strings.TrimSpace(in.Body) == "" || !utf8.ValidString(in.Body) || utf8.RuneCountInString(in.Body) > 4000 || len(in.Body) > 8192 {
		return nil, validation("Invalid reply kind or body (maximum 4000 characters and 8 KiB)")
	}
	old, e := q.ticketParent(true)
	if e != nil {
		return nil, e
	}
	if !slices.Contains(ticketReplyKinds(q.p, old), in.Kind) {
		if old.Data["status"] == "closed" {
			return nil, validation("Closed tickets cannot receive replies")
		}
		return nil, forbidden()
	}
	if old.Version != in.Version {
		return nil, conflict()
	}
	next := old
	next.Version++
	field := "description"
	if in.Kind == "note" {
		field = "internal_notes"
	}
	if !Allowed(q.p, "tickets", "read", next) || !ticketFieldReadable(q.p, next, field) || !CanWrite(q.p, "tickets", "reply", next, map[string]any{field: in.Body}) {
		return nil, forbidden()
	}
	if _, e = q.tx.Exec(q.ctx(), "UPDATE resources SET version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND version=$3", q.p.TenantID, old.ID, in.Version); e != nil {
		return nil, e
	}
	event, e := q.insertTicketEvent(old.ID, in.Kind, in.Body, nil)
	if e != nil {
		return nil, e
	}
	if e = q.audit("reply", "tickets", old.ID, map[string]any{"kind": in.Kind, "previous_version": old.Version, "version": old.Version + 1}); e != nil {
		return nil, e
	}
	next, e = q.record("tickets", old.ID, false)
	if e != nil {
		return nil, e
	}
	return map[string]any{"record": q.visible("tickets", "read", next), "event": publicTicketEvent(q.p, next, event)}, nil
}
func assignTicket(q *request) (any, error) {
	var in struct {
		Version    int            `json:"version"`
		Owner      string         `json:"owner_id"`
		Department OptionalString `json:"department_id"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	old, e := q.ticketParent(true)
	if e != nil {
		return nil, e
	}
	if old.Version != in.Version {
		return nil, conflict()
	}
	dep, e := q.ownerDepartment(in.Owner)
	if e != nil {
		return nil, e
	}
	if !in.Department.Present {
		in.Department = OptionalString{Present: true, Value: dep}
	}
	next, e := q.updateRecord("tickets", old.ID, resourceInput{Version: in.Version, Owner: &in.Owner, Department: in.Department, Data: map[string]any{}}, "assign")
	if e != nil {
		return nil, e
	}
	return q.visible("tickets", "read", next), nil
}
