// internal/importer/copy_test.go
package importer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	// Create temp directories
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(srcDir, "test.mkv")
	content := []byte("test video content")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	// Copy file
	dstPath := filepath.Join(dstDir, "copied.mkv")
	size, err := CopyFile(srcPath, dstPath)
	if err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}

	// Verify content
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Error("content mismatch")
	}
}

func TestCopyFile_CreatesDirectory(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "test.mkv")
	if err := os.WriteFile(srcPath, []byte("content"), 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	// Destination in nested directory that doesn't exist
	dstPath := filepath.Join(dstDir, "nested", "deep", "copied.mkv")
	_, err := CopyFile(srcPath, dstPath)
	if err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		t.Error("destination file should exist")
	}
}

func TestCopyFile_DestinationExists(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "test.mkv")
	if err := os.WriteFile(srcPath, []byte("content"), 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	dstPath := filepath.Join(dstDir, "existing.mkv")
	if err := os.WriteFile(dstPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("create existing: %v", err)
	}

	_, err := CopyFile(srcPath, dstPath)
	if err != ErrDestinationExists {
		t.Errorf("expected ErrDestinationExists, got %v", err)
	}
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	dstDir := t.TempDir()
	_, err := CopyFile("/nonexistent/file.mkv", filepath.Join(dstDir, "out.mkv"))
	if err == nil {
		t.Error("expected error for missing source")
	}
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
		if err := os.WriteFile(path, make([]byte, size), 0644); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	// Also create nested video
	nested := filepath.Join(dir, "subdir", "nested.mkv")
	if err := os.MkdirAll(filepath.Dir(nested), 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	if err := os.WriteFile(nested, make([]byte, 2000), 0644); err != nil {
		t.Fatalf("create nested: %v", err)
	}

	path, size, err := FindLargestVideo(dir)
	if err != nil {
		t.Fatalf("FindLargestVideo: %v", err)
	}

	if filepath.Base(path) != "nested.mkv" {
		t.Errorf("expected nested.mkv, got %s", filepath.Base(path))
	}
	if size != 2000 {
		t.Errorf("size = %d, want 2000", size)
	}
}

func TestFindLargestVideo_NoVideos(t *testing.T) {
	dir := t.TempDir()

	// Create only non-video files
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	_, _, err := FindLargestVideo(dir)
	if err != ErrNoVideoFile {
		t.Errorf("expected ErrNoVideoFile, got %v", err)
	}
}
