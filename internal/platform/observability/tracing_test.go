package observability

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestTraceParentRoundTrip 覆盖 W3C traceparent 的注入与提取
func TestTraceParentRoundTrip(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	ctx, span := Tracer().Start(context.Background(), "test.parent")
	defer span.End()
	traceParent := TraceParent(ctx)
	if !strings.HasPrefix(traceParent, "00-") {
		t.Fatalf("traceparent = %q", traceParent)
	}
	extracted := ContextWithTraceParent(context.Background(), traceParent)
	if TraceID(extracted) != TraceID(ctx) {
		t.Fatalf(
			"传播后 Trace ID = %q，期望 %q",
			TraceID(extracted),
			TraceID(ctx),
		)
	}
}

// TestTraceExportEndpoint 覆盖 OTLP HTTP 基地址和完整导出路径的规范化
func TestTraceExportEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{
			name:     "基地址",
			endpoint: "http://127.0.0.1:4318",
			expected: "http://127.0.0.1:4318/v1/traces",
		},
		{
			name:     "尾斜杠",
			endpoint: "http://127.0.0.1:4318/",
			expected: "http://127.0.0.1:4318/v1/traces",
		},
		{
			name:     "完整路径",
			endpoint: "http://127.0.0.1:4318/v1/traces",
			expected: "http://127.0.0.1:4318/v1/traces",
		},
		{
			name:     "空配置",
			endpoint: " ",
			expected: "http://127.0.0.1:4318/v1/traces",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := traceExportEndpoint(test.endpoint); actual != test.expected {
				t.Fatalf(
					"traceExportEndpoint(%q) = %q，期望 %q",
					test.endpoint,
					actual,
					test.expected,
				)
			}
		})
	}
}
