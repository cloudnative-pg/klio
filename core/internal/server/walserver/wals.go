package walserver

import (
	"path"
	"regexp"
	"strings"
)

// walSubdirectoryLength is the length of the prefix of the WAL file
// name that will be used to create the directory where the WAL
// file will be stored.
//
// As an example, with a prefix of 16 characters:
//
//	cluster-example/0000000100000000/00000001000000000000000E
const walSubdirectoryLength = 16

// expectedWalFileNameLength is the expected name of a WAL
// file.
const expectedWalFileNameLength = 24

// walFileRE matches a WAL file regular expression.
var walFileRE = regexp.MustCompile(
	`^` +
		`([\dA-Fa-f]{8})` + // everything has a timeline
		`(?:` +
		`([\dA-Fa-f]{8})([\dA-Fa-f]{8})` + // segment name, if a wal file
		`(?:` + // and optional
		`\.[\dA-Fa-f]{8}\.backup` + // offset, if a backup label
		`|` +
		`\.partial` + // partial, if a partial file
		`)?` +
		`|` +
		`\.history` + // or only .history, if a history file
		`)` +
		`$`)

// getWALArchivePath gets the name of the file where
// the passed WAL file will be archived.
func getWALArchivePath(clusterName, walName string) string {
	walNameWithoutExtension := strings.TrimSuffix(walName, path.Ext(walName))
	if len(walNameWithoutExtension) == expectedWalFileNameLength {
		return path.Join(clusterName, walName[0:walSubdirectoryLength], walName)
	}

	return path.Join(clusterName, walName)
}

// validateWalFileName checks if the passed file name belongs to
// a WAL file or not.
func validateWalFileName(name string) error {
	if !walFileRE.MatchString(name) {
		return NewIncorrectWALNameError(name)
	}

	return nil
}
