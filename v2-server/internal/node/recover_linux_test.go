//go:build linux

package node

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecoverRuntimePreservesFilesAndRespectsLiveOwner(t *testing.T) {
	root := socketDirectory(t)
	paths, lock, err := claimRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := RecoverRuntime(context.Background(), root, time.Millisecond); !errors.Is(err, ErrRuntimeBusy) {
		t.Fatalf("live owner bypassed: %v", err)
	}
	if err := os.MkdirAll(paths.Repositories, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(paths.Repositories, "keep")
	if err := os.WriteFile(file, []byte("workspace"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRuntime(t.Context(), root, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "workspace" {
		t.Fatalf("workspace changed: %q %v", data, err)
	}
	_, lock, err = claimRoot(root)
	if err != nil {
		t.Fatal("recovery retained owner lock", err)
	}
	_ = lock.Close()
}
