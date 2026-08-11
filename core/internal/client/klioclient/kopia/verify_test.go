/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

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

	// An unresolved snapshot set means Kopia never walked any object, so the
	// result carries no integrity evidence: reporting corruption there fails an
	// intact backup.
	t.Run("unresolved snapshot set is retryable, not corruption", func(t *testing.T) {
		verifyErr := errors.New("while verifying Kopia snapshots: command failed: exit status 1")
		result := kopiaClient.VerifyResult{
			ErrorCount:   1,
			ErrorStrings: []string{"found 0 of the 3 requested snapshot IDs to verify"},
		}

		err := classifyVerifyError(ctx, result, verifyErr)

		var backupErr *BackupVerificationError
		require.NotErrorAs(t, err, &backupErr)
		require.ErrorIs(t, err, ErrSnapshotsUnresolved)
		require.ErrorIs(t, err, verifyErr)
	})

	// A missing object or blob is real data loss and must stay fatal, even
	// though Kopia reports it with wording that also contains "not found".
	t.Run("missing blob is still corruption", func(t *testing.T) {
		verifyErr := errors.New("while verifying Kopia snapshots: command failed: exit status 1")
		result := kopiaClient.VerifyResult{
			ErrorCount: 1,
			ErrorStrings: []string{
				"object 8f848427a18ebe0fbc2f063b4616e362 is backed by missing blob " +
					"p79f56bafb33cd7823558352ea947c830-s59fd9cbc252f04f7143",
			},
		}

		err := classifyVerifyError(ctx, result, verifyErr)

		var backupErr *BackupVerificationError
		require.ErrorAs(t, err, &backupErr)
		require.NotErrorIs(t, err, ErrSnapshotsUnresolved)
	})

	t.Run("missing object referenced by a directory id is still corruption", func(t *testing.T) {
		verifyErr := errors.New("while verifying Kopia snapshots: command failed: exit status 1")
		result := kopiaClient.VerifyResult{
			ErrorCount: 1,
			ErrorStrings: []string{
				"error reading directory: unable to open object: kdeadbeef: " +
					"content kdeadbeef not found: object not found",
			},
		}

		err := classifyVerifyError(ctx, result, verifyErr)

		var backupErr *BackupVerificationError
		require.ErrorAs(t, err, &backupErr)
	})

	// A mixed result must not be downgraded: one unresolved snapshot alongside
	// genuine damage is still corruption.
	t.Run("corruption mixed with an unresolved snapshot stays corruption", func(t *testing.T) {
		verifyErr := errors.New("while verifying Kopia snapshots: command failed: exit status 1")
		result := kopiaClient.VerifyResult{
			ErrorCount: 2,
			ErrorStrings: []string{
				"found 0 of the 3 requested snapshot IDs to verify",
				"object abc is backed by missing blob p123",
			},
		}

		err := classifyVerifyError(ctx, result, verifyErr)

		var backupErr *BackupVerificationError
		require.ErrorAs(t, err, &backupErr)
	})
}
