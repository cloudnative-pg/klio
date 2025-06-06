package kopiaserver

import (
	"context"
	"fmt"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/filesystem"
)

// InitOptions are the configuration information
// needed to create a new Kopia repository.
type InitOptions struct {
	// Path is the directory where the repository is created
	Path string

	// Password is used to encrypt the repository
	Password string
}

// Initialize creates a new Kopia repository with the
// specified configuration.
func Initialize(ctx context.Context, opts InitOptions) error {
	storage, err := filesystem.New(ctx, &filesystem.Options{
		Path: opts.Path,
	}, true)
	if err != nil {
		return fmt.Errorf("while creating Kopia filesystem storage: %w", err)
	}

	if err := repo.Initialize(ctx, storage, &repo.NewRepositoryOptions{}, opts.Password); err != nil {
		return fmt.Errorf("while creating the Kopia repository: %w", err)
	}

	return nil
}
