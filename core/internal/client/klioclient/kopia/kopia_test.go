package kopia

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHostname(t *testing.T) {
	type testCase struct {
		name         string
		input        string
		expectedUser string
		expectedHost string
		expectedErr  string
	}

	// Define the table of test cases
	testCases := []testCase{
		{
			name:         "happy path",
			input:        "klio@host",
			expectedUser: "klio",
			expectedHost: "host",
			expectedErr:  "",
		},
		{
			name:         "empty common name",
			input:        "",
			expectedUser: "",
			expectedHost: "",
			expectedErr:  `commonName must be in the form userName@hostName, got ""`,
		},
		{
			name:         "invalid common name format with no @",
			input:        "invalidCommonName",
			expectedUser: "",
			expectedHost: "",
			expectedErr:  `commonName must be in the form userName@hostName, got "invalidCommonName"`,
		},
		{
			name:         "invalid common name with multiple @",
			input:        "klio@host@secondHost",
			expectedUser: "",
			expectedHost: "",
			expectedErr:  `commonName must be in the form userName@hostName, got "klio@host@secondHost"`,
		},
		{
			name:         "invalid common name with no user part",
			input:        "@hostname",
			expectedUser: "",
			expectedHost: "",
			expectedErr:  `userName part in commonName cannot be empty, got "@hostname"`,
		},
		{
			name:         "invalid common name with no hostname part",
			input:        "klio@",
			expectedUser: "",
			expectedHost: "",
			expectedErr:  `hostName part in commonName cannot be empty, got "klio@"`,
		},
	}

	// Iterate through test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualUser, actualHost, actualErr := extractUserNameAndHostName(tc.input)

			assert.Equal(t, tc.expectedUser, actualUser, "The extracted userName did not match the expectation")
			assert.Equal(t, tc.expectedHost, actualHost, "The extracted hostName did not match the expectation")

			if tc.expectedErr == "" {
				// We expect no error
				assert.NoError(t, actualErr, "Expected no error, but got one")
			} else {
				// We expect an error
				require.Error(t, actualErr, "Expected an error, but got none")
				assert.Equal(t, tc.expectedErr, actualErr.Error(), "The error message did not match the expected message")
			}
		})
	}
}
