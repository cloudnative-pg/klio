package kopia

import (
	"context"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// SetRetentionPolicy sets the retention policy for backups of this cluster.
func (s *Connection) SetRetentionPolicy(ctx context.Context, t kopia.Target, p kopia.RetentionPolicy) error {
	return s.kopia.SetKopiaPolicy(ctx, t, &p)
}

// GetRetentionPolicy gets the currently applied retention policy for this cluster.
func (s *Connection) GetRetentionPolicy(ctx context.Context, t kopia.Target) (*kopia.RetentionPolicy, error) {
	policy, err := s.kopia.GetCurrentKopiaPolicy(ctx, t)
	if err != nil {
		return nil, err
	}

	return &policy.RetentionPolicy, nil
}

// ApplyRetentionPolicy applies the retention policy for this cluster, deleting any
// snapshots that are no longer needed.
func (s *Connection) ApplyRetentionPolicy(ctx context.Context, t kopia.Target) error {
	return s.kopia.ApplyKopiaPolicy(ctx, t)
}
