package kopia

import (
	"slices"
	"strings"
	"testing"
)

const kopiaCheckForUpdatesEnv = "KOPIA_CHECK_FOR_UPDATES=false"

func TestEnvPassword(t *testing.T) {
	cases := []struct {
		name           string
		client         Client
		mustContain    []string
		mustNotContain []string
	}{
		{
			name: "Password provided",
			client: Client{
				Password: "password",
			},
			mustContain: []string{kopiaCheckForUpdatesEnv, "KOPIA_PASSWORD=password"}, // NOSONAR
		},
		{
			name: "Empty password",
			client: Client{
				Password: "",
			},
			mustContain:    []string{kopiaCheckForUpdatesEnv},
			mustNotContain: []string{"KOPIA_PASSWORD="},
		},
		{
			name: "Other fields set but no password",
			client: Client{
				ConfigFile:  "/path/to/config",
				KopiaBinary: "/path/to/kopia",
				Password:    "",
			},
			mustContain:    []string{kopiaCheckForUpdatesEnv},
			mustNotContain: []string{"KOPIA_PASSWORD="},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.client.kopiaEnvironmentVariables()

			for _, expected := range tc.mustContain {
				if !slices.Contains(got, expected) {
					t.Errorf("kopiaEnvironmentVariables() missing %q", expected)
				}
			}

			for _, notExpected := range tc.mustNotContain {
				for _, env := range got {
					if strings.HasPrefix(env, notExpected) {
						t.Errorf("kopiaEnvironmentVariables() unexpectedly contains %q", env)
					}
				}
			}
		})
	}
}
