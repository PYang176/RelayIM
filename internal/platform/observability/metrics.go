// Package observability 提供健康检查、请求追踪和 Prometheus 指标。
package observability

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics 持有当前进程独立的 Prometheus 注册表和业务指标。
type Metrics struct {
	registry       *prometheus.Registry
	requests       *prometheus.CounterVec
	requestLatency *prometheus.HistogramVec
	inFlight       prometheus.Gauge
	messages       *prometheus.CounterVec
	attachments    *prometheus.CounterVec
}

// NewMetrics 创建指标注册表和 HTTP、运行时基础指标。
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{
			Namespace: "ginchat",
		}),
	)

	metrics := &Metrics{
		registry: registry,
		requests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "ginchat",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "HTTP 请求总数",
			},
			[]string{"method", "route", "status"},
		),
		requestLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "ginchat",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP 请求耗时",
				Buckets: []float64{
					0.005,
					0.01,
					0.025,
					0.05,
					0.1,
					0.25,
					0.5,
					1,
					2.5,
					5,
				},
			},
			[]string{"method", "route", "status"},
		),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "ginchat",
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "当前正在处理的 HTTP 请求数",
		}),
		messages: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "ginchat",
				Subsystem: "message",
				Name:      "accepted_total",
				Help:      "服务端成功接受并持久化的消息数",
			},
			[]string{"type", "duplicate"},
		),
		attachments: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "ginchat",
				Subsystem: "attachment",
				Name:      "uploads_total",
				Help:      "附件上传统计",
			},
			[]string{"result"},
		),
	}
	registry.MustRegister(
		metrics.requests,
		metrics.requestLatency,
		metrics.inFlight,
		metrics.messages,
		metrics.attachments,
	)
	return metrics
}

// RecordMessageAccepted 记录服务端成功接受的消息。
func (m *Metrics) RecordMessageAccepted(
	messageType string,
	duplicate bool,
) {
	duplicateLabel := "false"
	if duplicate {
		duplicateLabel = "true"
	}
	m.messages.With(prometheus.Labels{
		"type":      messageType,
		"duplicate": duplicateLabel,
	}).Inc()
}

// RecordAttachmentUpload 记录附件上传成功或失败。
func (m *Metrics) RecordAttachmentUpload(success bool) {
	result := "failure"
	if success {
		result = "success"
	}
	m.attachments.WithLabelValues(result).Inc()
}

// Handler 返回 Prometheus 标准抓取端点。
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// BeginRequest 记录当前处理中的请求数。
func (m *Metrics) BeginRequest() {
	m.inFlight.Inc()
}

// ObserveHTTP 记录一次 HTTP 请求的状态和耗时。
func (m *Metrics) ObserveHTTP(
	method string,
	route string,
	status int,
	duration time.Duration,
) {
	if route == "" {
		route = "unmatched"
	}
	labels := prometheus.Labels{
		"method": method,
		"route":  route,
		"status": httpStatusLabel(status),
	}
	m.requests.With(labels).Inc()
	m.requestLatency.With(labels).Observe(duration.Seconds())
	m.inFlight.Dec()
}

// RegisterWebSocketConnections 注册进程内 WebSocket 连接数采集函数。
func (m *Metrics) RegisterWebSocketConnections(count func() int) {
	m.registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "ginchat",
			Name:      "websocket_connections",
			Help:      "当前进程内有效 WebSocket 连接数",
		},
		func() float64 {
			return float64(count())
		},
	))
}

// RegisterDatabaseStats 注册 MySQL 连接池状态采集函数。
func (m *Metrics) RegisterDatabaseStats(stats func() sql.DBStats) {
	states := []string{"open", "in_use", "idle", "max"}
	for _, state := range states {
		state := state
		m.registry.MustRegister(prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "ginchat",
				Subsystem: "db",
				Name:      "connections",
				Help:      "MySQL 连接池连接数",
				ConstLabels: prometheus.Labels{
					"state": state,
				},
			},
			func() float64 {
				value := stats()
				switch state {
				case "open":
					return float64(value.OpenConnections)
				case "in_use":
					return float64(value.InUse)
				case "idle":
					return float64(value.Idle)
				case "max":
					return float64(value.MaxOpenConnections)
				default:
					return 0
				}
			},
		))
	}
}

// OutboxStats 是 Outbox 各状态的当前数量。
type OutboxStats struct {
	Pending    int64
	Processing int64
	Published  int64
	Dead       int64
}

// RegisterOutboxStats 注册 Outbox 状态采集器。
func (m *Metrics) RegisterOutboxStats(
	load func(ctx context.Context) (OutboxStats, error),
) {
	m.registry.MustRegister(&outboxCollector{load: load})
}

// Registry 返回只读注册表，供测试或自定义 Exporter 使用。
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

func httpStatusLabel(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

type outboxCollector struct {
	load func(ctx context.Context) (OutboxStats, error)
}

var (
	outboxEventsDesc = prometheus.NewDesc(
		"ginchat_outbox_events",
		"Outbox 各状态事件数量",
		[]string{"status"},
		nil,
	)
	outboxQuerySuccessDesc = prometheus.NewDesc(
		"ginchat_outbox_query_success",
		"最近一次 Outbox 状态查询是否成功",
		nil,
		nil,
	)
)

// Describe 描述 Outbox 采集器产生的指标。
func (c *outboxCollector) Describe(channel chan<- *prometheus.Desc) {
	channel <- outboxEventsDesc
	channel <- outboxQuerySuccessDesc
}

// Collect 每次抓取只执行一次数据库聚合查询。
func (c *outboxCollector) Collect(channel chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stats, err := c.load(ctx)
	if err != nil {
		channel <- prometheus.MustNewConstMetric(
			outboxQuerySuccessDesc,
			prometheus.GaugeValue,
			0,
		)
		return
	}
	channel <- prometheus.MustNewConstMetric(
		outboxQuerySuccessDesc,
		prometheus.GaugeValue,
		1,
	)
	values := []struct {
		status string
		value  int64
	}{
		{status: "pending", value: stats.Pending},
		{status: "processing", value: stats.Processing},
		{status: "published", value: stats.Published},
		{status: "dead", value: stats.Dead},
	}
	for _, value := range values {
		channel <- prometheus.MustNewConstMetric(
			outboxEventsDesc,
			prometheus.GaugeValue,
			float64(value.value),
			value.status,
		)
	}
}
