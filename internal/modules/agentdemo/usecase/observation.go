package usecase

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const agentTracer = "personaos/agentdemo"

func startAgentSpan(ctx context.Context, name string, deviceTypeID int64) (context.Context, trace.Span) {
	return otel.Tracer(agentTracer).Start(ctx, name,
		trace.WithAttributes(attribute.Int64("device_type.id", deviceTypeID)))
}

// Do not export provider error strings: they can contain conversation data or credentials.
func finishAgentSpan(span trace.Span, err error) {
	if err != nil {
		status := "failed"
		switch {
		case errors.Is(err, context.Canceled):
			status = "canceled"
		case errors.Is(err, context.DeadlineExceeded):
			status = "timeout"
		}
		span.SetStatus(codes.Error, status)
		span.SetAttributes(attribute.String("error.type", status))
	}
	span.End()
}
