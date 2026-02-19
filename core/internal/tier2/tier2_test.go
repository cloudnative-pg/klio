package tier2

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

type clientVerifier struct {
	region              string
	endpoint            string
	expectPathStyle     bool
	accessKeyID         string
	secretAccessKey     string
	sessionToken        string
	checkCredentials    bool
	endpointShouldBeNil bool
}

//nolint:cyclop // Test helper with many verification checks
func verifyClient(ctx context.Context, t *testing.T, client *s3.Client, v clientVerifier) {
	t.Helper()

	opts := client.Options()

	// Verify region if specified
	if v.region != "" && opts.Region != v.region {
		t.Errorf("Expected region %q, got %q", v.region, opts.Region)
	}

	// Verify endpoint
	if v.endpointShouldBeNil {
		if opts.BaseEndpoint != nil {
			t.Errorf("Expected endpoint to be nil, got %v", *opts.BaseEndpoint)
		}
	} else if v.endpoint != "" {
		if opts.BaseEndpoint == nil || *opts.BaseEndpoint != v.endpoint {
			t.Errorf("Expected endpoint %q, got %v", v.endpoint, opts.BaseEndpoint)
		}
	}

	// Verify path style
	if opts.UsePathStyle != v.expectPathStyle {
		t.Errorf("Expected UsePathStyle=%v, got %v", v.expectPathStyle, opts.UsePathStyle)
	}

	// Verify credentials if requested
	if v.checkCredentials {
		creds, err := opts.Credentials.Retrieve(ctx)
		if err != nil {
			t.Fatalf("Failed to retrieve credentials: %v", err)
		}

		if v.accessKeyID != "" && creds.AccessKeyID != v.accessKeyID {
			t.Errorf("Expected AccessKeyID %q, got %q", v.accessKeyID, creds.AccessKeyID)
		}

		if v.secretAccessKey != "" && creds.SecretAccessKey != v.secretAccessKey {
			t.Errorf("Expected SecretAccessKey %q, got %q", v.secretAccessKey, creds.SecretAccessKey)
		}

		if v.sessionToken != "" && creds.SessionToken != v.sessionToken {
			t.Errorf("Expected SessionToken %q, got %q", v.sessionToken, creds.SessionToken)
		}
	}
}

func TestCreateAWSS3Client(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.Tier2Config
		verifier clientVerifier
	}{
		{
			name: "Explicit credentials with endpoint",
			cfg: config.Tier2Config{
				//nolint:gosec // keys are examples and not an issue
				S3: config.S3Configuration{
					BucketName:      "test-bucket",
					Region:          "us-east-1",
					Endpoint:        "https://s3.us-east-1.amazonaws.com",
					AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
					SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				},
			},
			//nolint:gosec // keys are examples and not an issue
			verifier: clientVerifier{
				region:           "us-east-1",
				endpoint:         "https://s3.us-east-1.amazonaws.com",
				expectPathStyle:  true,
				accessKeyID:      "AKIAIOSFODNN7EXAMPLE",
				secretAccessKey:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				checkCredentials: true,
			},
		},
		{
			name: "IAM role without credentials",
			cfg: config.Tier2Config{
				S3: config.S3Configuration{
					BucketName: "test-bucket",
					Region:     "us-east-1",
				},
			},
			verifier: clientVerifier{
				region:              "us-east-1",
				expectPathStyle:     false,
				endpointShouldBeNil: true,
			},
		},
		{
			name: "Session token with credentials",
			cfg: config.Tier2Config{
				//nolint:gosec // keys are examples and not an issue
				S3: config.S3Configuration{
					BucketName:      "test-bucket",
					Region:          "us-east-1",
					AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
					SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
					SessionToken:    "session-token-example",
				},
			},
			//nolint:gosec // keys are examples and not an issue
			verifier: clientVerifier{
				region:           "us-east-1",
				accessKeyID:      "AKIAIOSFODNN7EXAMPLE",
				secretAccessKey:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				sessionToken:     "session-token-example",
				checkCredentials: true,
			},
		},
		{
			name: "Explicit credentials without endpoint",
			cfg: config.Tier2Config{
				//nolint:gosec // keys are examples and not an issue
				S3: config.S3Configuration{
					BucketName:      "test-bucket",
					Region:          "us-east-1",
					AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
					SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				},
			},
			//nolint:gosec // keys are examples and not an issue
			verifier: clientVerifier{
				region:              "us-east-1",
				expectPathStyle:     false,
				endpointShouldBeNil: true,
				accessKeyID:         "AKIAIOSFODNN7EXAMPLE",
				secretAccessKey:     "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				checkCredentials:    true,
			},
		},
		{
			name: "Without region - uses SDK defaults",
			cfg: config.Tier2Config{
				//nolint:gosec // keys are examples and not an issue
				S3: config.S3Configuration{
					BucketName:      "test-bucket",
					AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
					SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				},
			},
			//nolint:gosec // keys are examples and not an issue
			verifier: clientVerifier{
				region:              "",
				expectPathStyle:     false,
				endpointShouldBeNil: true,
				accessKeyID:         "AKIAIOSFODNN7EXAMPLE",
				secretAccessKey:     "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				checkCredentials:    true,
			},
		},
		{
			name: "S3-compatible storage with explicit credentials",
			cfg: config.Tier2Config{
				S3: config.S3Configuration{
					BucketName:      "bucket",
					Region:          "us-east-1",
					Endpoint:        "https://s3-compatible.example.com",
					AccessKeyID:     "AKIA...",
					SecretAccessKey: "secret",
				},
			},
			verifier: clientVerifier{
				region:           "us-east-1",
				endpoint:         "https://s3-compatible.example.com",
				expectPathStyle:  true,
				accessKeyID:      "AKIA...",
				secretAccessKey:  "secret",
				checkCredentials: true,
			},
		},
		{
			name: "IAM role with custom endpoint",
			cfg: config.Tier2Config{
				S3: config.S3Configuration{
					BucketName: "bucket",
					Region:     "us-east-1",
					Endpoint:   "https://custom-s3.example.com",
				},
			},
			verifier: clientVerifier{
				region:          "us-east-1",
				endpoint:        "https://custom-s3.example.com",
				expectPathStyle: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client, err := CreateAWSS3Client(ctx, &tt.cfg)
			if err != nil {
				t.Fatalf("CreateAWSS3Client failed: %v", err)
			}

			if client == nil {
				t.Fatal("Expected client to be non-nil")
			}

			verifyClient(ctx, t, client, tt.verifier)
		})
	}
}

func TestConnectBase(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Tier2Config
	}{
		{
			name: "With explicit credentials",
			cfg: config.Tier2Config{
				//nolint:gosec // keys are examples and not an issue
				S3: config.S3Configuration{
					BucketName:      "test-bucket",
					Region:          "us-east-1",
					Prefix:          "test-prefix",
					AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
					SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				},
			},
		},
		{
			name: "With IAM role",
			cfg: config.Tier2Config{
				S3: config.S3Configuration{
					BucketName: "test-bucket",
					Region:     "us-east-1",
					Prefix:     "test-prefix",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fs, err := ConnectBase(ctx, &tt.cfg)
			if err != nil {
				t.Fatalf("ConnectBase failed: %v", err)
			}

			if fs == nil {
				t.Fatal("Expected fs to be non-nil")
			}
		})
	}
}

func TestConnectWAL(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Tier2Config
	}{
		{
			name: "With explicit credentials",
			cfg: config.Tier2Config{
				//nolint:gosec // keys are examples and not an issue
				S3: config.S3Configuration{
					BucketName:      "test-bucket",
					Region:          "us-east-1",
					Prefix:          "test-prefix",
					AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
					SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				},
			},
		},
		{
			name: "With IAM role",
			cfg: config.Tier2Config{
				S3: config.S3Configuration{
					BucketName: "test-bucket",
					Region:     "us-east-1",
					Prefix:     "test-prefix",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fs, err := ConnectWAL(ctx, &tt.cfg)
			if err != nil {
				t.Fatalf("ConnectWAL failed: %v", err)
			}

			if fs == nil {
				t.Fatal("Expected fs to be non-nil")
			}
		})
	}
}
