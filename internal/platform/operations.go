package platform

import (
	"encoding/json"

	"github.com/ljn-forge/tenantforge/extension"
)

func operateResource(q *request) (any, error) {
	module, e := q.module()
	if e != nil {
		return nil, e
	}
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	m, _ := Descriptor(module)
	var operation *extension.Operation
	for _, op := range m.Operations {
		if op.Key == q.r.PathValue("action") {
			copy := op
			operation = &copy
			break
		}
	}
	if operation == nil {
		return nil, notFound()
	}
	action := operation.Action
	if action == "" {
		action = operation.Key
	}
	var in struct {
		Version int            `json:"version"`
		Data    map[string]any `json:"data"`
	}
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	if in.Data == nil {
		return nil, validation("Operation data object is required")
	}
	old, e := q.record(module, id, false)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, module, action, old) {
		return nil, forbidden()
	}
	if old.Version != in.Version {
		return nil, conflict()
	}
	// Only fields readable under this operation are exposed to its callback.
	view := q.visible(module, action, old)
	if e = q.release(); e != nil {
		return nil, e
	}
	data, e := operation.Apply(extension.OperationContext{Context: q.ctx(), ActorID: q.p.ID, TenantID: q.p.TenantID, Record: view, Data: cloneJSONMap(in.Data)})
	if e != nil {
		return nil, validation("Operation input or state is invalid")
	}
	if e = q.ctx().Err(); e != nil {
		return nil, e
	}
	if e = q.reopen(); e != nil {
		return nil, e
	}
	r, e := q.updateRecord(module, id, resourceInput{Version: in.Version, Data: data}, action)
	if e != nil {
		return nil, e
	}
	return q.visible(module, "read", r), nil
}
func cloneJSONMap(m map[string]any) map[string]any {
	var out map[string]any
	json.Unmarshal(jsonBytes(m), &out)
	return out
}
