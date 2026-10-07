package extension

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"
)

type Registry struct {
	mu      sync.RWMutex
	modules []Module
	jobs    map[string]JobDefinition
	frozen  bool
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

func NewRegistry() *Registry {
	return &Registry{modules: []Module{}, jobs: map[string]JobDefinition{}}
}

var Default = NewRegistry()

// RegisterModule is the typed extension point. Call it during startup before New.
func RegisterModule(m Module) error { return Default.Register(m) }
func (r *Registry) Register(m Module) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return errors.New("module registry frozen")
	}
	if !identifier.MatchString(m.Key) || m.Label == "" || m.LabelEN == "" || len(m.Fields) == 0 || len(m.Fields) > 100 || len(m.Actions) == 0 {
		return errors.New("invalid module descriptor")
	}
	if slices.Contains([]string{"members", "departments", "roles", "invitations", "audit", "files", "jobs", "auth", "users", "sessions", "tenants", "platform", "setup", "permissions", "dashboard"}, m.Key) {
		return errors.New("reserved module key")
	}
	for _, old := range r.modules {
		if old.Key == m.Key {
			return errors.New("duplicate module key")
		}
	}
	standard := []string{"read", "create", "update", "delete", "assign", "export", "import"}
	operations := map[string]bool{}
	for _, op := range m.Operations {
		if !identifier.MatchString(op.Key) || op.Label == "" || op.LabelEN == "" || op.Apply == nil || operations[op.Key] || slices.Contains(standard, op.Key) {
			return errors.New("invalid or duplicate operation")
		}
		action := op.Action
		if action == "" {
			action = op.Key
		}
		if !slices.Contains(m.Actions, action) {
			return errors.New("operation action not declared")
		}
		operations[op.Key] = true
	}
	seen := map[string]bool{}
	for _, action := range m.Actions {
		if !identifier.MatchString(action) || seen[action] || !slices.Contains(standard, action) && !operations[action] {
			return errors.New("unsupported or duplicate action")
		}
		seen[action] = true
	}
	seen = map[string]bool{}
	for _, f := range m.Fields {
		if !identifier.MatchString(f.Key) || seen[f.Key] || slices.Contains([]string{"id", "tenant_id", "owner_id", "department_id", "version", "created_at", "updated_at"}, f.Key) || f.Label == "" || f.LabelEN == "" || !slices.Contains([]string{"string", "text", "number", "boolean", "enum", "date"}, f.Type) {
			return errors.New("invalid, reserved or duplicate field")
		}
		seen[f.Key] = true
		if e := validateOptions(f); e != nil {
			return e
		}
		if f.Readonly && f.Required {
			return errors.New("readonly field cannot be required on creation")
		}
	}
	r.modules = append(r.modules, cloneModules([]Module{m})[0])
	return nil
}
func (r *Registry) Freeze() { r.mu.Lock(); defer r.mu.Unlock(); r.frozen = true }
func (r *Registry) Modules() []Module {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := append([]Module{}, r.modules...)
	for i := range out {
		out[i].Fields = append([]Field{}, out[i].Fields...)
		out[i].Actions = append([]string{}, out[i].Actions...)
		out[i].Operations = append([]Operation{}, out[i].Operations...)
		for j := range out[i].Fields {
			out[i].Fields[j].Options = append([]string{}, out[i].Fields[j].Options...)
		}
	}
	return out
}

func cloneModules(ms []Module) []Module {
	out := append([]Module{}, ms...)
	for i := range out {
		out[i].Fields = append([]Field{}, out[i].Fields...)
		out[i].Actions = append([]string{}, out[i].Actions...)
		out[i].Operations = append([]Operation{}, out[i].Operations...)
		for j := range out[i].Fields {
			out[i].Fields[j].Options = append([]string{}, out[i].Fields[j].Options...)
		}
	}
	return out
}

func RegisterJob(d JobDefinition) error { return Default.RegisterJob(d) }
func (r *Registry) RegisterJob(d JobDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return errors.New("module registry frozen")
	}
	if !identifier.MatchString(d.Type) || d.Type == "export" || d.Run == nil || len(d.ResultFields) > 32 {
		return errors.New("invalid job registration")
	}
	if _, ok := r.jobs[d.Type]; ok {
		return errors.New("duplicate job type")
	}
	found := false
	for _, m := range r.modules {
		if m.Key == d.Resource && slices.Contains(m.Actions, d.Action) {
			found = true
		}
	}
	if !found {
		return errors.New("unknown job authority")
	}
	seen := map[string]bool{}
	for _, f := range d.ResultFields {
		if !identifier.MatchString(f.Key) || seen[f.Key] || f.Sensitive || slices.Contains([]string{"password", "secret", "token", "credentials", "email", "mfa", "recovery_code"}, f.Key) || !slices.Contains([]string{"string", "text", "number", "boolean", "enum", "date"}, f.Type) {
			return errors.New("invalid job result field")
		}
		seen[f.Key] = true
		if e := validateOptions(f); e != nil {
			return e
		}
	}
	d.ResultFields = cloneFields(d.ResultFields)
	r.jobs[d.Type] = d
	return nil
}
func (r *Registry) Job(key string) (JobDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.jobs[key]
	d.ResultFields = cloneFields(d.ResultFields)
	return d, ok
}
func (r *Registry) Jobs() []JobDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []JobDefinition{}
	for _, d := range r.jobs {
		d.ResultFields = cloneFields(d.ResultFields)
		out = append(out, d)
	}
	slices.SortFunc(out, func(a, b JobDefinition) int { return strings.Compare(a.Type, b.Type) })
	return out
}
func cloneFields(fs []Field) []Field {
	out := append([]Field{}, fs...)
	for i := range out {
		out[i].Options = append([]string{}, out[i].Options...)
	}
	return out
}

func validateOptions(f Field) error {
	if f.Type == "enum" {
		if len(f.Options) == 0 || len(f.Options) > 100 {
			return errors.New("enum options required")
		}
		options := map[string]bool{}
		for _, option := range f.Options {
			if strings.TrimSpace(option) == "" || len(option) > 120 || options[option] {
				return errors.New("invalid enum option")
			}
			options[option] = true
		}
	} else if len(f.Options) > 0 {
		return errors.New("options require enum field")
	}
	return nil
}
