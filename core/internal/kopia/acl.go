package kopia

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// EnableACL enables ACLs for the Kopia server.
// Returns true if ACLs were already enabled, false if they were just enabled.
func (s *Client) EnableACL(ctx context.Context) (bool, error) {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"--config-file=" + s.ConfigFile,
		"--log-dir=/tmp",
		"server",
		"acl",
		"enable",
	}

	cmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	cmd.Env = s.envPassword()

	contextLogger.Info("Enabling Kopia ACLs", "args", args)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if ACLs are already enabled
		const aclEnabledMatchStr = "ACLs already enabled"
		if bytes.Contains(output, []byte(aclEnabledMatchStr)) {
			return true, nil
		}

		return false, fmt.Errorf("while enabling Kopia ACLs: %w (output: %s)", err, string(output))
	}

	return false, nil
}

// AddACLUserOptions contains options for adding an ACL user.
type AddACLUserOptions struct {
	// User is the username to grant access to.
	User string

	// Access is the access level (e.g., "READ", "WRITE").
	Access string

	// Target is the target specification (e.g., "type=snapshot").
	Target string

	// Overwrite indicates whether to overwrite existing ACL entries.
	Overwrite bool
}

// AddACLUser adds a user to the Kopia server ACLs.
func (s *Client) AddACLUser(ctx context.Context, opts AddACLUserOptions) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"--config-file=" + s.ConfigFile,
		"--log-dir=/tmp",
		"server",
		"acl",
		"add",
		"--access=" + opts.Access,
		"--target=" + opts.Target,
		"--user=" + opts.User,
	}

	if opts.Overwrite {
		args = append(args[:5], append([]string{"--overwrite"}, args[5:]...)...)
	}

	cmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	cmd.Env = s.envPassword()

	contextLogger.Info("Adding user to Kopia ACLs", "args", args, "user", opts.User)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("while adding user %s to Kopia ACLs: %w", opts.User, err)
	}

	return nil
}
