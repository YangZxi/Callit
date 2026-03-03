package admin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRenameWorkerFileSuccess(t *testing.T) {
	dir := t.TempDir()
	oldName := "old.py"
	newName := "new.py"

	oldPath := filepath.Join(dir, oldName)
	if err := os.WriteFile(oldPath, []byte("print('ok')"), 0o644); err != nil {
		t.Fatalf("write source file failed: %v", err)
	}

	if err := renameWorkerFile(dir, oldName, newName); err != nil {
		t.Fatalf("renameWorkerFile failed: %v", err)
	}

	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source file should be removed after rename, got err=%v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, newName)); err != nil {
		t.Fatalf("renamed file should exist, err=%v", err)
	}
}

func TestRenameWorkerFileConflict(t *testing.T) {
	dir := t.TempDir()
	oldName := "worker.js"
	newName := "exists.js"

	if err := os.WriteFile(filepath.Join(dir, oldName), []byte("1"), 0o644); err != nil {
		t.Fatalf("write source file failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, newName), []byte("2"), 0o644); err != nil {
		t.Fatalf("write target file failed: %v", err)
	}

	err := renameWorkerFile(dir, oldName, newName)
	if !errors.Is(err, errTargetFileExists) {
		t.Fatalf("expected errTargetFileExists, got %v", err)
	}
}

func TestRenameWorkerFileSourceNotExist(t *testing.T) {
	dir := t.TempDir()
	err := renameWorkerFile(dir, "not_found.ts", "new.ts")
	if !errors.Is(err, errSourceFileNotExist) {
		t.Fatalf("expected errSourceFileNotExist, got %v", err)
	}
}
