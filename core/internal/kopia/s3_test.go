package kopia

import (
	"slices"
	"testing"
)

func TestBuildConnectS3Args(t *testing.T) {
	baseOpts := func() S3RepoOpts {
		return S3RepoOpts{
			CommonRepoOpts: CommonRepoOpts{
				CacheDirectory: "/cache",
			},
			BucketName: "my-bucket",
			Endpoint:   "https://s3.example.com",
			Prefix:     "backups",
		}
	}

	cases := []struct {
		name       string
		configFile string
		mutate     func(*S3RepoOpts)
		wantFlag   string
		wantAbsent string
	}{
		{
			name:     "ReadOnly true includes --readonly",
			mutate:   func(o *S3RepoOpts) { o.ReadOnly = true },
			wantFlag: "--readonly",
		},
		{
			name:       "ReadOnly false excludes --readonly",
			mutate:     func(o *S3RepoOpts) { o.ReadOnly = false },
			wantAbsent: "--readonly",
		},
		{
			name:     "PersistCredentials true includes --persist-credentials",
			mutate:   func(o *S3RepoOpts) { o.PersistCredentials = true },
			wantFlag: "--persist-credentials",
		},
		{
			name:       "PersistCredentials false excludes --persist-credentials",
			mutate:     func(o *S3RepoOpts) { o.PersistCredentials = false },
			wantAbsent: "--persist-credentials",
		},
		{
			name:       "config file path is included",
			configFile: "/custom/path/config.json",
			wantFlag:   "--config-file=/custom/path/config.json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := baseOpts()
			if tc.mutate != nil {
				tc.mutate(&opts)
			}

			cfgFile := "/etc/kopia/config"
			if tc.configFile != "" {
				cfgFile = tc.configFile
			}

			args, err := buildConnectS3Args(cfgFile, opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantFlag != "" && !slices.Contains(args, tc.wantFlag) {
				t.Errorf("expected args to contain %q, got %v", tc.wantFlag, args)
			}
			if tc.wantAbsent != "" && slices.Contains(args, tc.wantAbsent) {
				t.Errorf("expected args not to contain %q, got %v", tc.wantAbsent, args)
			}
		})
	}
}

func TestBuildConnectS3ArgsBothFlags(t *testing.T) {
	opts := S3RepoOpts{
		CommonRepoOpts: CommonRepoOpts{
			CacheDirectory:     "/cache",
			ReadOnly:           true,
			PersistCredentials: true,
		},
		BucketName: "my-bucket",
		Endpoint:   "https://s3.example.com",
		Prefix:     "backups",
	}

	args, err := buildConnectS3Args("/etc/kopia/config", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !slices.Contains(args, "--readonly") {
		t.Errorf("expected args to contain --readonly, got %v", args)
	}
	if !slices.Contains(args, "--persist-credentials") {
		t.Errorf("expected args to contain --persist-credentials, got %v", args)
	}
}

func TestBuildConnectS3ArgsInvalidEndpoint(t *testing.T) {
	opts := S3RepoOpts{
		CommonRepoOpts: CommonRepoOpts{
			CacheDirectory: "/cache",
		},
		BucketName: "my-bucket",
		Endpoint:   "://invalid",
		Prefix:     "backups",
	}

	_, err := buildConnectS3Args("/etc/kopia/config", opts)
	if err == nil {
		t.Error("expected error for invalid endpoint, got nil")
	}
}
