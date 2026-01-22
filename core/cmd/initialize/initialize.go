package initialize

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/afero"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/repository"
)

var (
	// ErrWALDirectoryNotEmpty is raised when initializing a WAL directory that is not empty.
	ErrWALDirectoryNotEmpty = errors.New("cannot initialize as Klio WAL directory because it is not empty")

	// ErrKopiaDirectoryNotEmpty is raised when initializing a Kopia repository in a directory that is not
	// empty.
	ErrKopiaDirectoryNotEmpty = errors.New("cannot initialize as Kopia repository because it is not empty")
)

// Options are the options needed to initialize a new
// pair of repositories.
type Options struct {
	// The WAL FS
	WalFS afero.Fs

	// The encryption password to be used for the WALs
	WalEncryptionPassword string

	// The Kopia FS
	KopiaFS afero.Fs

	// The Kopia encryption password
	KopiaEncryptionPassword string

	// A callback to be used to create the underlying Kopia storage.
	KopiaInitializeRepo func() error

	// If true, the initialization process doesn't fail if both
	// the storage areas are initialized.
	SkipIfExisting bool
}

// Run initializes the Klio WAL and the Kopia repository specified by the
// options.
func Run(ctx context.Context, opts Options) error {
	var walDirectoryIsEmpty, kopiaDirectoryIsEmpty bool
	var err error

	contextLogger := log.FromContext(ctx)

	walDirectoryIsEmpty, err = canInitRepoDirectory(opts.WalFS)
	if err != nil {
		return fmt.Errorf("while checking if the Klio WAL FS is safe to use: %w", err)
	}

	kopiaDirectoryIsEmpty, err = canInitRepoDirectory(opts.KopiaFS)
	if err != nil {
		return fmt.Errorf("while checking if the Kopia repository is safe to use: %w", err)
	}

	switch {
	case walDirectoryIsEmpty && kopiaDirectoryIsEmpty:
		if err := repository.Initialize(repository.Options{
			FS:       opts.WalFS,
			Password: opts.WalEncryptionPassword,
		}); err != nil {
			return fmt.Errorf("while initializing the Klio WAL directory, %w", err)
		}

		if err := opts.KopiaInitializeRepo(); err != nil {
			return fmt.Errorf("while initializing the Kopia repository directory, %w", err)
		}

	case opts.SkipIfExisting:
		contextLogger.Info(
			"The klio repository already exists, skipping initialization.",
			"walDirectoryIsEmpty", walDirectoryIsEmpty,
			"kopiaDirectoryIsEmpty", kopiaDirectoryIsEmpty,
		)

	case !walDirectoryIsEmpty:
		return ErrWALDirectoryNotEmpty

	case !kopiaDirectoryIsEmpty:
		return ErrKopiaDirectoryNotEmpty
	}

	return nil
}
