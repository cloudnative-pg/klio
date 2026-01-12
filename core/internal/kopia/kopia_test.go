package kopia

import (
	"slices"
	"testing"
)

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
			expected: []string{"KOPIA_PASSWORD=password"},
		},
		{
			name: "Empty password returns nil",
			client: Client{
				Password: "",
			},
			expected: nil,
		},
		{
			name: "Other fields set but no password",
			client: Client{
				ConfigFile:  "/path/to/config",
				KopiaBinary: "/path/to/kopia",
				Password:    "",
			},
			expected: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.client.envPassword()

			if !slices.Equal(got, tc.expected) {
				t.Errorf("envPassword() = %v, want %v", got, tc.expected)
			}
		})
	}
}
