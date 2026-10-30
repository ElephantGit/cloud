//go:build linux

package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/node"
)

const markerName = "project-runtime.json"

type registration struct {
	Version int    `json:"version"`
	Project string `json:"project"`
}

// Recover confirms every registered Node's old process tree has stopped before the application
// marks old sessions interrupted. Unknown entries and symlinks fail closed without deletion.
func Recover(ctx context.Context, root string) error {
	roots, err := registeredRoots(root)
	if err != nil {
		return err
	}
	for _, path := range roots {
		if node.RecoverRuntime(ctx, filepath.Join(path, "w"), 3*time.Second) != nil {
			return execution.ErrStopUnconfirmed
		}
	}
	return nil
}

// Cleanup removes only registered runtime directories, after all of them pass stop recovery.
// Repository mirrors and PostgreSQL history must live outside Root and are never touched.
// The embedding application must hold its data-directory lock throughout this call.
func Cleanup(ctx context.Context, root string) error {
	roots, err := registeredRoots(root)
	if err != nil {
		return err
	}
	for _, path := range roots {
		if node.RecoverRuntime(ctx, filepath.Join(path, "w"), 3*time.Second) != nil {
			return execution.ErrStopUnconfirmed
		}
	}
	for _, path := range roots {
		if err := ctx.Err(); err != nil {
			return err
		}
		if os.RemoveAll(path) != nil {
			return execution.ErrUnavailable
		}
	}
	return nil
}

// Cleanup first proves this Driver's sessions stopped, then removes its registered runtime data.
func (d *Driver) Cleanup(ctx context.Context) error {
	if err := d.Close(ctx); err != nil {
		return err
	}
	return Cleanup(ctx, d.opts.Root)
}

func registeredRoots(root string) ([]string, error) {
	if !filepath.IsAbs(root) {
		return nil, execution.ErrInvalidRequest
	}
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, execution.ErrUnavailable
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, execution.ErrUnavailable
	}
	roots := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, execution.ErrUnavailable
		}
		path := filepath.Join(root, entry.Name())
		marker, err := readRegistration(path)
		if err != nil || marker.Version != 1 || marker.Project == "" || entry.Name() != digest(marker.Project)[:16] {
			return nil, execution.ErrUnavailable
		}
		roots = append(roots, path)
	}
	return roots, nil
}

func readRegistration(root string) (registration, error) {
	var marker registration
	path := filepath.Join(root, markerName)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return marker, execution.ErrUnavailable
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &marker) != nil {
		return marker, execution.ErrUnavailable
	}
	return marker, nil
}

func (p *projectRuntime) register() error {
	if err := os.MkdirAll(p.root, 0700); err != nil {
		return err
	}
	marker := registration{Version: 1, Project: p.id}
	if stored, err := readRegistration(p.root); err == nil {
		if stored == marker {
			return nil
		}
		return execution.ErrUnavailable
	}
	entries, err := os.ReadDir(p.root)
	if err != nil || len(entries) != 0 {
		return execution.ErrUnavailable
	}
	file, err := os.OpenFile(filepath.Join(p.root, markerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	writeErr := json.NewEncoder(file).Encode(marker)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	dir, err := os.Open(p.root)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
