package kopia

// Client is a wrapper around the Kopia CLI.
type Client struct {
	// ConfigFile is the path to the Kopia configuration file.
	ConfigFile string

	// KopiaBinary is the path to the Kopia binary executable.
	KopiaBinary string

	// Password is the repository encryption password.
	Password string
}

func (s *Client) envPassword() []string {
	if s.Password == "" {
		return nil
	}

	return []string{
		"KOPIA_PASSWORD=" + s.Password,
	}
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
}
