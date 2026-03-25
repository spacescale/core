package sysinfo

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadBootID(t *testing.T) {
	bootID, err := ReadBootID()
	require.NoError(t, err)

	assert.NotEmpty(t, bootID)
	assert.Equal(t, strings.TrimSpace(bootID), bootID)
	assert.Len(t, bootID, 36)
	assert.Contains(t, bootID, "-")
}
