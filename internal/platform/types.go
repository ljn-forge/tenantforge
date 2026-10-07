package platform

import (
	"encoding/json"
	"time"
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
type Field struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	LabelEN   string   `json:"label_en"`
	Type      string   `json:"type"`
	Required  bool     `json:"required,omitempty"`
	Sensitive bool     `json:"sensitive,omitempty"`
	Readonly  bool     `json:"readonly,omitempty"`
	Options   []string `json:"options,omitempty"`
}
type Module struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	LabelEN     string   `json:"label_en"`
	Description string   `json:"description"`
	Fields      []Field  `json:"fields"`
	Actions     []string `json:"actions"`
}
type Record struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	OwnerID      string         `json:"owner_id"`
	DepartmentID string         `json:"department_id"`
	Version      int            `json:"version"`
	Data         map[string]any `json:"data"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}
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
