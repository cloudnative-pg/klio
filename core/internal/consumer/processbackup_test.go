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

package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/queue"
)

// stubBackupSteps implements backupSteps for unit tests. Each field controls
// what the corresponding step returns; the bool fields record whether the step
// was called.
type stubBackupSteps struct {
	manifests    []kopia.Manifest
	manifestsErr error
	verifyErr    error
	relayErr     error
	maintain2Err error
	maintainErr  error

	verifyCalled    bool
	relayCalled     bool
	maintain2Called bool
	maintainCalled  bool
}

func (s *stubBackupSteps) listManifests(_ context.Context, _ string) ([]kopia.Manifest, error) {
	return s.manifests, s.manifestsErr
}

func (s *stubBackupSteps) verifyTier1(_ context.Context, _ string) error {
	s.verifyCalled = true
	return s.verifyErr
}

func (s *stubBackupSteps) relayTier2(_ context.Context, _ *queue.BackupTask, _ []kopia.Manifest) error {
	s.relayCalled = true
	return s.relayErr
}

func (s *stubBackupSteps) maintainTier2(_ context.Context, _ *queue.BackupTask, _ []kopia.Manifest) error {
	s.maintain2Called = true
	return s.maintain2Err
}

func (s *stubBackupSteps) maintainTier1(_ context.Context, _ string, _ []kopia.Manifest) error {
	s.maintainCalled = true
	return s.maintainErr
}

func TestProcessBackup(t *testing.T) {
	// processBackup records the relay counter inline, so the instruments must be
	// initialized (to no-op instruments here) to avoid a nil dereference.
	opentelemetry.InitServerBackupMetrics()

	errBoom := errors.New("boom")
	// A single zero-value manifest is enough: the step stubs ignore the
	// contents and only the slice length matters to processBackup.
	someEntries := []kopia.Manifest{{}}

	tests := []struct {
		name         string
		sendToTier2  bool
		tier2Enabled bool
		manifests    []kopia.Manifest
		manifestsErr error
		verifyErr    error
		relayErr     error
		maintain2Err error
		maintainErr  error

		wantErr       bool
		wantVerify    bool
		wantRelay     bool
		wantMaintain2 bool
		wantMaintain  bool
	}{
		{
			name:         "manifest listing fails is retried",
			manifestsErr: errBoom,
			wantErr:      true,
		},
		{
			name:      "no manifests is a no-op",
			manifests: nil,
		},
		{
			name:         "tier1-only backup verifies and maintains tier1",
			manifests:    someEntries,
			wantVerify:   true,
			wantMaintain: true,
		},
		{
			name:       "tier1 verification failure is retried",
			manifests:  someEntries,
			verifyErr:  errBoom,
			wantErr:    true,
			wantVerify: true,
		},
		{
			name:          "tier2 relay success runs both maintenances",
			sendToTier2:   true,
			tier2Enabled:  true,
			manifests:     someEntries,
			wantVerify:    true,
			wantRelay:     true,
			wantMaintain2: true,
			wantMaintain:  true,
		},
		{
			name:         "tier2 relay failure is retried before any maintenance",
			sendToTier2:  true,
			tier2Enabled: true,
			manifests:    someEntries,
			relayErr:     errBoom,
			wantErr:      true,
			wantVerify:   true,
			wantRelay:    true,
		},
		{
			name:          "tier2 maintenance failure is retried before tier1 maintenance",
			sendToTier2:   true,
			tier2Enabled:  true,
			manifests:     someEntries,
			maintain2Err:  errBoom,
			wantErr:       true,
			wantVerify:    true,
			wantRelay:     true,
			wantMaintain2: true,
		},
		{
			name:         "tier2 requested but not configured fails after tier1 maintenance",
			sendToTier2:  true,
			tier2Enabled: false,
			manifests:    someEntries,
			wantErr:      true,
			wantVerify:   true,
			wantMaintain: true,
		},
		{
			name:         "tier1 maintenance error is swallowed",
			manifests:    someEntries,
			maintainErr:  errBoom,
			wantVerify:   true,
			wantMaintain: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubBackupSteps{
				manifests:    tt.manifests,
				manifestsErr: tt.manifestsErr,
				verifyErr:    tt.verifyErr,
				relayErr:     tt.relayErr,
				maintain2Err: tt.maintain2Err,
				maintainErr:  tt.maintainErr,
			}

			b := &Backup{
				tier2Enabled: tt.tier2Enabled,
				steps:        stub,
			}

			task := &queue.BackupTask{ClusterName: "cluster", SendToTier2: tt.sendToTier2}
			err := b.processBackup(context.Background(), task)

			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if stub.verifyCalled != tt.wantVerify {
				t.Errorf("verifyTier1 called = %v, want %v", stub.verifyCalled, tt.wantVerify)
			}
			if stub.relayCalled != tt.wantRelay {
				t.Errorf("relayTier2 called = %v, want %v", stub.relayCalled, tt.wantRelay)
			}
			if stub.maintain2Called != tt.wantMaintain2 {
				t.Errorf("maintainTier2 called = %v, want %v", stub.maintain2Called, tt.wantMaintain2)
			}
			if stub.maintainCalled != tt.wantMaintain {
				t.Errorf("maintainTier1 called = %v, want %v", stub.maintainCalled, tt.wantMaintain)
			}
		})
	}
}
