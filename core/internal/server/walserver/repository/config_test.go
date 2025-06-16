package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigFile(t *testing.T) {
	config, err := createNewRepositoryConfiguration("test-password")
	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.Len(t, config.Keys, 1)
	assert.NotEmpty(t, config.Keys[0].CipherText)
	assert.NotEmpty(t, config.Keys[0].Iterations)
	assert.NotEmpty(t, config.Keys[0].Salt)
	assert.NotEmpty(t, config.Keys[0].Nonce)
}
