package kopia

import (
	"slices"
	"testing"
)

func TestBuildConnectRemoteArgs(t *testing.T) {
	baseOpts := RemoteRepoOpts{
		CommonRepoOpts: CommonRepoOpts{
			CacheDirectory: "/cache",
		},
		URL:                   "https://kopia.example.com:51515",
		ClientCertPath:        "/certs/client.crt",
		ClientKeyPath:         "/certs/client.key",
		ServerCertFingerprint: "sha256:abc123",
		Username:              "testuser",
		Hostname:              "testhost",
	}

	t.Run("ReadOnly true includes --readonly", func(t *testing.T) {
		opts := baseOpts
		opts.ReadOnly = true

		args := buildConnectRemoteArgs("/etc/kopia/config", opts)

		if !slices.Contains(args, "--readonly") {
			t.Errorf("expected args to contain --readonly, got %v", args)
		}
	})

	t.Run("ReadOnly false excludes --readonly", func(t *testing.T) {
		opts := baseOpts
		opts.ReadOnly = false

		args := buildConnectRemoteArgs("/etc/kopia/config", opts)

		if slices.Contains(args, "--readonly") {
			t.Errorf("expected args not to contain --readonly, got %v", args)
		}
	})

	t.Run("config file and URL are included", func(t *testing.T) {
		opts := baseOpts

		args := buildConnectRemoteArgs("/custom/path/config.json", opts)

		if !slices.Contains(args, "--config-file=/custom/path/config.json") {
			t.Errorf("expected args to contain config file path, got %v", args)
		}
		if !slices.Contains(args, "--url=https://kopia.example.com:51515") {
			t.Errorf("expected args to contain URL, got %v", args)
		}
	})

	t.Run("client certificate paths are included", func(t *testing.T) {
		opts := baseOpts

		args := buildConnectRemoteArgs("/etc/kopia/config", opts)

		if !slices.Contains(args, "--client-certificate=/certs/client.crt") {
			t.Errorf("expected args to contain client cert path, got %v", args)
		}
		if !slices.Contains(args, "--client-key=/certs/client.key") {
			t.Errorf("expected args to contain client key path, got %v", args)
		}
	})

	t.Run("server fingerprint and overrides are included", func(t *testing.T) {
		opts := baseOpts

		args := buildConnectRemoteArgs("/etc/kopia/config", opts)

		if !slices.Contains(args, "--server-cert-fingerprint=sha256:abc123") {
			t.Errorf("expected args to contain server cert fingerprint, got %v", args)
		}
		if !slices.Contains(args, "--override-username=testuser") {
			t.Errorf("expected args to contain override username, got %v", args)
		}
		if !slices.Contains(args, "--override-hostname=testhost") {
			t.Errorf("expected args to contain override hostname, got %v", args)
		}
	})
}
