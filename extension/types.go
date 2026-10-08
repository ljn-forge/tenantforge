// Package extension provides validated resource, operation and job registration without database or HTTP access.
package extension

import (
	"context"
	"encoding/json"
	"time"
)

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
type OperationContext struct {
	Context           context.Context
	TenantID, ActorID string
	Record            Record
	Data              map[string]any
}
type Operation struct {
	Key     string                                         `json:"key"`
	Label   string                                         `json:"label"`
	LabelEN string                                         `json:"label_en"`
	Action  string                                         `json:"-"`
	Apply   func(OperationContext) (map[string]any, error) `json:"-"`
}
type Module struct {
	DescriptionEN string      `json:"description_en,omitempty"`
	Operations    []Operation `json:"operations,omitempty"`
	Key           string      `json:"key"`
	Label         string      `json:"label"`
	LabelEN       string      `json:"label_en"`
	Description   string      `json:"description"`
	Fields        []Field     `json:"fields"`
	Actions       []string    `json:"actions"`
}
type Record struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	OwnerID      string         `json:"owner_id"`
	DepartmentID string         `json:"department_id"`
	Version      int            `json:"version"`
	Data         map[string]any `json:"data"`
	Capabilities Capabilities   `json:"capabilities"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type Capabilities struct {
	ReplyKinds  []string `json:"reply_kinds,omitempty"`
	Actions     []string `json:"actions"`
	WriteFields []string `json:"write_fields"`
	Transitions []string `json:"transitions"`
}
type JobContext interface {
	Context() context.Context
	ID() string
	TenantID() string
	ActorID() string
	Payload() json.RawMessage
	WalkRecords(func(Record) error) error
	UpdateRecord(string, int, map[string]any) (Record, error)
}
type JobDefinition struct {
	Type, Resource, Action                     string
	Label, LabelEN, Description, DescriptionEN string
	// Nil keeps the job API-only; an explicit empty slice declares an empty UI form.
	InputFields  []Field
	Validate     func(json.RawMessage) error
	ResultFields []Field
	Run          func(JobContext) (map[string]any, error)
}

// Error exposes only registered, stable diagnostic codes. Provider errors must never become public messages.
type Error struct {
	Code      string
	Permanent bool
}

func (e *Error) Error() string { return "extension operation failed" }
