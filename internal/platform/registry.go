package platform

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
	frozen  bool
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

func NewRegistry() *Registry { return &Registry{modules: cloneModules(Modules)} }

var moduleRegistry = NewRegistry()

// RegisterModule is the typed extension point. Call it during startup before New.
func RegisterModule(m Module) error { return moduleRegistry.Register(m) }
func (r *Registry) Register(m Module) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return errors.New("module registry frozen")
	}
	if !identifier.MatchString(m.Key) || m.Label == "" || m.LabelEN == "" || len(m.Fields) == 0 || len(m.Fields) > 100 || len(m.Actions) == 0 {
		return errors.New("invalid module descriptor")
	}
	if _, ok := resourceActions[m.Key]; ok || slices.Contains([]string{"auth", "users", "sessions", "tenants", "platform", "setup", "permissions", "dashboard"}, m.Key) {
		return errors.New("reserved module key")
	}
	for _, old := range r.modules {
		if old.Key == m.Key {
			return errors.New("duplicate module key")
		}
	}
	seen := map[string]bool{}
	for _, a := range m.Actions {
		if !slices.Contains([]string{"read", "create", "update", "delete", "assign", "export", "import"}, a) || seen[a] {
			return errors.New("unsupported or duplicate action")
		}
		seen[a] = true
	}
	seen = map[string]bool{}
	for _, f := range m.Fields {
		if !identifier.MatchString(f.Key) || seen[f.Key] || slices.Contains([]string{"id", "tenant_id", "owner_id", "department_id", "version", "created_at", "updated_at"}, f.Key) || f.Label == "" || f.LabelEN == "" || !slices.Contains([]string{"string", "text", "number", "boolean", "enum", "date"}, f.Type) {
			return errors.New("invalid, reserved or duplicate field")
		}
		seen[f.Key] = true
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
		for j := range out[i].Fields {
			out[i].Fields[j].Options = append([]string{}, out[i].Fields[j].Options...)
		}
	}
	return out
}
func resourceModules() []Module { return moduleRegistry.Modules() }

func cloneModules(ms []Module) []Module {
	out := append([]Module{}, ms...)
	for i := range out {
		out[i].Fields = append([]Field{}, out[i].Fields...)
		out[i].Actions = append([]string{}, out[i].Actions...)
		for j := range out[i].Fields {
			out[i].Fields[j].Options = append([]string{}, out[i].Fields[j].Options...)
		}
	}
	return out
}
