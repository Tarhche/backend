package providers

import (
	"context"
	fallbackLog "log"
	"log/slog"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"
)

type openTelemetryProvider struct {
	serviceName string
	namespace   string

	terminate func()
}

var _ provider.Provider = &openTelemetryProvider{}

func NewOpenTelemetryProvider(serviceName string, namespace string) *openTelemetryProvider {
	return &openTelemetryProvider{
		serviceName: serviceName,
		namespace:   namespace,
	}
}

func (p *openTelemetryProvider) Register(ctx context.Context, c provider.Container) error {
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithAttributes(
			semconv.ServiceName(p.serviceName),
			semconv.ServiceNamespace(p.namespace),
		),
	)
	if err != nil {
		return err
	}

	logProvider, err := openTelemetryLogProvider(ctx, res)
	if err != nil {
		return err
	}

	traceProvider, err := openTelementryTraceProvider(ctx, res)
	if err != nil {
		return err
	}

	meterProvider, err := openTelemetryMeterProvider(ctx, res)
	if err != nil {
		return err
	}

	p.terminate = func() {
		if err := logProvider.Shutdown(ctx); err != nil {
			fallbackLog.Println(err)
		}

		if err := traceProvider.Shutdown(ctx); err != nil {
			fallbackLog.Println(err)
		}

		if err := meterProvider.Shutdown(ctx); err != nil {
			fallbackLog.Println(err)
		}
	}

	loggerResolver := func(name string) *slog.Logger {
		return otelslog.NewLogger(name, otelslog.WithLoggerProvider(logProvider))
	}

	// name should be provided on resolve step, so we bind a factory function that takes the name as an argument
	if err := c.Bind(loggerResolver, bind.Lazy()); err != nil {
		return err
	}

	// the resource identifies this service instance on every signal; it is
	// shared with providers that emit telemetry themselves (e.g. profiling)
	return c.Bind(func() *resource.Resource { return res }, bind.Singleton())
}

func (p *openTelemetryProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *openTelemetryProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func openTelemetryLogProvider(ctx context.Context, res *resource.Resource) (*log.LoggerProvider, error) {
	httpLogExporter, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, err
	}

	logProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(httpLogExporter)),
		log.WithResource(res),
	)

	return logProvider, nil
}

func openTelementryTraceProvider(ctx context.Context, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	httpTraceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(httpTraceExporter),
		sdktrace.WithResource(res),
	)

	// Makes the tracer available to instrumentation libraries
	otel.SetTracerProvider(traceProvider)

	// Propagates trace context across service boundaries using W3C standards
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return traceProvider, nil
}

func openTelemetryMeterProvider(ctx context.Context, res *resource.Resource) (*metric.MeterProvider, error) {
	httpMetricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, err
	}

	// Create the MeterProvider with the exporter and resource
	// Set a periodic reader to export metrics every 10 seconds
	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(httpMetricExporter, metric.WithInterval(10*time.Second))),
	)

	// Set the global MeterProvider
	otel.SetMeterProvider(meterProvider)

	return meterProvider, nil
}
