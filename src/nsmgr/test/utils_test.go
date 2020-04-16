package test

import (
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestTempFolder(t *testing.T) {
	folder := TempFolder()
	defer func() {
		require.Nil(t, os.Remove(folder))
	}()
	info, err := os.Stat(folder)
	require.False(t, os.IsNotExist(err))
	require.True(t, info.IsDir())
}
