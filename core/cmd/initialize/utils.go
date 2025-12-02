package initialize

import (
	"os"

	"github.com/spf13/afero"
)

// canInitRepoDirectory checks whether a directory does not exist or is empty
// and can be used to create a new repository.
func canInitRepoDirectory(fs afero.Fs) (bool, error) {
	entries, err := afero.ReadDir(fs, ".")
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	return len(entries) == 0, nil
}
