package platform

import (
	"encoding/json"

	"github.com/ljn-forge/tenantforge/extension"
)

type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}
type Grant struct {
	Resource      string      `json:"resource"`
	Action        string      `json:"action"`
	Scope         string      `json:"scope"`
	DepartmentIDs []string    `json:"department_ids,omitempty"`
	ReadFields    []string    `json:"read_fields,omitempty"`
	WriteFields   []string    `json:"write_fields,omitempty"`
	UnmaskFields  []string    `json:"unmask_fields,omitempty"`
	Conditions    []Condition `json:"conditions,omitempty"`
}
type Field = extension.Field
type Module = extension.Module
type Record = extension.Record
type Capabilities = extension.Capabilities
type Principal struct {
	MustChangePassword                              bool
	ID, TenantID, DepartmentID, MemberID, SessionID string
	PlatformAdmin                                   bool
	Departments                                     map[string]string
	Grants                                          []Grant
}

// OptionalString distinguishes omission (preserve) from explicit null (clear).
type OptionalString struct {
	Present bool
	Value   string
}

func (o *OptionalString) UnmarshalJSON(b []byte) error {
	o.Present = true
	if string(b) == "null" {
		o.Value = ""
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}
