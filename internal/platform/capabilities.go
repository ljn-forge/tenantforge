package platform

import (
	"slices"
	"strings"
)

func capabilities(p Principal, module string, r Record) Capabilities {
	c := Capabilities{Actions: []string{}, WriteFields: []string{}, Transitions: []string{}}
	m, ok := Descriptor(module)
	if !ok {
		m.Actions = resourceActions[module]
	}
	for _, action := range m.Actions {
		if Allowed(p, module, action, r) {
			c.Actions = append(c.Actions, action)
		}
	}
	for _, f := range m.Fields {
		if f.Readonly {
			continue
		}
		for _, g := range matching(p, module, "update", r) {
			if fieldAllowed(g.WriteFields, f) {
				c.WriteFields = append(c.WriteFields, f.Key)
				break
			}
		}
	}
	if policy, exists := policies[module]; exists && policy.Transitions != nil {
		c.Transitions = policy.Transitions(p, r)
	}
	for _, op := range m.Operations {
		action := op.Action
		if action == "" {
			action = op.Key
		}
		if action != op.Key && Allowed(p, module, action, r) {
			c.Actions = append(c.Actions, op.Key)
		}
	}
	return c
}
func (q *request) controlCapabilities(module string, row map[string]any, owner, department string) map[string]any {
	row["capabilities"] = capabilities(q.p, module, q.controlRow(owner, department))
	return row
}
func (q *request) publicJob(row map[string]any) map[string]any {
	out := jobPublic(row)
	if hash := stringValue(row["authority_hash"]); hash != "" && hash != jobAuthority(q.p, stringValue(row["authority_resource"]), stringValue(row["authority_action"])) {
		delete(out, "result")
	}
	return q.controlCapabilities("jobs", out, stringValue(row["actor_id"]), stringValue(row["department_id"]))
}

// Control rows carry an internal owner projection so scopes are evaluated before serialization.
func (q *request) decorateControlRow(row map[string]any) {
	path := q.r.URL.Path
	resource := ""
	owner := stringValue(row["_cap_owner"])
	department := stringValue(row["department_id"])
	switch {
	case strings.HasPrefix(path, "/api/v1/jobs"):
		return
	case strings.HasPrefix(path, "/api/v1/files"):
		resource = "files"
		owner = stringValue(row["owner_id"])
	case strings.HasPrefix(path, "/api/v1/members"):
		resource = "members"
		owner = stringValue(row["user_id"])
	case strings.HasPrefix(path, "/api/v1/roles"):
		resource = "roles"
	case strings.HasPrefix(path, "/api/v1/departments"):
		resource = "departments"
		department = stringValue(row["id"])
	case strings.HasPrefix(path, "/api/v1/invitations"):
		resource = "invitations"
	case strings.HasPrefix(path, "/api/v1/platform/tenants"):
		actions := []string{}
		if q.p.PlatformAdmin {
			actions = []string{"read", "update"}
		}
		row["capabilities"] = Capabilities{Actions: actions, WriteFields: []string{}, Transitions: []string{}}
	}
	if resource != "" && row["id"] != nil {
		cap := capabilities(q.p, resource, q.controlRow(owner, department))
		if resource == "roles" && row["builtin"] == true {
			cap.Actions = slices.DeleteFunc(cap.Actions, func(a string) bool { return a == "update" || a == "delete" })
		}
		row["capabilities"] = cap
	}
	delete(row, "_cap_owner")
	if resource == "jobs" {
		if hash := stringValue(row["authority_hash"]); hash != "" && hash != jobAuthority(q.p, stringValue(row["authority_resource"]), stringValue(row["authority_action"])) {
			delete(row, "result")
		}
		delete(row, "authority_hash")
		delete(row, "authority_resource")
		delete(row, "authority_action")
	}
}
