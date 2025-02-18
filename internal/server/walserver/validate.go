package walserver

import (
	"fmt"
	"strings"
	"unicode"
)

// InvalidRepositoryFileError is raised when a string that will be used as a
// path contains invalid characters.
type InvalidRepositoryFileError struct {
	s string
}

// Error implements the error interface.
func (e *InvalidRepositoryFileError) Error() string {
	return fmt.Sprintf("invalid string: '%s'", e.s)
}

func validatePathComponent(component string) error {
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
