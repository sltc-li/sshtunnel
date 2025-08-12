package sshtunnel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadKeyFileExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	tmpFile, err := os.CreateTemp(home, "key")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	content := []byte("test-key")
	if err := os.WriteFile(tmpFile.Name(), content, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	tildePath := filepath.Join("~", filepath.Base(tmpFile.Name()))
	got, err := readKeyFile(tildePath)
	if err != nil {
		t.Fatalf("readKeyFile: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestReadKeyFileFallbackToFilesystem(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "key")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	content := []byte("fs-key")
	if err := os.WriteFile(tmpFile.Name(), content, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := readKeyFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("readKeyFile: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}
