package kopiaserver

import (
	"context"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

func enableACLs(ctx context.Context, kopiaBinary string, configFileName string) {
	contextLogger := log.FromContext(ctx)

	client := &kopia.Client{
		ConfigFile:  configFileName,
		KopiaBinary: kopiaBinary,
	}

	alreadyEnabled, err := client.EnableACL(ctx)
	if err != nil {
		contextLogger.Error(err, "failed to execute ACLs enablement")
		return
	}

	if alreadyEnabled {
		contextLogger.Info("ACLs already enabled", "configFileName", configFileName)
	} else {
		contextLogger.Info("ACLs enabled", "configFileName", configFileName)
	}

	err = client.AddACLUser(ctx, kopia.AddACLUserOptions{
		User:      "snapshot_reader@klio",
		Access:    "READ",
		Target:    "type=snapshot",
		Overwrite: true,
	})
	if err != nil {
		contextLogger.Error(err, "failed to add snapshot_reader to ACLs")
		return
	}

	contextLogger.Info("User snapshot_reader added to ACLs", "configFileName", configFileName)
}
