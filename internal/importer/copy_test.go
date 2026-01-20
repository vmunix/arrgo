// internal/importer/copy_test.go
package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFile(t *testing.T) {
	// Create temp directories
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(srcDir, "test.mkv")
	content := []byte("test video content")
	require.NoError(t, os.WriteFile(srcPath, content, 0644), "create source")

	// Copy file
	dstPath := filepath.Join(dstDir, "copied.mkv")
	size, err := CopyFile(srcPath, dstPath)
	require.NoError(t, err, "CopyFile")

	assert.Equal(t, int64(len(content)), size)

	// Verify content
	got, err := os.ReadFile(dstPath)
	require.NoError(t, err, "read dest")
	assert.Equal(t, string(content), string(got), "content mismatch")
}

func TestCopyFile_CreatesDirectory(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "test.mkv")
	require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0644), "create source")

	// Destination in nested directory that doesn't exist
	dstPath := filepath.Join(dstDir, "nested", "deep", "copied.mkv")
	_, err := CopyFile(srcPath, dstPath)
	require.NoError(t, err, "CopyFile")

	_, statErr := os.Stat(dstPath)
	assert.False(t, os.IsNotExist(statErr), "destination file should exist")
}

func TestCopyFile_DestinationExists(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "test.mkv")
	require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0644), "create source")

	dstPath := filepath.Join(dstDir, "existing.mkv")
	require.NoError(t, os.WriteFile(dstPath, []byte("existing"), 0644), "create existing")

	_, err := CopyFile(srcPath, dstPath)
	assert.ErrorIs(t, err, ErrDestinationExists)
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	dstDir := t.TempDir()
	_, err := CopyFile("/nonexistent/file.mkv", filepath.Join(dstDir, "out.mkv"))
	assert.Error(t, err, "expected error for missing source")
}

func TestFindLargestVideo(t *testing.T) {
	dir := t.TempDir()

	// Create files of different sizes
	files := map[string]int{
		"small.mkv":  100,
		"large.mkv":  1000,
		"medium.mp4": 500,
		"readme.txt": 50,
		"sample.mkv": 10, // Sample files should be skipped
	}

	for name, size := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, make([]byte, size), 0644), "create %s", name)
	}

	// Also create nested video
	nested := filepath.Join(dir, "subdir", "nested.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(nested), 0755), "create subdir")
	require.NoError(t, os.WriteFile(nested, make([]byte, 2000), 0644), "create nested")

	path, size, err := FindLargestVideo(dir)
	require.NoError(t, err, "FindLargestVideo")

	assert.Equal(t, "nested.mkv", filepath.Base(path))
	assert.Equal(t, int64(2000), size)
}

func TestFindLargestVideo_NoVideos(t *testing.T) {
	dir := t.TempDir()

	// Create only non-video files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0644), "create file")

	_, _, err := FindLargestVideo(dir)
	assert.ErrorIs(t, err, ErrNoVideoFile)
}
