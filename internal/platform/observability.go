package platform

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metricsState struct {
	registry       *prometheus.Registry
	requests       *prometheus.CounterVec
	duration       *prometheus.HistogramVec
	workers        *prometheus.CounterVec
	workerDuration *prometheus.HistogramVec
	backlog        *prometheus.GaugeVec
	handler        http.Handler
}

func newMetrics(a *App) *metricsState {
	m := &metricsState{registry: prometheus.NewRegistry(), requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tenantforge_http_requests_total", Help: "HTTP requests by registered route and status."}, []string{"route", "method", "status"}), duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tenantforge_http_request_duration_seconds", Help: "HTTP duration including authorized transfer.", Buckets: prometheus.DefBuckets}, []string{"route", "method"}), workers: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tenantforge_worker_outcomes_total", Help: "Bounded worker lane outcomes."}, []string{"lane", "stage", "code"}), workerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tenantforge_worker_duration_seconds", Help: "Worker lane iteration latency.", Buckets: prometheus.DefBuckets}, []string{"lane"}), backlog: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "tenantforge_queue_backlog", Help: "Durable queue items by lane and state."}, []string{"lane", "state"})}
	m.registry.MustRegister(m.requests, m.duration, m.workers, m.workerDuration, m.backlog, prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "tenantforge_requests_total", Help: "API requests."}, func() float64 { return float64(a.requests.Load()) }), prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "tenantforge_rejected_total", Help: "API rejected requests."}, func() float64 { return float64(a.failures.Load()) }))
	m.handler = promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
	return m
}
func (m *metricsState) observeHTTP(route, method string, status int, duration time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	if status == 0 {
		status = 200
	}
	m.requests.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(route, method).Observe(duration.Seconds())
}
func (a *App) metrics(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if !a.Config.MetricsPublic && (ip == nil || !ip.IsLoopback()) {
		writeError(w, "", forbidden())
		return
	}
	a.metricsState.handler.ServeHTTP(w, r)
}

type observedResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *observedResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *observedResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(b)
}
func (w *observedResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (a *App) updateBacklog(ctx context.Context) error {
	for _, item := range []struct{ lane, state, sql string }{{"jobs", "pending", "SELECT count(*) FROM job_leases WHERE claim_token IS NULL"}, {"jobs", "running", "SELECT count(*) FROM job_leases WHERE claim_token IS NOT NULL"}, {"mail", "pending", "SELECT count(*) FROM mail_outbox WHERE status='pending'"}, {"mail", "failed", "SELECT count(*) FROM mail_outbox WHERE status='failed'"}, {"gc", "pending", "SELECT count(*) FROM storage_gc"}} {
		var count int64
		if e := a.DB.QueryRow(ctx, item.sql).Scan(&count); e != nil {
			return e
		}
		a.metricsState.backlog.WithLabelValues(item.lane, item.state).Set(float64(count))
	}
	return nil
}
