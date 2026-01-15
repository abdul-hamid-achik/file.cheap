package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type TraceCarrier struct {
	TraceParent string `json:"trace_parent,omitempty"`
	TraceState  string `json:"trace_state,omitempty"`
}

func InjectTraceContext(ctx context.Context) TraceCarrier {
	carrier := TraceCarrier{}
	propagator := propagation.TraceContext{}

	mapCarrier := propagation.MapCarrier{}
	propagator.Inject(ctx, mapCarrier)

	carrier.TraceParent = mapCarrier.Get("traceparent")
	carrier.TraceState = mapCarrier.Get("tracestate")

	return carrier
}

func ExtractTraceContext(ctx context.Context, carrier TraceCarrier) context.Context {
	if carrier.TraceParent == "" {
		return ctx
	}

	propagator := propagation.TraceContext{}
	mapCarrier := propagation.MapCarrier{
		"traceparent": carrier.TraceParent,
		"tracestate":  carrier.TraceState,
	}

	return propagator.Extract(ctx, mapCarrier)
}

func StartJobSpan(ctx context.Context, jobType, jobID string) (context.Context, trace.Span) {
	ctx, span := Tracer().Start(ctx, "job.process."+jobType,
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	span.SetAttributes(
		attribute.String("job.type", jobType),
		attribute.String("job.id", jobID),
	)
	return ctx, span
}

func StartJobEnqueueSpan(ctx context.Context, jobType string) (context.Context, trace.Span) {
	ctx, span := Tracer().Start(ctx, "job.enqueue."+jobType,
		trace.WithSpanKind(trace.SpanKindProducer),
	)
	span.SetAttributes(
		attribute.String("job.type", jobType),
	)
	return ctx, span
}
