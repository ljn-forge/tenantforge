package platform

import "github.com/ljn-forge/tenantforge/extension"

type jobTypeDescriptor struct {
	Type          string  `json:"type"`
	Label         string  `json:"label"`
	LabelEN       string  `json:"label_en"`
	Description   string  `json:"description,omitempty"`
	DescriptionEN string  `json:"description_en,omitempty"`
	Resource      string  `json:"resource"`
	Action        string  `json:"action"`
	InputFields   []Field `json:"input_fields"`
	ResultFields  []Field `json:"result_fields"`
}
type jobDisplayDefinition struct {
	Label         string  `json:"label"`
	LabelEN       string  `json:"label_en"`
	Description   string  `json:"description,omitempty"`
	DescriptionEN string  `json:"description_en,omitempty"`
	ResultFields  []Field `json:"result_fields"`
}

func displayJobDefinition(d extension.JobDefinition) jobDisplayDefinition {
	fields := d.ResultFields
	if fields == nil {
		fields = []Field{}
	}
	return jobDisplayDefinition{Label: envValue(d.Label, d.Type), LabelEN: envValue(d.LabelEN, d.Type), Description: d.Description, DescriptionEN: d.DescriptionEN, ResultFields: fields}
}
func jobTypes(q *request) (any, error) {
	if e := q.authorizeJobCreation(); e != nil {
		return nil, e
	}
	out := []jobTypeDescriptor{}
	for _, d := range moduleRegistry.Jobs() {
		if d.InputFields == nil || q.require(d.Resource, d.Action) != nil {
			continue
		}
		view := displayJobDefinition(d)
		out = append(out, jobTypeDescriptor{Type: d.Type, Label: view.Label, LabelEN: view.LabelEN, Description: view.Description, DescriptionEN: view.DescriptionEN, Resource: d.Resource, Action: d.Action, InputFields: d.InputFields, ResultFields: view.ResultFields})
	}
	return out, nil
}

func (q *request) authorizeJobCreation() error {
	if e := q.require("jobs", "create"); e != nil {
		return e
	}
	if !Allowed(q.p, "jobs", "create", q.controlRow(q.p.ID, q.p.DepartmentID)) {
		return forbidden()
	}
	return nil
}
