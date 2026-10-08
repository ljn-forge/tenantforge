package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	casbin "github.com/casbin/casbin/v2"

	"github.com/casbin/casbin/v2/model"
)

var Scopes = []string{"self", "department", "department_tree", "selected_departments", "tenant"}
var resourceActions = map[string][]string{"members": {"read", "create", "update", "assign"}, "departments": {"read", "create", "update", "delete"}, "roles": {"read", "create", "update", "delete"}, "invitations": {"read", "create"}, "audit": {"read", "export"}, "files": {"read", "create", "delete"}, "jobs": {"read", "create", "update"}}
var Modules = []Module{
	{Key: "tickets", Label: "工单", LabelEN: "Tickets", Description: "管理服务工单及处理进度", DescriptionEN: "Manage service tickets and their progress", Actions: []string{"read", "create", "update", "delete", "assign", "reply", "export", "import"}, Fields: []Field{{Key: "title", Label: "标题", LabelEN: "Title", Type: "string", Required: true}, {Key: "description", Label: "描述", LabelEN: "Description", Type: "text"}, {Key: "status", Label: "状态", LabelEN: "Status", Type: "enum", Options: []string{"open", "in_progress", "resolved", "closed"}}, {Key: "priority", Label: "优先级", LabelEN: "Priority", Type: "enum", Options: []string{"low", "normal", "high", "urgent"}}, {Key: "customer_name", Label: "客户姓名", LabelEN: "Customer name", Type: "string"}, {Key: "resolution", Label: "解决说明", LabelEN: "Resolution", Type: "text"}, {Key: "customer_email", Label: "客户邮箱", LabelEN: "Customer email", Type: "string", Sensitive: true}, {Key: "internal_notes", Label: "内部备注", LabelEN: "Internal notes", Type: "text", Sensitive: true}}},
	{Key: "dictionaries", Label: "字典", LabelEN: "Dictionaries", Description: "管理业务字典与选项", DescriptionEN: "Manage business dictionaries and options", Actions: []string{"read", "create", "update", "delete", "export", "import"}, Fields: []Field{{Key: "key", Label: "键", LabelEN: "Key", Type: "string", Required: true}, {Key: "label", Label: "标签", LabelEN: "Label", Type: "string", Required: true}, {Key: "value", Label: "值", LabelEN: "Value", Type: "text"}, {Key: "category", Label: "类别", LabelEN: "Category", Type: "string"}, {Key: "enabled", Label: "启用", LabelEN: "Enabled", Type: "boolean"}}},
	{Key: "settings", Label: "设置", LabelEN: "Settings", Description: "管理当前租户的业务设置", DescriptionEN: "Manage settings for the current workspace", Actions: []string{"read", "create", "update", "delete", "export", "import"}, Fields: []Field{{Key: "key", Label: "键", LabelEN: "Key", Type: "string", Required: true}, {Key: "value", Label: "值", LabelEN: "Value", Type: "text"}, {Key: "description", Label: "说明", LabelEN: "Description", Type: "text"}}},
	{Key: "notifications", Label: "通知", LabelEN: "Notifications", Description: "管理租户公告与通知", DescriptionEN: "Manage workspace announcements and notifications", Actions: []string{"read", "create", "update", "delete", "export", "import"}, Fields: []Field{{Key: "title", Label: "标题", LabelEN: "Title", Type: "string", Required: true}, {Key: "body", Label: "正文", LabelEN: "Body", Type: "text"}, {Key: "read", Label: "已读", LabelEN: "Read", Type: "boolean"}}},
}

func Descriptor(key string) (Module, bool) {
	for _, m := range resourceModules() {
		if m.Key == key {
			return m, true
		}
	}
	return Module{}, false
}

var controlLabels = map[string][2]string{
	"members": {"用户", "Users"}, "departments": {"部门", "Departments"}, "roles": {"角色", "Roles"}, "invitations": {"邀请", "Invitations"}, "audit": {"审计日志", "Audit log"}, "files": {"文件", "Files"}, "jobs": {"后台任务", "Background jobs"},
}

func registry() []Module {
	out := resourceModules()
	for _, k := range []string{"members", "departments", "roles", "invitations", "audit", "files", "jobs"} {
		out = append(out, Module{Key: k, Label: controlLabels[k][0], LabelEN: controlLabels[k][1], Actions: resourceActions[k], Fields: []Field{}})
	}
	return out
}
func ownerGrants() []Grant {
	out := []Grant{}
	for _, m := range registry() {
		for _, a := range m.Actions {
			g := Grant{Resource: m.Key, Action: a, Scope: "tenant", ReadFields: []string{"*"}, WriteFields: []string{"*"}}
			for _, f := range m.Fields {
				if f.Sensitive {
					g.ReadFields = append(g.ReadFields, f.Key)
					g.WriteFields = append(g.WriteFields, f.Key)
					g.UnmaskFields = append(g.UnmaskFields, f.Key)
				}
			}
			out = append(out, g)
		}
	}
	return out
}
func actionGranted(g Grant, resource, action string) bool {
	// Casbin owns operation matching; the descriptor layer owns typed record scopes.
	return g.Resource == resource && g.Action == action
}
func NewOperationEnforcer(gs []Grant) (*casbin.Enforcer, error) {
	m, err := model.NewModelFromString(`[request_definition]
r = resource, action
[policy_definition]
p = resource, action
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = r.resource == p.resource && r.action == p.action`)
	if err != nil {
		return nil, err
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, err
	}
	e.EnableLog(false)
	for _, g := range gs {
		if _, err = e.AddPolicy(g.Resource, g.Action); err != nil {
			return nil, err
		}
	}
	return e, nil
}
func scopeMatches(p Principal, g Grant, r Record) bool {
	if p.TenantID == "" || r.TenantID != p.TenantID {
		return false
	}
	match := false
	switch g.Scope {
	case "tenant":
		match = true
	case "self":
		match = r.OwnerID == p.ID
	case "department":
		match = p.DepartmentID != "" && r.DepartmentID == p.DepartmentID
	case "selected_departments":
		match = slices.Contains(g.DepartmentIDs, r.DepartmentID)
	case "department_tree":
		if p.DepartmentID != "" {
			seen := map[string]bool{}
			for d := r.DepartmentID; d != "" && !seen[d]; d = p.Departments[d] {
				if d == p.DepartmentID {
					match = true
					break
				}
				seen[d] = true
			}
		}
	}
	if !match {
		return false
	}
	for _, c := range g.Conditions {
		v := r.Data[c.Field]
		if c.Op == "eq" {
			if !equalJSON(v, c.Value) {
				return false
			}
		} else if c.Op == "in" {
			vs, ok := c.Value.([]any)
			if !ok {
				return false
			}
			found := false
			for _, x := range vs {
				if equalJSON(v, x) {
					found = true
				}
			}
			if !found {
				return false
			}
		} else {
			return false
		}
	}
	return true
}
func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func matching(p Principal, res, action string, r Record) []Grant {
	out := []Grant{}
	for _, g := range p.Grants {
		if actionGranted(g, res, action) && scopeMatches(p, g, r) {
			out = append(out, g)
		}
	}
	return out
}
func Allowed(p Principal, res, action string, r Record) bool {
	return len(matching(p, res, action, r)) > 0
}
func fieldAllowed(fs []string, f Field) bool {
	return slices.Contains(fs, f.Key) || (!f.Sensitive && slices.Contains(fs, "*"))
}
func FilterData(p Principal, res, action string, r Record) map[string]any {
	out := map[string]any{}
	m, _ := Descriptor(res)
	gs := matching(p, res, action, r)
	for _, f := range m.Fields {
		for _, g := range gs {
			if fieldAllowed(g.ReadFields, f) {
				if v, ok := r.Data[f.Key]; ok {
					out[f.Key] = v
					if f.Sensitive && !slices.Contains(g.UnmaskFields, f.Key) {
						out[f.Key] = "••••"
					}
					if f.Sensitive && slices.Contains(g.UnmaskFields, f.Key) {
						break
					}
				}
			}
		}
	}
	return out
}
func CanWrite(p Principal, res, action string, r Record, d map[string]any) bool {
	gs := matching(p, res, action, r)
	if len(gs) == 0 {
		return false
	}
	m, _ := Descriptor(res)
	for k := range d {
		found := false
		for _, f := range m.Fields {
			if f.Key == k && !f.Readonly {
				for _, g := range gs {
					if fieldAllowed(g.WriteFields, f) {
						found = true
					}
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func ValidateGrants(gs []Grant) error {
	if len(gs) > 300 {
		return errors.New("too many grants")
	}
	for _, g := range gs {
		m, ok := Descriptor(g.Resource)
		if !ok {
			as, exists := resourceActions[g.Resource]
			if !exists {
				return errors.New("unknown resource")
			}
			m.Actions = as
		}
		if !slices.Contains(m.Actions, g.Action) || !slices.Contains(Scopes, g.Scope) {
			return errors.New("unknown action or scope")
		}
		if g.Scope == "selected_departments" && len(g.DepartmentIDs) == 0 {
			return errors.New("selected departments required")
		}
		for _, fs := range [][]string{g.ReadFields, g.WriteFields, g.UnmaskFields} {
			for _, key := range fs {
				if key == "*" && !reflect.DeepEqual(fs, g.UnmaskFields) {
					continue
				}
				found := false
				for _, f := range m.Fields {
					if f.Key == key {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("unknown field %s", key)
				}
			}
		}
		for _, c := range g.Conditions {
			found := false
			for _, f := range m.Fields {
				if f.Key == c.Field && !f.Sensitive {
					found = true
					if c.Op == "eq" {
						if !validValue(f, c.Value) {
							return errors.New("invalid condition type")
						}
					} else if c.Op == "in" {
						vs, ok := c.Value.([]any)
						if !ok || len(vs) == 0 || len(vs) > 100 {
							return errors.New("invalid condition list")
						}
						for _, v := range vs {
							if !validValue(f, v) {
								return errors.New("invalid condition type")
							}
						}
					} else {
						return errors.New("invalid condition operator")
					}
				}
			}
			if !found {
				return errors.New("unknown or sensitive condition field")
			}
		}
	}
	return nil
}
func CanDelegate(p Principal, gs []Grant) bool {
	for _, g := range gs {
		covered := false
		for _, a := range p.Grants {
			if a.Resource != g.Resource || a.Action != g.Action {
				continue
			}
			if a.Scope != "tenant" && (a.Scope != g.Scope || !subset(g.DepartmentIDs, a.DepartmentIDs)) {
				continue
			}
			if len(a.Conditions) > 0 && !reflect.DeepEqual(a.Conditions, g.Conditions) {
				continue
			}
			m, _ := Descriptor(g.Resource)
			ok := true
			for _, f := range m.Fields {
				if fieldAllowed(g.ReadFields, f) && !fieldAllowed(a.ReadFields, f) || fieldAllowed(g.WriteFields, f) && !fieldAllowed(a.WriteFields, f) || slices.Contains(g.UnmaskFields, f.Key) && !slices.Contains(a.UnmaskFields, f.Key) {
					ok = false
				}
			}
			if ok {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}
func subset(a, b []string) bool {
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}
func validValue(f Field, v any) bool {
	switch f.Type {
	case "string", "text":
		s, ok := v.(string)
		return ok && len(s) <= 20000
	case "enum":
		s, ok := v.(string)
		return ok && slices.Contains(f.Options, s)
	case "number":
		_, ok := v.(float64)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "date":
		s, ok := v.(string)
		if !ok {
			return false
		}
		_, err := time.Parse("2006-01-02", s)
		return err == nil
	}
	return false
}
func validateFields(fields []Field, d map[string]any, require bool) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range d {
		found := false
		for _, f := range fields {
			if f.Key == k {
				found = true
				if f.Readonly || !validValue(f, v) {
					return nil, fmt.Errorf("invalid field %s", k)
				}
				if f.Required && strings.TrimSpace(fmt.Sprint(v)) == "" {
					return nil, fmt.Errorf("required field %s", k)
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown field %s", k)
		}
		out[k] = v
	}
	if require {
		for _, f := range fields {
			if f.Required {
				if _, ok := out[f.Key]; !ok {
					return nil, fmt.Errorf("required field %s", f.Key)
				}
			}
		}
	}
	return out, nil
}
func ValidateData(key string, d map[string]any, create bool) (map[string]any, error) {
	m, ok := Descriptor(key)
	if !ok {
		return nil, errors.New("unknown module")
	}
	out, e := validateFields(m.Fields, d, create)
	if e != nil {
		return nil, e
	}
	if create {
		if policy, exists := policies[key]; exists && policy.Create != nil {
			if e = policy.Create(out); e != nil {
				return nil, e
			}
		}
	}
	return out, nil
}

func CanAssignMember(p Principal, old, next Record) bool {
	return Allowed(p, "members", "assign", old) && Allowed(p, "members", "assign", next)
}
func CanDelegateTo(p Principal, gs []Grant, targetID, targetDepartment string) bool {
	for _, g := range gs {
		covered := false
		for _, a := range p.Grants {
			if a.Resource != g.Resource || a.Action != g.Action || !fieldsCover(a, g) {
				continue
			}
			if len(a.Conditions) > 0 && !reflect.DeepEqual(a.Conditions, g.Conditions) {
				continue
			}
			if scopeContains(p, a, g, targetID, targetDepartment) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}
func fieldsCover(a, g Grant) bool {
	m, _ := Descriptor(g.Resource)
	for _, f := range m.Fields {
		if fieldAllowed(g.ReadFields, f) && !fieldAllowed(a.ReadFields, f) || fieldAllowed(g.WriteFields, f) && !fieldAllowed(a.WriteFields, f) || slices.Contains(g.UnmaskFields, f.Key) && !slices.Contains(a.UnmaskFields, f.Key) {
			return false
		}
	}
	return true
}
func inTree(p Principal, root, dep string) bool {
	if root == "" || dep == "" {
		return false
	}
	seen := map[string]bool{}
	for d := dep; d != "" && !seen[d]; d = p.Departments[d] {
		if d == root {
			return true
		}
		seen[d] = true
	}
	return false
}
func scopeContains(p Principal, a, g Grant, targetID, targetDep string) bool {
	if a.Scope == "tenant" {
		return true
	}
	if g.Scope == "tenant" {
		return false
	}
	if g.Scope == "self" {
		return a.Scope == "self" && p.ID == targetID
	}
	if a.Scope == "self" {
		return false
	}
	if g.Scope == "department_tree" {
		return a.Scope == "department_tree" && inTree(p, p.DepartmentID, targetDep)
	}
	ids := g.DepartmentIDs
	if g.Scope == "department" {
		if targetDep == "" {
			return false
		}
		ids = []string{targetDep}
	}
	if len(ids) == 0 {
		return false
	}
	for _, d := range ids {
		switch a.Scope {
		case "department":
			if p.DepartmentID == "" || p.DepartmentID != d {
				return false
			}
		case "department_tree":
			if !inTree(p, p.DepartmentID, d) {
				return false
			}
		case "selected_departments":
			if !slices.Contains(a.DepartmentIDs, d) {
				return false
			}
		default:
			return false
		}
	}
	return true
}
