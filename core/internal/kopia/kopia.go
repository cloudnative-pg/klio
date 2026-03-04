package kopia

import (
	"fmt"
	"os"
	"os/exec"
)

// Binary is name of the Kopia executable.
const Binary = "kopia"

// LookupBinary finds the Kopia binary in the system PATH and returns its absolute path.
// Returns an error if the binary is not found.
func LookupBinary() (string, error) {
	kopiaBinary, err := exec.LookPath(Binary)
	if err != nil {
		return "", fmt.Errorf("kopia binary not found (%q): %w", Binary, err)
	}

	return kopiaBinary, nil
}

// Client is a wrapper around the Kopia CLI.
type Client struct {
	// ConfigFile is the path to the Kopia configuration file.
	ConfigFile string

	// KopiaBinary is the path to the Kopia binary executable.
	KopiaBinary string

	// Password is the repository encryption password.
	Password string //nolint:gosec
}

func (s *Client) kopiaEnvironmentVariables() []string {
	result := os.Environ()
	result = append(result, "KOPIA_CHECK_FOR_UPDATES=false")
	if s.Password != "" {
		result = append(result, "KOPIA_PASSWORD="+s.Password)
	}

	return result
}

// CommonRepoOpts contains common options for repository operations.
type CommonRepoOpts struct {
	// KopiaBinary is the path to the Kopia binary executable.
	KopiaBinary string

	// EncryptionPassword is the password used to encrypt the repository.
	EncryptionPassword string

	// PersistCredentials indicates whether credentials should be persisted in the config file.
	PersistCredentials bool

	// CacheDirectory is the directory used for caching repository data.
	CacheDirectory string

	// ReadOnly indicates whether the repository connection should be read-only.
	// When true, the --readonly flag is passed to `kopia repository connect`,
	// which is the correct way to enforce read-only access at the repository level.
	ReadOnly bool
}
