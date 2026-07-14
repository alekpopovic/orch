package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	registry *prometheus.Registry

	apiRequests         *prometheus.CounterVec
	apiRequestDuration  *prometheus.HistogramVec
	schedulerRuns       prometheus.Counter
	schedulerErrors     prometheus.Counter
	schedulerAttempts   prometheus.Counter
	schedulerFailures   prometheus.Counter
	tasksClaimed        prometheus.Counter
	assignmentConflicts prometheus.Counter
	schedulerDuration   prometheus.Histogram
	reconcilerRuns      prometheus.Counter
	reconcilerErrors    prometheus.Counter
	reconcilerDuration  prometheus.Histogram
	tasksCreated        prometheus.Counter
	tasksFailed         prometheus.Counter
	rollouts            prometheus.Counter
	rolloutFailures     prometheus.Counter
	autoscalerDecisions *prometheus.CounterVec
	autoscalerErrors    prometheus.Counter
	autoscalerRecommend *prometheus.GaugeVec
	leaderStatus        *prometheus.GaugeVec
	leaderFailures      *prometheus.CounterVec
}

func NewServer() *Server {
	m := &Server{
		registry: prometheus.NewRegistry(),
		apiRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "api_requests_total",
			Help: "Total API requests by method, route, and status code.",
		}, []string{"method", "route", "status"}),
		apiRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "api_request_duration_seconds",
			Help:    "API request duration in seconds by method, route, and status code.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
		schedulerRuns: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scheduler_runs_total",
			Help: "Total scheduler runs.",
		}),
		schedulerErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scheduler_errors_total",
			Help: "Total scheduler run errors.",
		}),
		schedulerAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scheduler_scheduling_attempts_total",
			Help: "Total scheduler attempts to compute and persist assignments.",
		}),
		schedulerFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scheduler_scheduling_failures_total",
			Help: "Total scheduler attempts that failed before completion.",
		}),
		tasksClaimed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scheduler_tasks_claimed_total",
			Help: "Total pending tasks atomically claimed by the scheduler.",
		}),
		assignmentConflicts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scheduler_assignment_conflicts_total",
			Help: "Total scheduler assignment claim conflicts.",
		}),
		schedulerDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "scheduler_duration_seconds",
			Help:    "Scheduler run duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		reconcilerRuns: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "reconciler_runs_total",
			Help: "Total reconciler runs.",
		}),
		reconcilerErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "reconciler_errors_total",
			Help: "Total reconciler run errors.",
		}),
		reconcilerDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "reconciler_duration_seconds",
			Help:    "Reconciler run duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		tasksCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tasks_created_total",
			Help: "Total tasks created by controllers.",
		}),
		tasksFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tasks_failed_total",
			Help: "Total tasks reported failed.",
		}),
		rollouts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "rollouts_total",
			Help: "Total rollout or rollback requests accepted.",
		}),
		rolloutFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "rollout_failures_total",
			Help: "Total rollouts marked failed.",
		}),
		autoscalerDecisions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "autoscaler_decisions_total",
			Help: "Total autoscaler decisions by decision type.",
		}, []string{"decision"}),
		autoscalerErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "autoscaler_errors_total",
			Help: "Total autoscaler controller errors.",
		}),
		autoscalerRecommend: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "autoscaler_recommendation_replicas",
			Help: "Latest autoscaler replica recommendation by service.",
		}, []string{"service_id"}),
		leaderStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "controller_leader_status",
			Help: "Controller leader status by controller name; 1 when this process is leader.",
		}, []string{"controller"}),
		leaderFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "controller_leader_acquisition_failures_total",
			Help: "Total controller leader lock acquisition failures by controller name.",
		}, []string{"controller"}),
	}
	m.registry.MustRegister(
		m.apiRequests,
		m.apiRequestDuration,
		m.schedulerRuns,
		m.schedulerErrors,
		m.schedulerAttempts,
		m.schedulerFailures,
		m.tasksClaimed,
		m.assignmentConflicts,
		m.schedulerDuration,
		m.reconcilerRuns,
		m.reconcilerErrors,
		m.reconcilerDuration,
		m.tasksCreated,
		m.tasksFailed,
		m.rollouts,
		m.rolloutFailures,
		m.autoscalerDecisions,
		m.autoscalerErrors,
		m.autoscalerRecommend,
		m.leaderStatus,
		m.leaderFailures,
	)
	return m
}

func (m *Server) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Server) ObserveAPIRequest(method string, route string, status int, duration time.Duration) {
	statusCode := strconv.Itoa(status)
	m.apiRequests.WithLabelValues(method, route, statusCode).Inc()
	m.apiRequestDuration.WithLabelValues(method, route, statusCode).Observe(duration.Seconds())
}

func (m *Server) IncSchedulerRuns() {
	m.schedulerRuns.Inc()
}

func (m *Server) IncSchedulerErrors() {
	m.schedulerErrors.Inc()
}

func (m *Server) ObserveSchedulerDuration(duration time.Duration) {
	m.schedulerDuration.Observe(duration.Seconds())
}

func (m *Server) IncSchedulingAttempts() {
	m.schedulerAttempts.Inc()
}

func (m *Server) IncSchedulingFailures() {
	m.schedulerFailures.Inc()
}

func (m *Server) IncTasksClaimed() {
	m.tasksClaimed.Inc()
}

func (m *Server) IncAssignmentConflicts() {
	m.assignmentConflicts.Inc()
}

func (m *Server) IncReconciliationRuns() {
	m.reconcilerRuns.Inc()
}

func (m *Server) ObserveReconciliationDuration(duration time.Duration) {
	m.reconcilerDuration.Observe(duration.Seconds())
}

func (m *Server) IncReconciliationErrors() {
	m.reconcilerErrors.Inc()
}

func (m *Server) AddCreatedTasks(count int) {
	m.add(m.tasksCreated, count)
}

func (m *Server) AddStoppedTasks(int) {}

func (m *Server) IncTasksFailed() {
	m.tasksFailed.Inc()
}

func (m *Server) IncRollouts() {
	m.rollouts.Inc()
}

func (m *Server) IncRolloutFailures() {
	m.rolloutFailures.Inc()
}

func (m *Server) IncAutoscalerDecision(decision string) {
	m.autoscalerDecisions.WithLabelValues(decision).Inc()
}

func (m *Server) IncAutoscalerErrors() {
	m.autoscalerErrors.Inc()
}

func (m *Server) SetAutoscalerRecommendation(serviceID string, replicas int) {
	m.autoscalerRecommend.WithLabelValues(serviceID).Set(float64(replicas))
}

func (m *Server) SetLeaderStatus(controller string, leader bool) {
	value := 0.0
	if leader {
		value = 1
	}
	m.leaderStatus.WithLabelValues(controller).Set(value)
}

func (m *Server) IncLeaderAcquisitionFailure(controller string) {
	m.leaderFailures.WithLabelValues(controller).Inc()
}

func (m *Server) add(counter prometheus.Counter, count int) {
	if count <= 0 {
		return
	}
	counter.Add(float64(count))
}

type Agent struct {
	registry *prometheus.Registry

	heartbeatSuccess      prometheus.Counter
	heartbeatFailure      prometheus.Counter
	dockerOperations      *prometheus.CounterVec
	dockerOperationErrors *prometheus.CounterVec
	taskStateChanges      *prometheus.CounterVec
	healthcheckSuccess    prometheus.Counter
	healthcheckFailure    prometheus.Counter
}

func NewAgent() *Agent {
	m := &Agent{
		registry: prometheus.NewRegistry(),
		heartbeatSuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "heartbeat_success_total",
			Help: "Total successful heartbeat attempts.",
		}),
		heartbeatFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "heartbeat_failure_total",
			Help: "Total failed heartbeat attempts.",
		}),
		dockerOperations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "docker_operations_total",
			Help: "Total Docker runtime operations by operation type.",
		}, []string{"operation"}),
		dockerOperationErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "docker_operation_errors_total",
			Help: "Total Docker runtime operation errors by operation type.",
		}, []string{"operation"}),
		taskStateChanges: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "task_state_changes_total",
			Help: "Total task state changes reported by the agent.",
		}, []string{"status"}),
		healthcheckSuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "healthcheck_success_total",
			Help: "Total successful health checks.",
		}),
		healthcheckFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "healthcheck_failure_total",
			Help: "Total failed health checks.",
		}),
	}
	m.registry.MustRegister(
		m.heartbeatSuccess,
		m.heartbeatFailure,
		m.dockerOperations,
		m.dockerOperationErrors,
		m.taskStateChanges,
		m.healthcheckSuccess,
		m.healthcheckFailure,
	)
	return m
}

func (m *Agent) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Agent) IncHeartbeatSuccess() {
	m.heartbeatSuccess.Inc()
}

func (m *Agent) IncHeartbeatFailure() {
	m.heartbeatFailure.Inc()
}

func (m *Agent) IncDockerOperation(operation string) {
	m.dockerOperations.WithLabelValues(operation).Inc()
}

func (m *Agent) IncDockerOperationError(operation string) {
	m.dockerOperationErrors.WithLabelValues(operation).Inc()
}

func (m *Agent) IncTaskStateChange(status types.TaskStatus) {
	m.taskStateChanges.WithLabelValues(string(status)).Inc()
}

func (m *Agent) IncHealthcheckSuccess() {
	m.healthcheckSuccess.Inc()
}

func (m *Agent) IncHealthcheckFailure() {
	m.healthcheckFailure.Inc()
}
