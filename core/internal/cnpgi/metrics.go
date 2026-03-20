package cnpgi

import (
	"context"
	"sync/atomic"

	"github.com/cloudnative-pg/cnpg-i/pkg/metrics"
	"github.com/cloudnative-pg/machinery/pkg/log"
)

// verificationMetrics tracks backup verification outcomes.
//
//nolint:gochecknoglobals
var verificationMetrics struct {
	total    atomic.Int64
	failures atomic.Int64
}

// recordVerificationSuccess records a successful verification.
func recordVerificationSuccess() {
	verificationMetrics.total.Add(1)
}

// recordVerificationFailure records a failed verification.
func recordVerificationFailure() {
	verificationMetrics.total.Add(1)
	verificationMetrics.failures.Add(1)
}

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
				FqName:    "klio_backup_verifications_total",
				Help:      "Total number of backup verification attempts.",
				ValueType: &metrics.MetricType{Type: metrics.MetricType_TYPE_COUNTER},
			},
			{
				FqName:    "klio_backup_verification_failures_total",
				Help:      "Total number of backup verification failures.",
				ValueType: &metrics.MetricType{Type: metrics.MetricType_TYPE_COUNTER},
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
				FqName: "klio_backup_verifications_total",
				Value:  float64(verificationMetrics.total.Load()),
			},
			{
				FqName: "klio_backup_verification_failures_total",
				Value:  float64(verificationMetrics.failures.Load()),
			},
		},
	}, nil
}
