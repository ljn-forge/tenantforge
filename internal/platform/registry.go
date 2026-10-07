package platform

import (
	"regexp"

	"github.com/ljn-forge/tenantforge/extension"
)

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

type Registry = extension.Registry

func NewRegistry() *Registry {
	r := extension.NewRegistry()
	for _, m := range Modules {
		if e := r.Register(m); e != nil {
			panic("invalid builtin module")
		}
	}
	return r
}

var moduleRegistry = func() *Registry {
	for _, m := range Modules {
		if e := extension.RegisterModule(m); e != nil {
			panic("invalid builtin module")
		}
	}
	return extension.Default
}()

func RegisterModule(m Module) error { return moduleRegistry.Register(m) }
func resourceModules() []Module     { return moduleRegistry.Modules() }
