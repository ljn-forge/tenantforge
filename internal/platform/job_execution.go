package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"

	"github.com/ljn-forge/tenantforge/extension"
)

type jobRun struct {
	id, tenant, claim, actor, kind, resource, action, authority string
	payload                                                     []byte
	attempts, max                                               int
}

func (a *App) runOneJob(ctx context.Context) {
	j, e := a.claimJob(ctx)
	if e != nil {
		a.workerOutcome("jobs", stageClaim, e)
		return
	}
	if j != nil {
		a.executeJob(ctx, j)
	}
}
func (a *App) claimJob(ctx context.Context) (*jobRun, error) {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	j := &jobRun{}
	e = tx.QueryRow(ctx, "SELECT job_id::text,tenant_id::text FROM job_leases WHERE next_attempt_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY next_attempt_at LIMIT 1").Scan(&j.id, &j.tenant)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", j.tenant); e != nil {
		return nil, e
	}
	var status string
	if e = tx.QueryRow(ctx, "SELECT actor_id::text,type,payload,status,attempts,max_attempts FROM jobs WHERE id=$1 AND tenant_id=$2 FOR UPDATE SKIP LOCKED", j.id, j.tenant).Scan(&j.actor, &j.kind, &j.payload, &status, &j.attempts, &j.max); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, e
	}
	if status == "succeeded" || status == "failed" || status == "cancelled" || j.attempts >= j.max {
		if j.attempts >= j.max {
			if _, e = tx.Exec(ctx, "UPDATE jobs SET status='failed',error='Retry limit reached',error_code='retry_exhausted',error_stage='claim',updated_at=now() WHERE id=$1 AND status IN ('pending','running')", j.id); e != nil {
				return nil, e
			}
		}
		if _, e = tx.Exec(ctx, "DELETE FROM job_leases WHERE job_id=$1", j.id); e != nil {
			return nil, e
		}
		return nil, tx.Commit(ctx)
	}
	var leaseID string
	e = tx.QueryRow(ctx, "SELECT job_id::text FROM job_leases WHERE job_id=$1 AND next_attempt_at<=now() AND (lease_until IS NULL OR lease_until<now()) FOR UPDATE SKIP LOCKED", j.id).Scan(&leaseID)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	j.claim = digest(randomToken())
	j.attempts++
	if _, e = tx.Exec(ctx, "UPDATE job_leases SET claim_token=$1,lease_until=now()+interval '45 seconds' WHERE job_id=$2", j.claim, j.id); e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, "UPDATE jobs SET status='running',attempts=attempts+1,error='',error_code='',error_stage='',updated_at=now() WHERE id=$1", j.id); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return j, nil
}
func (a *App) executeJob(ctx context.Context, j *jobRun) {
	execCtx, cancel := context.WithCancel(ctx)
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-execCtx.Done():
				return
			case <-ticker.C:
				var valid bool
				probe, stop := context.WithTimeout(execCtx, time.Second)
				e := a.LimitsDB.QueryRow(probe, "SELECT EXISTS(SELECT FROM job_leases WHERE job_id=$1 AND claim_token=$2)", j.id, j.claim).Scan(&valid)
				stop()
				if e != nil || !valid {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-monitorDone }()
	stage := stagePrepare
	var data []byte
	result := map[string]any{}
	var runErr error
	var definition extension.JobDefinition
	if j.kind == "export" {
		var p exportPayload
		runErr = json.Unmarshal(j.payload, &p)
		j.resource = p.Module
		j.action = "export"
		if runErr == nil {
			service := j.service(a, execCtx)
			runErr = service.withRequest(false, func(q *request) error { var e error; data, e = q.exportCSV(p); return e })
		}
	} else {
		var exists bool
		definition, exists = moduleRegistry.Job(j.kind)
		if !exists {
			runErr = validation("Unknown job type")
		} else {
			j.resource, j.action = definition.Resource, definition.Action
			service := j.service(a, execCtx)
			runErr = service.withRequest(false, func(q *request) error { return nil })
			if runErr == nil {
				stage = stageHandler
				result, runErr = invokeJob(definition, service)
				if runErr == nil {
					runErr = validateJobResult(definition, result)
				}
			}
		}
	}
	key := ""
	published := false
	defer func() {
		if key != "" && !published {
			a.deleteOrQueue(key)
		}
	}()
	if runErr == nil && j.kind == "export" {
		stage = stageReserve
		key = uuid.NewString()
		runErr = j.service(a, execCtx).withRequest(true, func(q *request) error { return q.reserveStorage(key, int64(len(data))) })
		if runErr == nil {
			stage = stageStorage
			runErr = a.storage().Put(execCtx, key, bytes.NewReader(data), int64(len(data)), "text/csv")
		}
	}
	if execCtx.Err() != nil && runErr == nil {
		runErr = execCtx.Err()
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer finishCancel()
	published, e := a.finishJob(finishCtx, j, key, int64(len(data)), result, stage, runErr)
	if e != nil {
		a.workerOutcome("jobs", stagePublish, e)
	}
}
func (j *jobRun) service(a *App, ctx context.Context) *jobService {
	return &jobService{app: a, ctx: ctx, id: j.id, tenant: j.tenant, actor: j.actor, claim: j.claim, resource: j.resource, action: j.action, payload: append([]byte{}, j.payload...), authority: &j.authority}
}
func (a *App) finishJob(ctx context.Context, j *jobRun, key string, size int64, result map[string]any, stage diagnosticStage, execution error) (bool, error) {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	r, _ := http.NewRequestWithContext(ctx, "POST", "http://worker/", nil)
	q := &request{app: a, tx: tx, r: r, p: Principal{ID: j.actor, TenantID: j.tenant}, requestID: j.id}
	// Lock order matches request mutations: quota/invariant, membership, grants, then job/lease.
	auth := q.loadTenant(j.tenant)
	if auth == nil {
		auth = q.require(j.resource, j.action)
	}
	if auth == nil {
		var mustChange bool
		auth = tx.QueryRow(ctx, "SELECT must_change_password FROM users WHERE id=$1 FOR SHARE", j.actor).Scan(&mustChange)
		if auth == nil && mustChange {
			auth = forbidden()
		}
	}
	if auth == nil && j.authority != "" && jobAuthority(q.p, j.resource, j.action) != j.authority {
		auth = forbidden()
	}
	if _, e = tx.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", j.tenant); e != nil {
		return false, e
	}
	var valid bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM job_leases WHERE job_id=$1 AND claim_token=$2)", j.id, j.claim).Scan(&valid); e != nil {
		return false, e
	}
	if !valid {
		return false, nil
	}
	if execution == nil {
		execution = auth
		if auth != nil {
			stage = stageAuthorize
		}
	}
	if execution == nil && key != "" {
		execution = q.checkQuota("storage_bytes", 0)
		if execution == nil {
			execution = q.consumeReservation(key)
		}
		if execution != nil {
			stage = stagePublish
		}
	}
	status := "succeeded"
	code := ""
	safe := ""
	storedStage := ""
	terminal := true
	if execution != nil {
		code = string(safeDiagnostic(stage, execution))
		safe = "Job execution failed"
		storedStage = string(stage)
		var typed *extension.Error
		terminal = j.attempts >= j.max || code == "permission_revoked" || code == "validation_failed" || (errors.As(execution, &typed) && typed.Permanent)
		status = "pending"
		if terminal {
			status = "failed"
		}
		key = ""
		size = 0
		result = map[string]any{}
	}
	if result == nil {
		result = map[string]any{}
	}
	tag, e := tx.Exec(ctx, "UPDATE jobs SET status=$1,error=$2,error_code=$3,error_stage=$4,result_key=$5,result_bytes=$6,result=$7,authority_hash=$11,authority_resource=$12,authority_action=$13,updated_at=now() WHERE id=$8 AND tenant_id=$9 AND status='running' AND EXISTS(SELECT FROM job_leases WHERE job_id=$8 AND claim_token=$10)", status, safe, code, storedStage, key, size, jsonBytes(result), j.id, j.tenant, j.claim, j.authority, j.resource, j.action)
	if e != nil {
		return false, e
	}
	if tag.RowsAffected() != 1 {
		return false, conflict()
	}
	if terminal {
		_, e = tx.Exec(ctx, "DELETE FROM job_leases WHERE job_id=$1 AND claim_token=$2", j.id, j.claim)
	} else {
		_, e = tx.Exec(ctx, "UPDATE job_leases SET claim_token=NULL,lease_until=NULL,next_attempt_at=now()+($1*interval '5 seconds') WHERE job_id=$2 AND claim_token=$3", j.attempts, j.id, j.claim)
	}
	if e != nil {
		return false, e
	}
	if e = q.audit(status, "jobs", j.id, map[string]any{"type": j.kind, "error_stage": storedStage, "error_code": code}); e != nil {
		return false, e
	}
	if e = tx.Commit(ctx); e != nil {
		return false, e
	}
	a.workerOutcome("jobs", stage, execution)
	return status == "succeeded", nil
}

func invokeJob(d extension.JobDefinition, c extension.JobContext) (result map[string]any, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = errors.New("job handler failed")
		}
	}()
	return d.Run(c)
}
