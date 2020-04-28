package test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTempFolder(t *testing.T) {
	folder := TempFolder()
	require.NotNil(t, folder)
	defer func() {
		require.Nil(t, os.Remove(folder))
	}()
	info, err := os.Stat(folder)
	require.False(t, os.IsNotExist(err))
	require.True(t, info.IsDir())
}
