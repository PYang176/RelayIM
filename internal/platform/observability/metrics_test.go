package observability

import (
	"database/sql"
	"testing"
	"time"
)

func TestMetricsExposeHTTPAndWebSocketValues(t *testing.T) {
	metrics := NewMetrics()
	metrics.RegisterWebSocketConnections(func() int {
		return 3
	})
	metrics.RegisterDatabaseStats(func() sql.DBStats {
		return sql.DBStats{OpenConnections: 2}
	})
	metrics.BeginRequest()
	metrics.ObserveHTTP(
		"GET",
		"/api/v1/conversations",
		200,
		25*time.Millisecond,
	)

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("采集指标失败: %v", err)
	}
	names := make(map[string]bool, len(families))
	for _, family := range families {
		names[family.GetName()] = true
	}
	for _, name := range []string{
		"ginchat_http_requests_total",
		"ginchat_http_request_duration_seconds",
		"ginchat_http_requests_in_flight",
		"ginchat_websocket_connections",
		"go_goroutines",
	} {
		if !names[name] {
			t.Fatalf("缺少指标 %s", name)
		}
	}
}
