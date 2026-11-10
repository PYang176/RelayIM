package observability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "relayim"

// InitTracing 初始化 OTLP Trace Provider 并返回关闭函数。
func InitTracing(
	ctx context.Context,
	cfg config.TracingConfig,
	instanceID string,
) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	if !cfg.Enabled {
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpointURL(traceExportEndpoint(cfg.OTLPEndpoint)),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 OTLP Trace Exporter: %w", err)
	}
	res := resource.NewSchemaless(
		attribute.String("service.name", cfg.ServiceName),
		attribute.String("service.version", "1.0.0"),
		attribute.String("service.instance.id", instanceID),
	)
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			exporter,
			sdktrace.WithBatchTimeout(time.Second),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(cfg.SampleRatio),
		)),
	)
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

// traceExportEndpoint 把 OTLP HTTP 基地址规范化为 Trace 导出路径。
func traceExportEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return "http://127.0.0.1:4318/v1/traces"
	}
	if strings.HasSuffix(endpoint, "/v1/traces") {
		return endpoint
	}
	return endpoint + "/v1/traces"
}

// TraceParent 把当前 Span 上下文编码为 W3C traceparent。
func TraceParent(ctx context.Context) string {
	return TraceHeaders(ctx)["traceparent"]
}

// TraceHeaders 返回当前 Span 的 W3C 传播头。
func TraceHeaders(ctx context.Context) map[string]string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier
}

// ContextWithTraceParent 从持久化的 W3C traceparent 恢复父 Span。
func ContextWithTraceParent(ctx context.Context, traceParent string) context.Context {
	if traceParent == "" {
		return ctx
	}
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// Tracer 返回项目统一的 Trace Tracer。
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// TraceID 返回当前 Span 的 Trace ID；无有效 Span 时返回空字符串。
func TraceID(ctx context.Context) string {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return ""
	}
	return spanContext.TraceID().String()
}
