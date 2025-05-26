package walserver

import (
	"path"
	"regexp"
	"strings"
)

// expectedWalFileNameLength is the expected name of a WAL
// file
const expectedWalFileNameLength = 24

// walFileRE matches a WAL file regular expression
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
func getWALArchivePath(baseDir, clusterName, walName string) string {
	walNameWithoutExtension := strings.TrimSuffix(walName, path.Ext(walName))
	if len(walNameWithoutExtension) == expectedWalFileNameLength {
		return path.Join(baseDir, clusterName, walName[0:16], walName)
	}
	return path.Join(baseDir, clusterName, walName)
}

// validateWalFileName checks if the passed file name belongs to
// a WAL file or not
func validateWalFileName(name string) error {
	if !walFileRE.Match([]byte(name)) {
		return NewIncorrectWALNameError(name)
	}
	return nil
}
