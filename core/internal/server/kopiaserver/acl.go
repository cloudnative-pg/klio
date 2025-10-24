package kopiaserver

import (
	"context"
	"os/exec"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

const aclEnabledMatchStr = "ACLs already enabled"

func enableACLs(ctx context.Context, kopiaBinary string, configFileName string, password string) {
	contextLogger := log.FromContext(ctx)

	kopiaArgs := []string{
		"--config-file=" + configFileName,
		"--log-dir=/tmp",
		"server",
		"acl",
		"enable",
	}

	//nolint:gosec
	cmd := exec.CommandContext(ctx, kopiaBinary, kopiaArgs...)
	cmd.Env = []string{"KOPIA_PASSWORD=" + password}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if isACLsEnabled(string(output)) {
			contextLogger.Info(aclEnabledMatchStr, "configFileName", configFileName)
		} else {
			contextLogger.Error(err, "failed to execute ACLs enablement:")
			return
		}
	} else {
		contextLogger.Info("ACLs enabled", "configFileName", configFileName)
	}

	kopiaArgs = []string{
		"--config-file=" + configFileName,
		"--log-dir=/tmp",
		"server",
		"acl",
		"add",
		"--overwrite",
		"--access=READ",
		"--target=type=snapshot",
		"--user=snapshot_reader@klio",
	}

	//nolint:gosec
	cmd = exec.CommandContext(ctx, kopiaBinary, kopiaArgs...)
	cmd.Env = []string{"KOPIA_PASSWORD=" + password}

	err = cmd.Run()
	if err != nil {
		contextLogger.Error(err, "failed to add snapshot_reader to ACLs:")
		return
	}

	contextLogger.Info("User snapshot_reader added to ACLs", "configFileName", configFileName)
}

func isACLsEnabled(output string) bool {
	return strings.Contains(output, aclEnabledMatchStr)
}
