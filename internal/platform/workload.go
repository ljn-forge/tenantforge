package platform

// Aggregates use the same grant predicate as lists plus the authority to read each measured field.
func (q *request) workload() (map[string]int, error) {
	out := map[string]int{}
	for _, metric := range []struct{ key, field, value string }{{"open_tickets", "status", "open"}, {"urgent_tickets", "priority", "urgent"}} {
		b := &predicate{}
		where := b.readField(q.p, "tickets", "read", "r", "r.owner_id", "r.department_id", "r.data", Field{Key: metric.field}, true)
		if where == "false" {
			continue
		}
		where += " AND r.module=" + b.arg("tickets") + " AND r.data->>" + b.arg(metric.field) + "=" + b.arg(metric.value)
		var n int
		if e := q.tx.QueryRow(q.ctx(), "SELECT count(*) FROM resources r WHERE "+where, b.Args...).Scan(&n); e != nil {
			return nil, e
		}
		out[metric.key] = n
	}
	for _, metric := range []struct{ key, resource, from, alias, owner, dep, status string }{{"running_jobs", "jobs", "jobs j", "j", "j.actor_id", "j.department_id", "running"}, {"failed_jobs", "jobs", "jobs j", "j", "j.actor_id", "j.department_id", "failed"}, {"members", "members", "memberships m", "m", "m.user_id", "m.department_id", ""}} {
		b := &predicate{}
		where := b.authorized(q.p, metric.resource, "read", metric.alias, metric.owner, metric.dep, "")
		if where == "false" {
			continue
		}
		if metric.status != "" {
			where += " AND j.status=" + b.arg(metric.status)
		}
		var n int
		if e := q.tx.QueryRow(q.ctx(), "SELECT count(*) FROM "+metric.from+" WHERE "+where, b.Args...).Scan(&n); e != nil {
			return nil, e
		}
		out[metric.key] = n
	}
	return out, nil
}
