package cnpgi

import (
	"context"

	"github.com/cloudnative-pg/cnpg-i/pkg/metrics"
	"github.com/cloudnative-pg/machinery/pkg/log"
)

type metricsImpl struct {
	metrics.UnimplementedMetricsServer
}

func (m metricsImpl) GetCapabilities(
	ctx context.Context,
	_ *metrics.MetricsCapabilitiesRequest,
) (*metrics.MetricsCapabilitiesResult, error) {
	contextLogger := log.FromContext(ctx)
	contextLogger.Trace("metrics capabilities call received")

	return &metrics.MetricsCapabilitiesResult{
		Capabilities: []*metrics.MetricsCapability{
			{
				Type: &metrics.MetricsCapability_Rpc{
					Rpc: &metrics.MetricsCapability_RPC{
						Type: metrics.MetricsCapability_RPC_TYPE_METRICS,
					},
				},
			},
		},
	}, nil
}

func (m metricsImpl) Define(
	ctx context.Context,
	_ *metrics.DefineMetricsRequest,
) (*metrics.DefineMetricsResult, error) {
	contextLogger := log.FromContext(ctx)
	contextLogger.Trace("metrics define call received")

	return &metrics.DefineMetricsResult{
		Metrics: []*metrics.Metric{
			{
				FqName:         "klio_test_metric",
				Help:           "this is a test metric",
				VariableLabels: nil,
				ConstLabels:    map[string]string{"test": "value"},
				ValueType:      &metrics.MetricType{Type: metrics.MetricType_TYPE_GAUGE},
			},
		},
	}, nil
}

func (m metricsImpl) Collect(
	ctx context.Context,
	_ *metrics.CollectMetricsRequest,
) (*metrics.CollectMetricsResult, error) {
	contextLogger := log.FromContext(ctx)
	contextLogger.Trace("metrics collect call received")

	return &metrics.CollectMetricsResult{
		Metrics: []*metrics.CollectMetric{
			{
				FqName:         "klio_test_metric",
				VariableLabels: nil,
				Value:          42.0,
			},
		},
	}, nil
}
