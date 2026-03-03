package kopia

import (
	"slices"
	"testing"
)

const kopiaCheckForUpdatesEnv = "KOPIA_CHECK_FOR_UPDATES=false"

func TestEnvPassword(t *testing.T) {
	cases := []struct {
		name     string
		client   Client
		expected []string
	}{
		{
			name: "Password provided",
			client: Client{
				Password: "password",
			},
			expected: []string{kopiaCheckForUpdatesEnv, "KOPIA_PASSWORD=password"}, // NOSONAR
		},
		{
			name: "Empty password",
			client: Client{
				Password: "",
			},
			expected: []string{kopiaCheckForUpdatesEnv},
		},
		{
			name: "Other fields set but no password",
			client: Client{
				ConfigFile:  "/path/to/config",
				KopiaBinary: "/path/to/kopia",
				Password:    "",
			},
			expected: []string{kopiaCheckForUpdatesEnv},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.client.kopiaEnvironmentVariables()

			if !slices.Equal(got, tc.expected) {
				t.Errorf("envPassword() = %v, want %v", got, tc.expected)
			}
		})
	}
}
