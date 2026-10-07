package platform

import (
	"context"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"
)

type storageClaim struct{ id, key, token string }

// Journal ownership is decided in a short transaction. Storage I/O holds no pool connection.
func (a *App) claimStorage(ctx context.Context, key string) (*storageClaim, error) {
	tx, e := a.LimitsDB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	c := &storageClaim{}
	var tenant string
	where := "next_attempt_at<=now()"
	args := []any{}
	if key != "" {
		where = "storage_key=$1"
		args = append(args, key)
	}
	e = tx.QueryRow(ctx, "SELECT id::text,storage_key,coalesce(tenant_id::text,'') FROM storage_gc WHERE "+where+" AND (lease_until IS NULL OR lease_until<now()) ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT 1", args...).Scan(&c.id, &c.key, &tenant)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if tenant != "" {
		if _, e = tx.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", tenant); e != nil {
			return nil, e
		}
		var referenced bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM files WHERE tenant_id=$1 AND storage_key=$2) OR EXISTS(SELECT FROM jobs WHERE tenant_id=$1 AND result_key=$2)", tenant, c.key).Scan(&referenced); e != nil {
			return nil, e
		}
		if referenced {
			_, e = tx.Exec(ctx, "DELETE FROM storage_gc WHERE id=$1", c.id)
			if e != nil {
				return nil, e
			}
			return nil, tx.Commit(ctx)
		}
	}
	c.token = digest(randomToken())
	if _, e = tx.Exec(ctx, "UPDATE storage_gc SET claim_token=$1,lease_until=now()+interval '10 seconds' WHERE id=$2", c.token, c.id); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return c, nil
}
func (a *App) cleanupStorage(ctx context.Context, key string) {
	c, e := a.claimStorage(ctx, key)
	if e != nil {
		a.workerOutcome("gc", stageClaim, e)
		return
	}
	if c == nil {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	deletion := a.storage().Delete(deleteCtx, c.key)
	cancel()
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if deletion == nil {
		_, e = a.LimitsDB.Exec(finish, "DELETE FROM storage_gc WHERE id=$1 AND claim_token=$2", c.id, c.token)
	} else {
		_, e = a.LimitsDB.Exec(finish, "UPDATE storage_gc SET attempts=attempts+1,claim_token=NULL,lease_until=NULL,next_attempt_at=now()+interval '5 minutes',error_stage=$3,error_code=$4 WHERE id=$1 AND claim_token=$2", c.id, c.token, string(stageCleanup), string(safeDiagnostic(stageCleanup, deletion)))
	}
	a.workerOutcome("gc", stageCleanup, deletion)
	if e != nil {
		a.workerOutcome("gc", stagePublish, e)
	}
}
func (a *App) collectOneFile(ctx context.Context) { a.cleanupStorage(ctx, "") }
func (a *App) deleteOrQueue(key string) {
	if key == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	a.cleanupStorage(ctx, key)
}

// The claim fence prevents an expired reservation from being published after cleanup begins.
func (q *request) consumeReservation(key string) error {
	tag, e := q.tx.Exec(q.ctx(), "DELETE FROM storage_gc WHERE id=$1 AND tenant_id=$2 AND claim_token IS NULL AND next_attempt_at>now()", key, q.p.TenantID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return conflict()
	}
	return nil
}
