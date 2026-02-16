package kopia

import (
	"slices"
	"testing"
)

func TestBuildConnectFSArgs(t *testing.T) {
	baseOpts := FSRepoOpts{
		CommonRepoOpts: CommonRepoOpts{
			CacheDirectory: "/cache",
		},
		DataDirectory: "/data/repo",
	}

	t.Run("ReadOnly true includes --readonly", func(t *testing.T) {
		opts := baseOpts
		opts.ReadOnly = true

		args := buildConnectFSArgs("/etc/kopia/config", opts)

		if !slices.Contains(args, "--readonly") {
			t.Errorf("expected args to contain --readonly, got %v", args)
		}
	})

	t.Run("ReadOnly false excludes --readonly", func(t *testing.T) {
		opts := baseOpts
		opts.ReadOnly = false

		args := buildConnectFSArgs("/etc/kopia/config", opts)

		if slices.Contains(args, "--readonly") {
			t.Errorf("expected args not to contain --readonly, got %v", args)
		}
	})

	t.Run("PersistCredentials true includes --persist-credentials", func(t *testing.T) {
		opts := baseOpts
		opts.PersistCredentials = true

		args := buildConnectFSArgs("/etc/kopia/config", opts)

		if !slices.Contains(args, "--persist-credentials") {
			t.Errorf("expected args to contain --persist-credentials, got %v", args)
		}
	})

	t.Run("PersistCredentials false excludes --persist-credentials", func(t *testing.T) {
		opts := baseOpts
		opts.PersistCredentials = false

		args := buildConnectFSArgs("/etc/kopia/config", opts)

		if slices.Contains(args, "--persist-credentials") {
			t.Errorf("expected args not to contain --persist-credentials, got %v", args)
		}
	})

	t.Run("both ReadOnly and PersistCredentials", func(t *testing.T) {
		opts := baseOpts
		opts.ReadOnly = true
		opts.PersistCredentials = true

		args := buildConnectFSArgs("/etc/kopia/config", opts)

		if !slices.Contains(args, "--readonly") {
			t.Errorf("expected args to contain --readonly, got %v", args)
		}
		if !slices.Contains(args, "--persist-credentials") {
			t.Errorf("expected args to contain --persist-credentials, got %v", args)
		}
	})

	t.Run("config file and data directory are included", func(t *testing.T) {
		opts := baseOpts

		args := buildConnectFSArgs("/custom/path/config.json", opts)

		if !slices.Contains(args, "--config-file=/custom/path/config.json") {
			t.Errorf("expected args to contain config file path, got %v", args)
		}
		if !slices.Contains(args, "--path=/data/repo") {
			t.Errorf("expected args to contain data directory path, got %v", args)
		}
	})
}
