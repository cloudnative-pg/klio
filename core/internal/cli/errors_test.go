package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodedErrorExitCode(t *testing.T) {
	err := NewCodedError(errors.New("boom"), 1)
	var coded ExitCoder
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 1, coded.ExitCode())
}

func TestNewCodedErrorNil(t *testing.T) {
	assert.NoError(t, NewCodedError(nil, 1))
}

func TestCodedErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	wrapped := NewCodedError(inner, 1)
	require.ErrorIs(t, wrapped, inner)
	assert.Equal(t, "inner", wrapped.Error())
}

func TestCodedErrorErrorsAs(t *testing.T) {
	inner := errors.New("inner")
	wrapped := fmt.Errorf("outer: %w", NewCodedError(inner, 1))

	var coded ExitCoder
	require.ErrorAs(t, wrapped, &coded)
	assert.Equal(t, 1, coded.ExitCode())
}
