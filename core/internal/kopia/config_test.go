package kopia

import (
	"os"
	"testing"
)

func TestParseConfigFile(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expected    *ConfigData
		expectError bool
	}{
		{
			name:    "valid config file",
			content: `{"hostname": "test-host", "username": "test-user", "description": "test desc"}`,
			expected: &ConfigData{
				HostName:    "test-host",
				UserName:    "test-user",
				Description: "test desc",
			},
			expectError: false,
		},
		{
			name:    "config with only hostname and username",
			content: `{"hostname": "host1", "username": "user1"}`,
			expected: &ConfigData{
				HostName: "host1",
				UserName: "user1",
			},
			expectError: false,
		},
		{
			name:    "empty JSON object",
			content: `{}`,
			expected: &ConfigData{
				HostName: "",
				UserName: "",
			},
			expectError: false,
		},
		{
			name:        "invalid JSON",
			content:     `{invalid json}`,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile := createTempConfigFile(t, tc.content)
			result, err := ParseConfigFile(tmpFile)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}

				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			assertConfigDataEqual(t, result, tc.expected)
		})
	}
}

func createTempConfigFile(t *testing.T, content string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "kopia_config_test_*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	return tmpFile.Name()
}

func assertConfigDataEqual(t *testing.T, got, want *ConfigData) {
	t.Helper()

	if got.HostName != want.HostName {
		t.Errorf("HostName = %q, want %q", got.HostName, want.HostName)
	}
	if got.UserName != want.UserName {
		t.Errorf("UserName = %q, want %q", got.UserName, want.UserName)
	}
	if got.Description != want.Description {
		t.Errorf("Description = %q, want %q", got.Description, want.Description)
	}
}

func TestParseConfigFileNonExistentFile(t *testing.T) {
	_, err := ParseConfigFile("/nonexistent/path/to/config.json")
	if err == nil {
		t.Error("expected error for non-existent file, got none")
	}
}
