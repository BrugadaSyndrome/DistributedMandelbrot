package misc

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestWriteFileThenReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	contents := []byte(`{"CoordinatorAddress": "127.0.0.1:51000"}`)

	written, err := WriteFile(path, contents)
	if err != nil {
		t.Fatalf("WriteFile returned error: %s", err)
	}
	if written != len(contents) {
		t.Errorf("WriteFile wrote %d bytes, want %d", written, len(contents))
	}

	err, read := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %s", err)
	}
	if !bytes.Equal(read, contents) {
		t.Errorf("ReadFile = %q, want %q", read, contents)
	}
}

func TestWriteFileTruncatesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if _, err := WriteFile(path, []byte("a much longer first version")); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteFile(path, []byte("short")); err != nil {
		t.Fatal(err)
	}
	if err, read := ReadFile(path); err != nil || string(read) != "short" {
		t.Errorf("ReadFile = %q, %v; want %q, nil", read, err, "short")
	}
}

func TestFileErrors(t *testing.T) {
	if err, _ := ReadFile(""); err == nil {
		t.Error("ReadFile with an empty name should return an error")
	}
	if err, _ := ReadFile(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("ReadFile of a missing file should return an error")
	}
	if _, err := WriteFile("", []byte("x")); err == nil {
		t.Error("WriteFile with an empty name should return an error")
	}
	if _, err := WriteFile(filepath.Join(t.TempDir(), "no", "such", "dir.txt"), []byte("x")); err == nil {
		t.Error("WriteFile into a missing directory should return an error")
	}
}
