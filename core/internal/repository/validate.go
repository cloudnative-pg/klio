package repository

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

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

// InvalidRepositoryFileError is raised when a string that will be used as a
// path contains invalid characters.
type InvalidRepositoryFileError struct {
	s string
}

// Error implements the error interface.
func (e *InvalidRepositoryFileError) Error() string {
	return fmt.Sprintf("invalid string: '%s'", e.s)
}

// ValidatePathComponent validates each one of the paths that can be stored
// in a repository. Examples are the name of the cluster and the name of the
// WAL files.
func ValidatePathComponent(component string) error {
	isInvalidCharacter := func(character rune) bool {
		if unicode.IsLetter(character) {
			return false
		}

		if unicode.IsNumber(character) {
			return false
		}

		if character == '_' {
			return false
		}

		if character == '-' {
			return false
		}

		if character == '.' {
			return false
		}

		return true
	}

	if strings.ContainsFunc(component, isInvalidCharacter) {
		return &InvalidRepositoryFileError{s: component}
	}

	return nil
}

// ValidateWalFileName checks if a file name can be a WAL file name.
func ValidateWalFileName(name string) error {
	if !walFileRE.MatchString(name) {
		return NewIncorrectWALNameError(name)
	}

	return nil
}
