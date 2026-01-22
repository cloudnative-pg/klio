package kopiaconfig

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// VerifyTier1KopiaRepository verifies that a Tier1 Kopia repository can be accessed with the provided credentials.
func VerifyTier1KopiaRepository(ctx context.Context, cfg *config.Tier1Config) error {
	return verifyKopiaRepository("tier1", func(tmpName string) error {
		return CreateTier1KopiaConfigFile(ctx, tmpName, cfg)
	})
}

// VerifyTier2KopiaRepository verifies that a Tier2 Kopia repository can be accessed with the provided credentials.
func VerifyTier2KopiaRepository(ctx context.Context, cfg *config.Tier2Config) error {
	return verifyKopiaRepository("tier2", func(tmpName string) error {
		return CreateTier2KopiaConfigFile(ctx, tmpName, cfg)
	})
}

// verifyKopiaRepository is a helper that manages the lifecycle of a temporary configuration file.
// It handles the file creation, ensuring the file is closed and deleted after use.
func verifyKopiaRepository(tierLabel string, createKopiaConfigFile func(string) error) error {
	pattern := fmt.Sprintf("kopiaconfig_verify_%s_*", tierLabel)

	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return fmt.Errorf("while creating temporary config file: %w", err)
	}

	tmpFileName := tmpFile.Name()
	_ = tmpFile.Close()

	defer func() {
		_ = os.Remove(tmpFileName)
	}()

	if err := createKopiaConfigFile(tmpFileName); err != nil {
		return fmt.Errorf("while verifying %s Kopia repository: %w", tierLabel, err)
	}

	return nil
}
