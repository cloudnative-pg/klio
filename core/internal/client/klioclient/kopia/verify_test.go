package kopia

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kopiaClient "github.com/cloudnative-pg/klio/core/internal/kopia"
)

func TestBackupVerificationError(t *testing.T) {
	t.Run("Error includes error count", func(t *testing.T) {
		underlyingErr := errors.New("verification failed")
		err := &BackupVerificationError{
			Result: kopiaClient.VerifyResult{
				ErrorCount:   3,
				ErrorStrings: []string{"error1", "error2", "error3"},
			},
			Err: underlyingErr,
		}

		errMsg := err.Error()
		assert.Contains(t, errMsg, "3")
		assert.Contains(t, errMsg, "verification failed")
	})

	t.Run("Error with single error count", func(t *testing.T) {
		err := &BackupVerificationError{
			Result: kopiaClient.VerifyResult{
				ErrorCount:   1,
				ErrorStrings: []string{"corrupt object"},
			},
			Err: errors.New("kopia error"),
		}

		errMsg := err.Error()
		assert.Contains(t, errMsg, "1")
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		underlyingErr := errors.New("original error")
		err := &BackupVerificationError{
			Result: kopiaClient.VerifyResult{ErrorCount: 1},
			Err:    underlyingErr,
		}

		assert.Equal(t, underlyingErr, err.Unwrap())
		assert.ErrorIs(t, err, underlyingErr)
	})
}

func TestClassifyVerifyError(t *testing.T) {
	ctx := context.Background()

	t.Run("corruption detected", func(t *testing.T) {
		verifyErr := errors.New("corrupt object detected")
		result := kopiaClient.VerifyResult{
			ErrorCount:   2,
			ErrorStrings: []string{"bad object 1", "bad object 2"},
		}

		err := classifyVerifyError(ctx, result, verifyErr)

		var backupErr *BackupVerificationError
		require.ErrorAs(t, err, &backupErr)
		require.Equal(t, result, backupErr.Result)
		require.ErrorIs(t, err, verifyErr)
	})

	t.Run("infrastructure error does not return BackupVerificationError", func(t *testing.T) {
		infraErr := errors.New("connection refused")
		result := kopiaClient.VerifyResult{ErrorCount: 0}

		err := classifyVerifyError(ctx, result, infraErr)

		var backupErr *BackupVerificationError
		require.ErrorIs(t, err, infraErr)
		require.NotErrorAs(t, err, &backupErr)
	})
}
