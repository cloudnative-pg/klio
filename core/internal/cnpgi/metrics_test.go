package cnpgi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetMetrics(t *testing.T) {
	t.Helper()
	verificationMetrics.total.Store(0)
	verificationMetrics.failures.Store(0)
}

func TestRecordVerificationSuccess(t *testing.T) {
	resetMetrics(t)

	recordVerificationSuccess()
	recordVerificationSuccess()

	assert.Equal(t, int64(2), verificationMetrics.total.Load())
	assert.Equal(t, int64(0), verificationMetrics.failures.Load())
}

func TestRecordVerificationFailure(t *testing.T) {
	resetMetrics(t)

	recordVerificationFailure()

	assert.Equal(t, int64(1), verificationMetrics.total.Load())
	assert.Equal(t, int64(1), verificationMetrics.failures.Load())
}

func TestRecordMixed(t *testing.T) {
	resetMetrics(t)

	recordVerificationSuccess()
	recordVerificationFailure()
	recordVerificationSuccess()

	assert.Equal(t, int64(3), verificationMetrics.total.Load())
	assert.Equal(t, int64(1), verificationMetrics.failures.Load())
}

func TestMetricsDefine(t *testing.T) {
	m := metricsImpl{}
	result, err := m.Define(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.GetMetrics(), 2)

	assert.Equal(t, "klio_backup_verifications_total", result.GetMetrics()[0].GetFqName())
	assert.Equal(t, "klio_backup_verification_failures_total", result.GetMetrics()[1].GetFqName())
}

func TestMetricsCollect(t *testing.T) {
	resetMetrics(t)

	recordVerificationSuccess()
	recordVerificationFailure()

	m := metricsImpl{}
	result, err := m.Collect(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.GetMetrics(), 2)

	assert.Equal(t, "klio_backup_verifications_total", result.GetMetrics()[0].GetFqName())
	assert.InDelta(t, float64(2), result.GetMetrics()[0].GetValue(), 0)

	assert.Equal(t, "klio_backup_verification_failures_total", result.GetMetrics()[1].GetFqName())
	assert.InDelta(t, float64(1), result.GetMetrics()[1].GetValue(), 0)
}
