package observability

import (
	"context"
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
	metrics.RegisterOutboxStats(func(context.Context) (OutboxStats, error) {
		return OutboxStats{Pending: 4, Dead: 1}, nil
	})
	metrics.RecordMessageAccepted("text", false)
	metrics.RecordMessageAccepted("text", true)
	metrics.RecordAttachmentUpload(true)
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
		"ginchat_message_accepted_total",
		"ginchat_attachment_uploads_total",
		"ginchat_outbox_events",
		"ginchat_outbox_query_success",
		"go_goroutines",
	} {
		if !names[name] {
			t.Fatalf("缺少指标 %s", name)
		}
	}
}
