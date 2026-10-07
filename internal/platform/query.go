package platform

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// SQL arguments carry all user values. Identifiers come exclusively from registered descriptors.
type predicate struct{ Args []any }

func (b *predicate) arg(v any) string {
	b.Args = append(b.Args, v)
	return fmt.Sprintf("$%d", len(b.Args))
}
func (b *predicate) grant(p Principal, g Grant, alias, owner, department, data string) string {
	parts := []string{alias + ".tenant_id=" + b.arg(p.TenantID) + "::uuid"}
	switch g.Scope {
	case "tenant":
	case "self":
		parts = append(parts, owner+"="+b.arg(p.ID)+"::uuid")
	case "department":
		if p.DepartmentID == "" {
			return "false"
		}
		parts = append(parts, department+"="+b.arg(p.DepartmentID)+"::uuid")
	case "department_tree":
		if p.DepartmentID == "" {
			return "false"
		}
		ids := []string{p.DepartmentID}
		for id := range p.Departments {
			seen := map[string]bool{}
			for d := id; d != "" && !seen[d]; d = p.Departments[d] {
				if d == p.DepartmentID {
					ids = append(ids, id)
					break
				}
				seen[d] = true
			}
		}
		parts = append(parts, department+"::text=ANY("+b.arg(ids)+"::text[])")
	case "selected_departments":
		parts = append(parts, department+"::text=ANY("+b.arg(g.DepartmentIDs)+"::text[])")
	default:
		return "false"
	}
	for _, c := range g.Conditions {
		if data == "" {
			return "false"
		}
		field := data + "->" + b.arg(c.Field) + "::text"
		if c.Op == "eq" {
			parts = append(parts, "("+field+")="+b.arg(string(jsonBytes(c.Value)))+"::jsonb")
		} else if c.Op == "in" {
			vs, ok := c.Value.([]any)
			if !ok || len(vs) == 0 {
				return "false"
			}
			items := []string{}
			for _, v := range vs {
				items = append(items, b.arg(string(jsonBytes(v)))+"::jsonb")
			}
			parts = append(parts, "("+field+") IN ("+strings.Join(items, ",")+")")
		} else {
			return "false"
		}
	}
	return "(" + strings.Join(parts, " AND ") + ")"
}
func (b *predicate) authorized(p Principal, res, action, alias, owner, department, data string) string {
	ors := []string{}
	for _, g := range p.Grants {
		if actionGranted(g, res, action) {
			ors = append(ors, b.grant(p, g, alias, owner, department, data))
		}
	}
	if len(ors) == 0 {
		return "false"
	}
	return "(" + strings.Join(ors, " OR ") + ")"
}
func (b *predicate) readField(p Principal, res, action, alias, owner, department, data string, f Field, unmask bool) string {
	ors := []string{}
	for _, g := range p.Grants {
		if actionGranted(g, res, action) && fieldAllowed(g.ReadFields, f) && (!unmask || !f.Sensitive || slices.Contains(g.UnmaskFields, f.Key)) {
			ors = append(ors, b.grant(p, g, alias, owner, department, data))
		}
	}
	if len(ors) == 0 {
		return "false"
	}
	return "(" + strings.Join(ors, " OR ") + ")"
}
func (q *request) pagination() (int, int) {
	page, _ := strconv.Atoi(q.r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(q.r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
func (q *request) sqlPage(selectSQL, from, where, order string, b *predicate) (paged, error) {
	var total int
	if e := q.tx.QueryRow(q.ctx(), "SELECT count(*) "+from+" WHERE "+where, b.Args...).Scan(&total); e != nil {
		return paged{}, e
	}
	page, size := q.pagination()
	limit := b.arg(size)
	offset := b.arg((page - 1) * size)
	rs, e := q.rows(selectSQL+" "+from+" WHERE "+where+" ORDER BY "+order+" LIMIT "+limit+" OFFSET "+offset, b.Args...)
	return paged{rs, map[string]int{"page": page, "page_size": size, "total": total}}, e
}
func decodeRecords(rs []map[string]any) ([]storedRecord, error) {
	out := []storedRecord{}
	for _, row := range rs {
		var r storedRecord
		if e := json.Unmarshal(jsonBytes(row), &r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, nil
}

const recordSelect = `SELECT r.id::text,r.tenant_id::text,r.module,r.owner_id::text,coalesce(r.department_id::text,'') department_id,r.version,r.data,r.created_at,r.updated_at`

func (q *request) authorizedRecords(module, action string) ([]storedRecord, error) {
	b := &predicate{}
	where := b.authorized(q.p, module, action, "r", "r.owner_id", "r.department_id", "r.data") + " AND r.module=" + b.arg(module)
	rs, e := q.rows(recordSelect+" FROM resources r WHERE "+where+" ORDER BY r.created_at DESC,r.id", b.Args...)
	if e != nil {
		return nil, e
	}
	return decodeRecords(rs)
}
