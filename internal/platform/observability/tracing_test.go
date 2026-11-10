package observability

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

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
