package utils

import (
	gcpExporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/api/global"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ConfigureTracer Configures an implementation for tracing purposes
func ConfigureTracer(serviceName string, log *logrus.Entry) error {

	projectID := GetEnv("GOOGLE_CLOUD_PROJECT", "")

	if projectID != "" {

		exporter, err := gcpExporter.NewExporter(gcpExporter.WithProjectID(projectID))
		if err != nil {
			return err
		}

		tp, err := sdktrace.NewProvider(sdktrace.WithSyncer(exporter))
		if err != nil {
			return err
		}
		global.SetTraceProvider(tp)

	}else{
		tp, err := sdktrace.NewProvider()
		if err != nil {
			return err
		}
		global.SetTraceProvider(tp)

	}

	return nil
}
