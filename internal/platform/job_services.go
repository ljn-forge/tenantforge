package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"

	"github.com/ljn-forge/tenantforge/extension"
)

func createJob(q *request) (any, error) {
	var in struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	definition, ok := moduleRegistry.Job(in.Type)
	if !ok {
		return nil, validation("Unknown job type")
	}
	var payload map[string]any
	if len(in.Payload) > 64<<10 || json.Unmarshal(in.Payload, &payload) != nil || payload == nil {
		return nil, validation("Job payload must be a bounded object")
	}
	if definition.Validate != nil {
		if e := definition.Validate(append(json.RawMessage{}, in.Payload...)); e != nil {
			return nil, validation("Invalid job payload")
		}
	}
	if e := q.require(definition.Resource, definition.Action); e != nil {
		return nil, e
	}
	if !Allowed(q.p, definition.Resource, definition.Action, q.controlRow(q.p.ID, q.p.DepartmentID)) {
		b := &predicate{}
		where := b.authorized(q.p, definition.Resource, definition.Action, "r", "r.owner_id", "r.department_id", "r.data") + " AND r.module=" + b.arg(definition.Resource)
		var allowed bool
		if e := q.tx.QueryRow(q.ctx(), "SELECT EXISTS(SELECT FROM resources r WHERE "+where+")", b.Args...).Scan(&allowed); e != nil {
			return nil, e
		}
		if !allowed {
			return nil, forbidden()
		}
	}
	if e := q.checkQuota("jobs", 1); e != nil {
		return nil, e
	}
	id := uuid.NewString()
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO jobs(id,tenant_id,actor_id,department_id,type,payload) VALUES($1,$2,$3,$4,$5,$6)", id, q.p.TenantID, q.p.ID, nullable(q.p.DepartmentID), in.Type, in.Payload); e != nil {
		return nil, e
	}
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO job_leases(job_id,tenant_id) VALUES($1,$2)", id, q.p.TenantID); e != nil {
		return nil, e
	}
	if e := q.audit("create", "jobs", id, map[string]any{"type": in.Type}); e != nil {
		return nil, e
	}
	row, e := q.job(id)
	if e != nil {
		return nil, e
	}
	return q.publicJob(row), nil
}

type jobService struct {
	app                                        *App
	ctx                                        context.Context
	id, tenant, actor, claim, resource, action string
	authority                                  *string
	payload                                    json.RawMessage
}

func (s *jobService) Context() context.Context { return s.ctx }
func (s *jobService) ID() string               { return s.id }
func (s *jobService) TenantID() string         { return s.tenant }
func (s *jobService) ActorID() string          { return s.actor }
func (s *jobService) Payload() json.RawMessage { return append(json.RawMessage{}, s.payload...) }
func (s *jobService) withRequest(write bool, fn func(*request) error) error {
	tx, e := s.app.DB.Begin(s.ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	method := "GET"
	if write {
		method = "POST"
	}
	r, _ := http.NewRequestWithContext(s.ctx, method, "http://worker/", nil)
	q := &request{app: s.app, tx: tx, r: r, p: Principal{ID: s.actor, TenantID: s.tenant}, requestID: s.id}
	if e = q.loadTenant(s.tenant); e != nil {
		return e
	}
	var gated bool
	if e = tx.QueryRow(s.ctx, "SELECT must_change_password FROM users WHERE id=$1 FOR SHARE", s.actor).Scan(&gated); e != nil {
		return e
	}
	if gated {
		return forbidden()
	}
	if s.authority != nil {
		fingerprint := jobAuthority(q.p, s.resource, s.action)
		if *s.authority == "" {
			*s.authority = fingerprint
		} else if *s.authority != fingerprint {
			return forbidden()
		}
	}
	var valid bool
	if e = tx.QueryRow(s.ctx, "SELECT EXISTS(SELECT FROM job_leases WHERE job_id=$1 AND claim_token=$2)", s.id, s.claim).Scan(&valid); e != nil {
		return e
	}
	if !valid {
		return context.Canceled
	}
	if e = q.require(s.resource, s.action); e != nil {
		return e
	}
	if e = fn(q); e != nil {
		return e
	}
	return tx.Commit(s.ctx)
}
func (s *jobService) WalkRecords(visit func(extension.Record) error) error {
	if visit == nil {
		return errors.New("visitor required")
	}
	cursor := "00000000-0000-0000-0000-000000000000"
	for {
		var batch []storedRecord
		e := s.withRequest(false, func(q *request) error {
			b := &predicate{}
			where := b.authorized(q.p, s.resource, s.action, "r", "r.owner_id", "r.department_id", "r.data") + " AND r.module=" + b.arg(s.resource) + " AND r.id>" + b.arg(cursor) + "::uuid"
			rows, e := q.rows(recordSelect+" FROM resources r WHERE "+where+" ORDER BY r.id LIMIT 100", b.Args...)
			if e != nil {
				return e
			}
			batch, e = decodeRecords(rows)
			for i := range batch {
				batch[i].Record = q.visible(s.resource, s.action, batch[i].Record)
			}
			return e
		})
		if e != nil {
			return e
		}
		if len(batch) == 0 {
			return nil
		}
		for _, r := range batch {
			if e = s.ctx.Err(); e != nil {
				return e
			}
			if e = visit(r.Record); e != nil {
				return e
			}
		}
		cursor = batch[len(batch)-1].ID
	}
}
func (s *jobService) UpdateRecord(id string, version int, data map[string]any) (extension.Record, error) {
	if !validID(id) {
		return Record{}, notFound()
	}
	var result Record
	e := s.withRequest(true, func(q *request) error {
		var done bool
		if e := q.tx.QueryRow(q.ctx(), "SELECT EXISTS(SELECT FROM job_record_effects WHERE tenant_id=$1 AND job_id=$2 AND record_id=$3 AND action=$4 AND input_hash=$5)", s.tenant, s.id, id, s.action, digest(string(jsonBytes(data)))).Scan(&done); e != nil {
			return e
		}
		if done {
			r, e := q.record(s.resource, id, false)
			if e != nil {
				return e
			}
			if !Allowed(q.p, s.resource, s.action, r) {
				return forbidden()
			}
			result = q.visible(s.resource, s.action, r)
			return nil
		}
		// Wait for the authoritative record first, then fence cancellation only for the short mutation phase.
		if _, e := q.record(s.resource, id, true); e != nil {
			return e
		}
		var jobID string
		if e := q.tx.QueryRow(q.ctx(), "SELECT id::text FROM jobs WHERE id=$1 AND tenant_id=$2 FOR KEY SHARE", s.id, s.tenant).Scan(&jobID); e != nil {
			return e
		}
		var lease string
		if e := q.tx.QueryRow(q.ctx(), "SELECT job_id::text FROM job_leases WHERE job_id=$1 AND tenant_id=$2 AND claim_token=$3 AND lease_until>clock_timestamp() FOR SHARE", s.id, s.tenant, s.claim).Scan(&lease); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return context.Canceled
			}
			return e
		}
		r, e := q.updateRecord(s.resource, id, resourceInput{Version: version, Data: cloneJSONMap(data)}, s.action)
		if e != nil {
			return e
		}
		if _, e = q.tx.Exec(q.ctx(), "INSERT INTO job_record_effects(tenant_id,job_id,record_id,action,input_hash) VALUES($1,$2,$3,$4,$5)", s.tenant, s.id, id, s.action, digest(string(jsonBytes(data)))); e != nil {
			return e
		}
		result = q.visible(s.resource, s.action, r)
		return nil
	})
	return result, e
}
func validateJobResult(d extension.JobDefinition, result map[string]any) error {
	if result == nil {
		result = map[string]any{}
	}
	encoded, e := json.Marshal(result)
	if e != nil {
		return validation("Invalid typed job result")
	}
	if len(encoded) > 16<<10 {
		return validation("Job result exceeds limit")
	}
	for key, value := range result {
		found := false
		for _, f := range d.ResultFields {
			if f.Key == key && !f.Sensitive && validValue(f, value) {
				found = true
			}
		}
		if !found {
			return validation("Invalid job result metadata")
		}
	}
	for _, f := range d.ResultFields {
		if f.Required {
			if _, ok := result[f.Key]; !ok {
				return validation("Required job result metadata missing")
			}
		}
	}
	return nil
}

func jobAuthority(p Principal, resource, action string) string {
	canonical := []string{}
	actor, department := "", ""
	tree := false
	for _, g := range p.Grants {
		if !actionGranted(g, resource, action) {
			continue
		}
		canonical = append(canonical, string(jsonBytes(g)))
		if g.Scope == "self" {
			actor = p.ID
		}
		if g.Scope == "department" || g.Scope == "department_tree" {
			department = p.DepartmentID
		}
		if g.Scope == "department_tree" {
			tree = true
		}
	}
	slices.Sort(canonical)
	departments := []string{}
	if tree && p.DepartmentID != "" {
		departments = append(departments, p.DepartmentID)
		for id := range p.Departments {
			seen := map[string]bool{}
			for d := id; d != "" && !seen[d]; d = p.Departments[d] {
				if d == p.DepartmentID {
					if id != p.DepartmentID {
						departments = append(departments, id)
					}
					break
				}
				seen[d] = true
			}
		}
		slices.Sort(departments)
	}
	return digest(string(jsonBytes(struct {
		Actor, Department   string
		Departments, Grants []string
	}{actor, department, departments, canonical})))
}
