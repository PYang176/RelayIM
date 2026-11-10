package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"relayim/internal/platform/observability"
)

// TraceRequest 创建 HTTP Server Span，并提取上游 W3C traceparent。
func TraceRequest() gin.HandlerFunc {
	tracer := observability.Tracer()
	return func(c *gin.Context) {
		parentCtx := otel.GetTextMapPropagator().Extract(
			c.Request.Context(),
			propagation.HeaderCarrier(c.Request.Header),
		)
		ctx, span := tracer.Start(
			parentCtx,
			c.Request.Method+" "+c.Request.URL.Path,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", c.Request.Method),
				attribute.String("url.path", c.Request.URL.Path),
			),
		)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Trace-ID", observability.TraceID(ctx))
		span.SetAttributes(attribute.String(
			"relayim.request_id",
			c.GetString("request_id"),
		))

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := c.Writer.Status()
		span.SetName(c.Request.Method + " " + route)
		span.SetAttributes(
			attribute.String("http.route", route),
			attribute.Int("http.response.status_code", status),
		)
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
		for _, ginErr := range c.Errors {
			span.RecordError(ginErr.Err)
		}
		span.End()
	}
}
