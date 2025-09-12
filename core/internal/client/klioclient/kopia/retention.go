package kopia

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
)

// SetRetentionPolicy sets the retention policy for backups of this cluster.
func (s *Connection) SetRetentionPolicy(ctx context.Context, t Target, p policy.RetentionPolicy) error {
	contextLogger := log.FromContext(ctx)

	currentPolicy, err := s.getCurrentKopiaPolicy(ctx, t)
	if err != nil {
		return err
	}

	if currentPolicy == nil {
		currentPolicy = &policy.Policy{}
	}
	currentPolicy.RetentionPolicy = p

	ctx, writer, err := s.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: fmt.Sprintf("setting retention policy for %q", t.Hostname),
	})
	if err != nil {
		return fmt.Errorf("while creating repository writer session: %w", err)
	}
	defer func() {
		err := writer.Close(ctx)
		if err != nil {
			contextLogger.Error(err, "while closing repository write session to archive WALs")
		}
	}()

	err = policy.SetPolicy(ctx, writer, s.getPolicyTarget(t), currentPolicy)
	if err != nil {
		return fmt.Errorf("while writing policy for %q to repository: %w", t.Hostname, err)
	}

	if err := writer.Flush(ctx); err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	return nil
}

// GetRetentionPolicy gets the currently applied retention policy for this cluster.
func (s *Connection) GetRetentionPolicy(ctx context.Context, t Target) (*policy.RetentionPolicy, error) {
	currentPolicy, err := s.getCurrentKopiaPolicy(ctx, t)
	if err != nil {
		return nil, err
	}

	if currentPolicy == nil {
		return nil, nil
	}

	return &currentPolicy.RetentionPolicy, nil
}

func (s *Connection) getCurrentKopiaPolicy(ctx context.Context, t Target) (*policy.Policy, error) {
	tree, err := policy.TreeForSource(ctx, s.repository, s.getPolicyTarget(t))
	if err != nil {
		return nil, fmt.Errorf("error while getting the policy tree for host %q: %w", t.Hostname, err)
	}

	return tree.EffectivePolicy(), nil
}

func (s *Connection) getPolicyTarget(t Target) snapshot.SourceInfo {
	return snapshot.SourceInfo{
		UserName: t.Username,
		Host:     t.Hostname,
	}
}

// ApplyRetentionPolicy applies the retention policy for this cluster, deleting any
// snapshots that are no longer needed.
func (s *Connection) ApplyRetentionPolicy(ctx context.Context, t Target) error {
	ctx, writer, err := s.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: fmt.Sprintf("applying retention policy for %q", t.Hostname),
	})
	if err != nil {
		return fmt.Errorf("while creating repository writer session: %w", err)
	}
	defer func() {
		err := writer.Close(ctx)
		if err != nil {
			log.FromContext(ctx).Error(err, "while closing repository write session to apply retention policy")
		}
	}()

	_, err = policy.ApplyRetentionPolicy(ctx, writer, s.getPolicyTarget(t), true)
	if err != nil {
		return fmt.Errorf("while applying retention policy for source %v", s.getPolicyTarget(t))
	}

	if err := writer.Flush(ctx); err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	return nil
}
