//go:build linux

package node

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Asset pins a previously prepared executable. Runtime startup never downloads or compiles.
type Asset struct{ Path, SHA256 string }

func copyAsset(asset Asset, destination string) error {
	want, err := hex.DecodeString(asset.SHA256)
	if err != nil || len(want) != sha256.Size || !filepath.IsAbs(asset.Path) {
		return errors.New("asset needs absolute path and SHA-256")
	}
	fd, err := unix.Open(asset.Path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	source := os.NewFile(uintptr(fd), "runtime asset")
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("asset is not a regular file")
	}
	output, err := os.CreateTemp(filepath.Dir(destination), ".asset-")
	if err != nil {
		return err
	}
	temp := output.Name()
	defer os.Remove(temp)
	defer output.Close()
	hash := sha256.New()
	if _, err = io.Copy(io.MultiWriter(output, hash), source); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != hex.EncodeToString(want) {
		return fmt.Errorf("asset checksum mismatch: %s", filepath.Base(asset.Path))
	}
	if err = output.Chmod(0700); err != nil {
		return err
	}
	if err = output.Sync(); err != nil {
		return err
	}
	if err = output.Close(); err != nil {
		return err
	}
	return os.Rename(temp, destination)
}
