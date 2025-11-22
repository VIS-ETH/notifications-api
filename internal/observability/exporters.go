package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

func SetupTracer(ctx context.Context, res *resource.Resource) (*trace.TracerProvider, error) {
	tracesExp, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to startup otlp traces grpc exporter. Error: %v", err)
	}
	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(tracesExp),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	return tracerProvider, nil
}

func SetupMetrics(ctx context.Context, exportOtelMetrics bool, res *resource.Resource) (*metric.MeterProvider, error) {
	var options []metric.Option
	if exportOtelMetrics {
		metricsExp, err := otlpmetricgrpc.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to startup otlp metrics grpc exporter. Error: %v", err)
		}
		options = append(options, metric.WithReader(metric.NewPeriodicReader(metricsExp)))
	}

	promExporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus exporter: %v", err)
	}

	options = append(options,
		metric.WithResource(res),
		metric.WithReader(promExporter),
	)

	meterProvider := metric.NewMeterProvider(
		options...,
	)

	otel.SetMeterProvider(meterProvider)
	return meterProvider, nil
}
