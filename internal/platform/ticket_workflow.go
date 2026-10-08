package platform

import (
	"errors"
	"strings"
)

func validTransition(from, to string) bool {
	edges := map[string][]string{"open": {"in_progress"}, "in_progress": {"resolved"}, "resolved": {"closed", "in_progress"}, "closed": {"open"}}
	for _, s := range edges[from] {
		if to == s {
			return true
		}
	}
	return false
}
func transitionResource(q *request) (any, error) {
	m, e := q.module()
	if e != nil {
		return nil, e
	}
	if m != "tickets" {
		return nil, notFound()
	}
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	var in struct {
		Version    int     `json:"version"`
		Status     string  `json:"status"`
		Resolution *string `json:"resolution,omitempty"`
	}
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	data := map[string]any{"status": in.Status}
	if in.Resolution != nil {
		data["resolution"] = *in.Resolution
	}
	r, e := q.updateRecord(m, id, resourceInput{Version: in.Version, Data: data}, "update")
	if e != nil {
		return nil, e
	}
	return q.visible(m, "read", r), nil
}

func ticketTransitions(from string) []string {
	out := []string{}
	for _, to := range []string{"open", "in_progress", "resolved", "closed"} {
		if validTransition(from, to) {
			out = append(out, to)
		}
	}
	return out
}

type resourcePolicy struct {
	Create      func(map[string]any) error
	Change      func(Record, map[string]any) error
	Transitions func(Principal, Record) []string
}

var policies = map[string]resourcePolicy{"tickets": {
	Create: func(data map[string]any) error {
		if status, ok := data["status"]; ok && status != "open" {
			return errors.New("new tickets must start open")
		}
		if _, ok := data["status"]; !ok {
			data["status"] = "open"
		}
		if _, ok := data["priority"]; !ok {
			data["priority"] = "normal"
		}
		return nil
	},
	Change: func(old Record, data map[string]any) error {
		if data["status"] == "resolved" && old.Data["status"] != "resolved" {
			summary, ok := data["resolution"].(string)
			if !ok || strings.TrimSpace(summary) == "" {
				return validation("A fresh resolution summary is required")
			}
		}
		if status, ok := data["status"]; ok && status != old.Data["status"] && !validTransition(stringValue(old.Data["status"]), stringValue(status)) {
			return validation("Invalid ticket status transition")
		}
		return nil
	},
	Transitions: func(p Principal, old Record) []string {
		out := []string{}
		for _, status := range ticketTransitions(stringValue(old.Data["status"])) {
			delta := map[string]any{"status": status}
			if status == "resolved" {
				delta["resolution"] = ""
			}
			next := old
			next.Data = cloneJSONMap(old.Data)
			next.Data["status"] = status
			if CanWrite(p, "tickets", "update", old, delta) && CanWrite(p, "tickets", "update", next, delta) {
				out = append(out, status)
			}
		}
		return out
	},
}}
