package platform

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ljn-forge/tenantforge/extension"
)

type diagnosticStage string
type diagnosticCode string

const (
	stageClaim        diagnosticStage = "claim"
	stageAuthorize    diagnosticStage = "authorize"
	stagePrepare      diagnosticStage = "prepare"
	stageReserve      diagnosticStage = "reserve"
	stageStorage      diagnosticStage = "storage"
	stagePublish      diagnosticStage = "publish"
	stageHandler      diagnosticStage = "handler"
	stageCleanup      diagnosticStage = "cleanup"
	stageDeliver      diagnosticStage = "deliver"
	stageHousekeeping diagnosticStage = "housekeeping"
)

func safeDiagnostic(stage diagnosticStage, e error) diagnosticCode {
	if e == nil {
		return "ok"
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(e, context.Canceled) {
		return "cancelled"
	}
	switch errorCode(e) {
	case "forbidden", "unauthorized", "password_change_required":
		return "permission_revoked"
	case "rate_limited":
		return "quota_exceeded"
	case "validation":
		return "validation_failed"
	case "conflict":
		return "publish_conflict"
	}
	var typed *extension.Error
	if errors.As(e, &typed) {
		switch typed.Code {
		case "validation_failed", "permission_revoked", "quota_exceeded", "handler_failed":
			return diagnosticCode(typed.Code)
		}
	}
	switch stage {
	case stageStorage, stageCleanup:
		return "storage_unavailable"
	case stageDeliver:
		return "provider_unavailable"
	case stageHandler:
		return "handler_failed"
	default:
		return "database_unavailable"
	}
}
func (a *App) workerOutcome(lane string, stage diagnosticStage, e error) {
	code := safeDiagnostic(stage, e)
	if a.metricsState != nil {
		a.metricsState.workers.WithLabelValues(lane, string(stage), string(code)).Inc()
	}
	if e != nil {
		slog.Warn("worker operation failed", "lane", lane, "stage", stage, "code", code)
	}
}
func (a *App) startWorkers(ctx context.Context) {
	for _, lane := range []struct {
		name    string
		timeout time.Duration
		run     func(context.Context)
	}{{"jobs", 25 * time.Second, a.runOneJob}, {"mail", 20 * time.Second, a.sendOneMail}, {"gc", 4 * time.Second, a.collectOneFile}, {"housekeeping", 3 * time.Second, a.housekeeping}} {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			ticker := time.NewTicker(400 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					start := time.Now()
					taskCtx, cancel := context.WithTimeout(ctx, lane.timeout)
					lane.run(taskCtx)
					cancel()
					if a.metricsState != nil {
						a.metricsState.workerDuration.WithLabelValues(lane.name).Observe(time.Since(start).Seconds())
					}
				}
			}
		}()
	}
}
func (a *App) housekeeping(ctx context.Context) {
	if e := a.flushSessionActivity(ctx); e != nil {
		a.workerOutcome("housekeeping", stageHousekeeping, e)
	}
	for _, sql := range []string{"DELETE FROM login_attempts WHERE key IN (SELECT key FROM login_attempts WHERE window_at<now()-interval '1 day' LIMIT 1000)", "UPDATE mail_outbox SET status='failed',error_stage='claim',error_code='retry_exhausted' WHERE status='pending' AND attempts>=8 AND (lease_until IS NULL OR lease_until<now())", "DELETE FROM oidc_flows WHERE expires_at<now()", "DELETE FROM reset_tokens WHERE expires_at<now()", "DELETE FROM sessions WHERE expires_at<now()"} {
		if _, e := a.DB.Exec(ctx, sql); e != nil {
			a.workerOutcome("housekeeping", stageHousekeeping, e)
			return
		}
	}
	a.workerOutcome("housekeeping", stageHousekeeping, a.updateBacklog(ctx))
}
